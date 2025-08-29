package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/developer-Bushido/mariadb2tidb/internal/transformer"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var transformCmd = &cobra.Command{
	Use:   "transform [input.sql|inputDir]",
	Short: "Transform MariaDB schema to TiDB compatible format",
	Long: `Transform MariaDB SQL schema to be compatible with TiDB.
Applies various transformation rules to ensure compatibility.
If a directory is provided, all .sql files will be processed recursively
and written to the output directory preserving structure.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, _ := cmd.Flags().GetString("config")
		cfg, err := config.LoadConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		var inputPath string
		if len(args) > 0 {
			inputPath = args[0]
		} else {
			inputPath = cfg.InputDir
		}
		if inputPath == "" {
			return fmt.Errorf("input path required")
		}

		outputPathFlag, _ := cmd.Flags().GetString("output")
		var outputPath string
		if outputPathFlag != "" {
			outputPath = outputPathFlag
		} else {
			outputPath = cfg.OutputDir
		}
		workers, _ := cmd.Flags().GetInt("workers")

		info, err := os.Stat(inputPath)
		if err != nil {
			return fmt.Errorf("failed to stat input path: %w", err)
		}

		if info.IsDir() {
			if outputPath == "" {
				return fmt.Errorf("output directory required when input is a directory")
			}
			processor := transformer.NewDirProcessor(cfg)
			// Build cancellable context with optional timeout
			timeout, _ := cmd.Flags().GetDuration("timeout")
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			spinner, _ := pterm.DefaultSpinner.Start("Transforming directory")
			if err := processor.ProcessDirectory(ctx, inputPath, outputPath, workers); err != nil {
				spinner.Fail("Directory transformation failed")
				return fmt.Errorf("directory transformation failed: %w", err)
			}
			spinner.Success("Directory transformed")
			pterm.Success.Printfln("Successfully transformed %s -> %s", inputPath, outputPath)
			return nil
		}

		// Load SQL from input file
		loader := parser.NewLoader()
		stmts, err := loader.LoadFromFile(inputPath)
		if err != nil {
			return fmt.Errorf("failed to load SQL file: %w", err)
		}

		spinner, _ := pterm.DefaultSpinner.Start("Transforming schema")

		// Create transformation engine and apply rules
		engine := transformer.NewEngine(cfg)
		transformedStmts, err := engine.Transform(stmts)
		if err != nil {
			spinner.Fail("Transformation failed")
			return fmt.Errorf("transformation failed: %w", err)
		}

		// Write transformed SQL
		writer := parser.NewWriter()
		outputSQL, err := writer.WriteStatements(transformedStmts)
		if err != nil {
			spinner.Fail("Failed to write SQL")
			return fmt.Errorf("failed to write SQL: %w", err)
		}

		if outputPath == "" {
			fmt.Print(outputSQL)
		} else {
			err = os.WriteFile(outputPath, []byte(outputSQL), 0644)
			if err != nil {
				spinner.Fail("Failed to write output file")
				return fmt.Errorf("failed to write output file: %w", err)
			}
		}

		spinner.Success("Transformation complete")

		if outputPath == "" {
			pterm.Success.Printfln("Successfully transformed %s", inputPath)
		} else {
			pterm.Success.Printfln("Successfully transformed %s -> %s", inputPath, outputPath)
		}

		return nil
	},
}

var transformDirCmd = &cobra.Command{
	Use:   "transform-dir [inputDir]",
	Short: "Transform all SQL files in a directory",
	Long:  `Recursively transform all .sql files in a directory tree to be TiDB compatible. Output files preserve directory structure relative to the input directory.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, _ := cmd.Flags().GetString("config")
		cfg, err := config.LoadConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		var inputDir string
		if len(args) > 0 {
			inputDir = args[0]
		} else {
			inputDir = cfg.InputDir
		}
		if inputDir == "" {
			return fmt.Errorf("input directory required")
		}

		outputDirFlag, _ := cmd.Flags().GetString("output")
		var outputDir string
		if outputDirFlag != "" {
			outputDir = outputDirFlag
		} else {
			outputDir = cfg.OutputDir
		}
		if outputDir == "" {
			return fmt.Errorf("output directory required")
		}
		workers, _ := cmd.Flags().GetInt("workers")

		processor := transformer.NewDirProcessor(cfg)
		timeout, _ := cmd.Flags().GetDuration("timeout")
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		spinner, _ := pterm.DefaultSpinner.Start("Transforming directory")
		if err := processor.ProcessDirectory(ctx, inputDir, outputDir, workers); err != nil {
			spinner.Fail("Directory transformation failed")
			return err
		}
		spinner.Success("Directory transformed")
		pterm.Success.Printfln("Transformed directory %s -> %s", inputDir, outputDir)
		return nil
	},
}

