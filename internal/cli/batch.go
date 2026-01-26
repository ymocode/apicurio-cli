package cli

import (
	"github.com/spf13/cobra"
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Batch operations for multiple schemas",
	Long:  `Perform batch operations on multiple schema files in a directory.`,
}

func init() {
	rootCmd.AddCommand(batchCmd)

	// Common batch flags
	batchCmd.PersistentFlags().String("dir", ".", "Directory to scan for schemas")
	batchCmd.PersistentFlags().String("pattern", "*.avsc", "File pattern to match (e.g., *.avsc, *.json)")
	batchCmd.PersistentFlags().Bool("recursive", true, "Scan directories recursively")
	batchCmd.PersistentFlags().Int("parallel", 4, "Number of parallel workers")
	batchCmd.PersistentFlags().Bool("continue-on-error", false, "Continue processing even if some schemas fail")
	batchCmd.PersistentFlags().String("format", "json", "Output format: json, table, summary, markdown")
}
