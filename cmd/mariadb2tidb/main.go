package main

import (
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
	verbose bool
	noColor bool

	rootCmd = &cobra.Command{
		Use:   "mariadb2tidb",
		Short: "MariaDB to TiDB migration tool",
		Long: `A command-line tool for migrating MariaDB schemas and data to TiDB.
Provides functionality for schema transformation, data extraction, and parallel import.`,
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if noColor {
				pterm.DisableColor()
			}
			if verbose {
				pterm.EnableDebugMessages()
			}
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
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		pterm.Error.Printfln("%v", err)
		os.Exit(1)
	}
}
