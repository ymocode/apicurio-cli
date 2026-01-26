package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/batch"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
	"github.com/ymocode/apicurio-client/internal/operations"
	"github.com/ymocode/apicurio-client/internal/output"
	"github.com/ymocode/apicurio-client/internal/registry"
)

var batchValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate multiple schemas in a directory",
	Long:  `Validate all schemas in a directory against the registry for compatibility.`,
	RunE:  runBatchValidate,
}

func init() {
	batchCmd.AddCommand(batchValidateCmd)

	batchValidateCmd.Flags().Bool("fail-on-error", true, "Exit with error code if any validation fails")
	batchValidateCmd.Flags().Bool("fail-on-breaking", false, "Exit with error code only on breaking changes")
	batchValidateCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
}

func runBatchValidate(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()
	startTime := log.StartTimer("batch validation operation")

	// Get flags
	dir, _ := cmd.Flags().GetString("dir")
	pattern, _ := cmd.Flags().GetString("pattern")
	recursive, _ := cmd.Flags().GetBool("recursive")
	parallel, _ := cmd.Flags().GetInt("parallel")
	continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")
	failOnError, _ := cmd.Flags().GetBool("fail-on-error")
	failOnBreaking, _ := cmd.Flags().GetBool("fail-on-breaking")

	log.Info("Discovering schemas: dir=%s, pattern=%s, recursive=%v", dir, pattern, recursive)

	// Discover schemas
	files, err := batch.DiscoverSchemas(dir, pattern, recursive)
	if err != nil {
		log.Error("Failed to discover schemas: %v", err)
		return fmt.Errorf("failed to discover schemas: %w", err)
	}

	if len(files) == 0 {
		log.Warn("No schema files found in %s matching pattern %s", dir, pattern)
		fmt.Printf("No schema files found in %s matching pattern %s\n", dir, pattern)
		return nil
	}

	log.Info("Found %d schema file(s) to validate", len(files))
	fmt.Printf("Found %d schema file(s) to validate\n\n", len(files))

	log.Info("Loading configuration")
	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	log.Debug("Registry URL: %s, API version: %s", cfg.RegistryURL, cfg.APIVersion)

	log.Info("Creating registry client (API: %s)", cfg.APIVersion)
	// Create registry client
	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.BatchOperationTimeout)
	defer cancel()

	// Process batch
	opts := batch.BatchOptions{
		Directory:       dir,
		Pattern:         pattern,
		Recursive:       recursive,
		Parallel:        parallel,
		ContinueOnError: continueOnError,
	}

	log.Info("Processing batch with %d workers", parallel)
	summary := batch.ProcessBatch(ctx, files, opts, func(ctx context.Context, filePath string) batch.BatchResult {
		return operations.ProcessSingleValidation(ctx, client, cfg, filePath)
	})

	duration := startTime.Stop()
	log.Info("Batch validation completed: total=%d, success=%d, failed=%d, skipped=%d (duration: %dms)",
		summary.Total, summary.Success, summary.Failed, summary.Skipped, duration.Milliseconds())

	// Format and display results
	batchOutput, err := batch.FormatBatchSummary(summary, format)
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	if err := output.WriteOutput(batchOutput, outputPath); err != nil {
		return err
	}

	// Determine exit code
	if failOnBreaking {
		// Check if any schema has breaking changes
		for _, result := range summary.Results {
			if result.Status == config.StatusFailed && result.ChangeLevel == config.ChangeLevelMajor {
				log.Error("Breaking changes detected in one or more schemas")
				return fmt.Errorf("breaking changes detected in one or more schemas")
			}
		}
	} else if failOnError && summary.Failed > 0 {
		log.Error("Validation failed for %d schema(s)", summary.Failed)
		return fmt.Errorf("validation failed for %d schema(s)", summary.Failed)
	}

	return nil
}
