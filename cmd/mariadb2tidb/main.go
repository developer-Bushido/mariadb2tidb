package main

import (
	"os"

	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	// Populated via -ldflags at build time
	version = "dev"
	commit  = "none"
	date    = "unknown"

	verbose bool
	noColor bool

	rootCmd = &cobra.Command{
		Use:   "mariadb2tidb",
		Short: "MariaDB to TiDB migration tool",
		Long: `A command-line tool for migrating MariaDB schemas and data to TiDB.
Provides functionality for schema transformation, data extraction, and parallel import.`,
		Version: version,
            PersistentPreRun: func(_ *cobra.Command, args []string) {
			if noColor {
				pterm.DisableColor()
			}
			if verbose {
				pterm.EnableDebugMessages()
			}
			// Initialize logger (development level if verbose)
			_ = utils.InitLogger(verbose)
		},
	}
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colorized output")

	// Add subcommands
	rootCmd.AddCommand(transformCmd)
	rootCmd.AddCommand(transformDirCmd)
	rootCmd.AddCommand(extractCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	// Ensure logger flushes if initialized
	defer func() { _ = utils.GetLogger().Sync() }()
	if err := rootCmd.Execute(); err != nil {
		pterm.Error.Printfln("%v", err)
		os.Exit(1)
	}
}
