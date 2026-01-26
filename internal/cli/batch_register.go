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
	"github.com/ymocode/apicurio-client/internal/schema"
)

var batchRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register multiple schemas in a directory",
	Long:  `Register all schemas in a directory to the registry.`,
	RunE:  runBatchRegister,
}

func init() {
	batchCmd.AddCommand(batchRegisterCmd)

	batchRegisterCmd.Flags().Bool("dry-run", false, "Preview registration without actually registering")
	batchRegisterCmd.Flags().Bool("skip-validation", false, "Skip validation before registration")
	batchRegisterCmd.Flags().Bool("fail-on-error", true, "Exit with error code if any registration fails")
	batchRegisterCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
}

func runBatchRegister(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()
	startTime := log.StartTimer("batch registration operation")

	// Get flags
	dir, _ := cmd.Flags().GetString("dir")
	pattern, _ := cmd.Flags().GetString("pattern")
	recursive, _ := cmd.Flags().GetBool("recursive")
	parallel, _ := cmd.Flags().GetInt("parallel")
	continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipValidation, _ := cmd.Flags().GetBool("skip-validation")
	failOnError, _ := cmd.Flags().GetBool("fail-on-error")

	if dryRun {
		log.Info("Running in DRY RUN mode - no actual registration will occur")
	}

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

	log.Info("Found %d schema file(s) to register", len(files))
	if dryRun {
		fmt.Printf("[DRY RUN] Found %d schema file(s) to register\n\n", len(files))
	} else {
		fmt.Printf("Found %d schema file(s) to register\n\n", len(files))
	}

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
		DryRun:          dryRun,
	}

	log.Info("Processing batch with %d workers (skipValidation=%v)", parallel, skipValidation)
	summary := batch.ProcessBatch(ctx, files, opts, func(ctx context.Context, filePath string) batch.BatchResult {
		return registerSingleSchema(ctx, client, cfg, filePath, dryRun, skipValidation)
	})

	duration := startTime.Stop()
	log.Info("Batch registration completed: total=%d, success=%d, failed=%d, skipped=%d (duration: %dms)",
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
	if failOnError && summary.Failed > 0 {
		log.Error("Registration failed for %d schema(s)", summary.Failed)
		return fmt.Errorf("registration failed for %d schema(s)", summary.Failed)
	}

	return nil
}

func registerSingleSchema(ctx context.Context, client registry.RegistryClient, cfg *config.Config, filePath string, dryRun bool, skipValidation bool) batch.BatchResult {
	result := batch.BatchResult{
		FilePath: filePath,
		Action:   config.ActionRegistered,
		Status:   config.StatusSkipped,
	}

	if dryRun {
		result.Action = "dry-run"
	}

	// Parse schema
	avroSchema, err := schema.ParseAvroSchema(filePath)
	if err != nil {
		result.Status = config.StatusFailed
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to parse schema: %v", err))
		result.Message = "Parse error"
		return result
	}

	result.Namespace = avroSchema.Namespace
	result.Name = avroSchema.Name
	result.Version = avroSchema.Version

	// If dry-run, validate and stop
	if dryRun {
		if !skipValidation {
			valResult := operations.ValidateSchema(ctx, client, cfg, avroSchema)
			if !valResult.Success || !valResult.IsCompatible {
				result.Status = config.StatusFailed
				result.Errors = valResult.ValidationErrors
				result.Message = "Would fail validation"
				return result
			}
		}
		result.Status = config.StatusSuccess
		result.Message = "Would register successfully"
		return result
	}

	// Use the shared registration function
	regResult := operations.RegisterSchema(ctx, client, cfg, avroSchema, skipValidation)
	if !regResult.Success {
		result.Status = config.StatusFailed
		result.Errors = append(result.Errors, regResult.Error.Error())
		result.Message = "Registration failed"
		return result
	}

	result.Status = config.StatusSuccess
	if regResult.IsNewArtifact {
		result.Message = fmt.Sprintf("Created artifact %s (global ID: %d)", regResult.Version, regResult.GlobalID)
	} else {
		result.Message = fmt.Sprintf("Created version %s (global ID: %d)", regResult.Version, regResult.GlobalID)
	}
	return result
}