var extractCmd = &cobra.Command{
	Use:   "extract [input.sql]",
	Short: "Extract specific database from multi-database SQL file",
	Long:  `Extract a specific database schema from a SQL file containing multiple databases.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		inputFile := args[0]
		database, _ := cmd.Flags().GetString("database")
		outputFile, _ := cmd.Flags().GetString("output")

		pterm.Info.Printfln("Extracting database '%s' from %s -> %s (stub implementation)", database, inputFile, outputFile)
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import [schema.sql]",
	Short: "Import schema/data to TiDB with parallel execution",
	Long:  `Import schema and data to TiDB database using parallel connections for better performance.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		schemaFile := args[0]
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		parallel, _ := cmd.Flags().GetInt("parallel")

		pterm.Info.Printfln("Importing %s to %s:%d with %d parallel connections (stub implementation)",
			schemaFile, host, port, parallel)
		return nil
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate [schema.sql]",
	Short: "Validate SQL schema for TiDB compatibility",
	Long:  `Validate that the SQL schema is compatible with TiDB and report any issues.`,
	Args:  cobra.ExactArgs(1),
    RunE: func(_ *cobra.Command, args []string) error {
		schemaFile := args[0]

		pterm.Info.Printfln("Validating %s for TiDB compatibility (stub implementation)", schemaFile)
		return nil
	},
}

var diffCmd = &cobra.Command{
	Use:   "diff [original.sql] [transformed.sql]",
	Short: "Compare two SQL files and generate diff report",
	Long:  `Generate a human-readable diff report between original and transformed SQL files.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		originalFile := args[0]
		transformedFile := args[1]
		outputFile, _ := cmd.Flags().GetString("output")

		pterm.Info.Printfln("Generating diff between %s and %s -> %s (stub implementation)",
			originalFile, transformedFile, outputFile)
		return nil
	},
}

func init() {
	// Transform command flags
	transformCmd.Flags().StringP("output", "o", "", "Output file or directory (default: stdout or config output_dir)")
	transformCmd.Flags().IntP("workers", "w", 4, "Number of parallel workers (directory mode)")
	transformCmd.Flags().Duration("timeout", 0, "Optional timeout for directory transformations, e.g. 30m")
	transformCmd.Flags().String("config", "", "Path to YAML configuration file")

	// Transform-dir command flags
	transformDirCmd.Flags().StringP("output", "o", "", "Output directory (default: config output_dir)")
	transformDirCmd.Flags().IntP("workers", "w", 4, "Number of parallel workers")
	transformDirCmd.Flags().Duration("timeout", 0, "Optional timeout, e.g. 30m")
	transformDirCmd.Flags().String("config", "", "Path to YAML configuration file")

	// Extract command flags
	extractCmd.Flags().String("database", "", "Database name to extract (required)")
	extractCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	extractCmd.MarkFlagRequired("database")

	// Import command flags
	importCmd.Flags().String("host", "localhost", "TiDB host")
	importCmd.Flags().Int("port", 4000, "TiDB port")
	importCmd.Flags().Int("parallel", 8, "Number of parallel connections")

	// Diff command flags
	diffCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
}

// versionCmd prints detailed version information including commit and build date
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
    Run: func(_ *cobra.Command, _ []string) {
		pterm.Println(pterm.Blue("version:"), version)
		pterm.Println(pterm.Blue("commit:"), commit)
		pterm.Println(pterm.Blue("date:"), date)
	},
}
