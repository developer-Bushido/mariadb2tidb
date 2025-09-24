package transformer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	progressbar "github.com/schollz/progressbar/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	intparser "github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
)

// DirProcessor transforms all SQL files in a directory tree.
type DirProcessor struct {
	engine *Engine
	config *config.Config
	logger *zap.Logger
}

// NewDirProcessor creates a directory processor with a fresh transformation engine.
func NewDirProcessor(cfg *config.Config) *DirProcessor {
	return &DirProcessor{
		engine: NewEngine(cfg),
		config: cfg,
		logger: utils.GetLogger(),
	}
}

// ProcessDirectory transforms all .sql files found under inputDir and writes
// the output to outputDir while preserving the relative directory structure.
// Workers controls the maximum number of files processed in parallel.
func (p *DirProcessor) ProcessDirectory(ctx context.Context, inputDir, outputDir string, workers int) error {
	if workers <= 0 {
		workers = 1
	}

	// Collect all SQL files
	var files []string
	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	if len(files) == 0 {
		p.logger.Warn("no SQL files found", zap.String("dir", inputDir))
		return nil
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Set up progress bar
	bar := progressbar.NewOptions(len(files),
		progressbar.OptionSetDescription("Transforming"),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for _, file := range files {
		file := file // capture range variable
		g.Go(func() error {
			if err := p.processFile(ctx, inputDir, outputDir, file); err != nil {
				return err
			}
			bar.Add(1)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

func (p *DirProcessor) processFile(ctx context.Context, inputDir, outputDir, filePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	loader := intparser.NewLoaderWithConfig(p.config)
	writer := intparser.NewWriter()

	stmts, err := loader.LoadFromFile(filePath)
	if err != nil {
		return fmt.Errorf("load %s: %w", filePath, err)
	}

	transformed, err := p.engine.Transform(stmts)
	if err != nil {
		return fmt.Errorf("transform %s: %w", filePath, err)
	}

	outputSQL, err := writer.WriteStatements(transformed)
	if err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}

	rel, err := filepath.Rel(inputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel path %s: %w", filePath, err)
	}
	outPath := filepath.Join(outputDir, rel)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outPath, err)
	}
	if err := os.WriteFile(outPath, []byte(outputSQL), 0o644); err != nil {
		return fmt.Errorf("write file %s: %w", outPath, err)
	}
	p.logger.Debug("processed file", zap.String("input", filePath), zap.String("output", outPath))
	return nil
}
