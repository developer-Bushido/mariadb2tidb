// Package transformer wires the parser and rules together: the Engine
// applies rules to parsed statements, and DirProcessor runs the pipeline
// over directory trees in parallel.
package transformer

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	progressbar "github.com/schollz/progressbar/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
)

// dumplingDataFile matches data chunks of Dumpling (db.table.000000000.sql,
// db.table.0000000010000.csv.gz) and mydumper (db.table.00000.sql,
// db.table.00000.00001.sql): db.table, then a zero-padded chunk number of
// at least five digits. They hold rows, not schema, and are copied
// unchanged. Ordinary names such as shop.20261022.sql do not match.
var dumplingDataFile = regexp.MustCompile(`(?i)^[^.]+\.[^.]+\.\d{5,}(\.\d+)?\.(sql|csv|dat)(\.(gz|zst|snappy))?$`)

// DirProcessor transforms all SQL files in a directory tree.
type DirProcessor struct {
	config *config.Config
	report *compat.Reporter
	logger *zap.Logger
}

// NewDirProcessor creates a directory processor. A nil cfg means the
// default configuration.
func NewDirProcessor(cfg *config.Config) *DirProcessor {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &DirProcessor{
		config: cfg,
		report: compat.NewReporter(),
		logger: utils.GetLogger(),
	}
}

// Warnings returns the migration warnings of the last run, prefixed with
// the file they came from.
func (p *DirProcessor) Warnings() []string {
	return p.report.Warnings()
}

// ProcessDirectory mirrors inputDir into outputDir: schema .sql files are
// transformed, Dumpling data chunks and all other files (metadata, CSV,
// compressed chunks) are copied byte for byte, so the output directory is a
// complete source for TiDB Lightning. Workers controls the maximum number of
// files processed in parallel.
func (p *DirProcessor) ProcessDirectory(ctx context.Context, inputDir, outputDir string, workers int) error {
	if workers <= 0 {
		workers = 1
	}
	inputDir, err := checkDirs(inputDir, outputDir)
	if err != nil {
		return err
	}

	var files []string
	err = filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.Type()&os.ModeSymlink != 0:
			// Skipping it would silently lose its data; following it could
			// leave the tree.
			return fmt.Errorf("%s is a symbolic link; copy the real file or directory into the input tree", path)
		case d.Type().IsRegular():
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	if len(files) == 0 {
		p.logger.Warn("no files found", zap.String("dir", inputDir))
		return nil
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	bar := progressbar.NewOptions(len(files),
		progressbar.OptionSetDescription("Transforming"),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for _, file := range files {
		g.Go(func() error {
			if err := p.processFile(ctx, inputDir, outputDir, file); err != nil {
				return err
			}
			_ = bar.Add(1)
			return nil
		})
	}

	return g.Wait()
}

// checkDirs resolves a symlinked input root and refuses an output directory
// that is the input or nested with it: copying a file onto itself
// truncates it.
func checkDirs(inputDir, outputDir string) (string, error) {
	in, err := filepath.EvalSymlinks(inputDir)
	if err != nil {
		return "", fmt.Errorf("input directory: %w", err)
	}
	if in, err = filepath.Abs(in); err != nil {
		return "", err
	}
	out, err := filepath.Abs(outputDir)
	if err != nil {
		return "", err
	}
	out = resolveExisting(out)
	if within(out, in) || within(in, out) {
		return "", fmt.Errorf("output directory %s must not be the input directory %s or inside/around it", outputDir, inputDir)
	}
	return in, nil
}

// resolveExisting resolves symlinks in the longest existing prefix of an
// absolute path (the output directory may not exist yet, but its parent
// may be a symlink such as macOS's /var -> /private/var).
func resolveExisting(path string) string {
	var rest []string
	for p := path; ; p = filepath.Dir(p) {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(append([]string{resolved}, rest...)...)
		}
		if parent := filepath.Dir(p); parent == p {
			return path
		}
		rest = append([]string{filepath.Base(p)}, rest...)
	}
}

func within(path, dir string) bool {
	if rel, err := filepath.Rel(dir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	// Names can differ and still be the same directory: case-insensitive
	// file systems (macOS by default: Dump and dump), hard-to-spot links.
	di, err := os.Stat(dir)
	if err != nil {
		return false
	}
	for p := path; ; p = filepath.Dir(p) {
		if fi, err := os.Stat(p); err == nil && os.SameFile(fi, di) {
			return true
		}
		if filepath.Dir(p) == p {
			return false
		}
	}
}

// compression returns the compression suffix of name (".gz", ".zst",
// ".snappy") and the name without it.
func compression(name string) (string, string) {
	for _, ext := range []string{".gz", ".zst", ".snappy"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			return ext, name[:len(name)-len(ext)]
		}
	}
	return "", name
}

// isSchemaFile reports .sql files to transform (possibly compressed):
// everything except Dumpling data chunks.
func isSchemaFile(name string) bool {
	_, base := compression(name)
	return strings.HasSuffix(strings.ToLower(base), ".sql") && !dumplingDataFile.MatchString(filepath.Base(name))
}

func (p *DirProcessor) processFile(ctx context.Context, inputDir, outputDir, filePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rel, err := filepath.Rel(inputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel path %s: %w", filePath, err)
	}
	outPath := filepath.Join(outputDir, rel)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outPath, err)
	}

	if !isSchemaFile(filepath.Base(filePath)) {
		if err := copyFile(filePath, outPath); err != nil {
			return fmt.Errorf("copy %s: %w", filePath, err)
		}
		p.logger.Debug("copied file", zap.String("input", filePath), zap.String("output", outPath))
		return nil
	}

	// Per-file reporter so warnings name the file they came from.
	fileReport := compat.NewReporter()
	defer func() {
		for _, w := range fileReport.Warnings() {
			p.report.Warnf("%s: %s", rel, w)
		}
	}()

	ext, _ := compression(filePath)
	if ext != "" && ext != ".gz" {
		return fmt.Errorf("%s: %s-compressed schema files are not supported; decompress them first", filePath, ext)
	}
	input, err := readMaybeGzip(filePath, ext == ".gz")
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}
	outputSQL, err := TransformString(p.config, string(input), fileReport)
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}

	if err := writeMaybeGzip(outPath, []byte(outputSQL), ext == ".gz"); err != nil {
		return fmt.Errorf("write file %s: %w", outPath, err)
	}
	p.logger.Debug("processed file", zap.String("input", filePath), zap.String("output", outPath))
	return nil
}

func readMaybeGzip(path string, gz bool) ([]byte, error) {
	if !gz {
		return os.ReadFile(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	return io.ReadAll(zr)
}

func writeMaybeGzip(path string, data []byte, gz bool) error {
	if gz {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		data = buf.Bytes()
	}
	return writeAtomic(path, bytes.NewReader(data))
}

// writeAtomic writes r to a temporary file next to path and renames it into
// place. A hard link or symlink already at path (to an input file, say) is
// replaced instead of written through, so the input is never truncated.
func writeAtomic(path string, r io.Reader) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	return writeAtomic(dst, in)
}
