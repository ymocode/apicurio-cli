package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
	"github.com/ymocode/apicurio-client/internal/operations"
	"github.com/ymocode/apicurio-client/internal/output"
	"github.com/ymocode/apicurio-client/internal/registry"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate an Avro schema against registry compatibility rules",
	Long:  `Perform dry-run validation of an Avro schema using Apicurio's TestUpdateArtifact API.`,
	RunE:  runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)

	validateCmd.Flags().String("file", "", "Path to Avro schema file (required)")
	validateCmd.Flags().String("format", "json", "Output format: json, table, summary, markdown")
	validateCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")

	validateCmd.MarkFlagRequired("file")
}

func runValidate(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()
	startTime := log.StartTimer("validation operation")

	// Get flags
	schemaFile, _ := cmd.Flags().GetString("file")
	if schemaFile == "" {
		return fmt.Errorf("--file is required")
	}
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")

	log.Info("Loading configuration")
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	log.Debug("Registry URL: %s, API version: %s", cfg.RegistryURL, cfg.APIVersion)

	log.Info("Parsing and validating schema file: %s", schemaFile)

	log.Info("Creating registry client (API: %s)", cfg.APIVersion)
	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultOperationTimeout)
	defer cancel()

	// Use the shared validation logic (single file = batch of 1)
	log.Info("Validating schema compatibility")
	batchResult := operations.ProcessSingleValidation(ctx, client, cfg, schemaFile)

	duration := startTime.Stop()

	// Check if validation succeeded
	if batchResult.Status == config.StatusFailed {
		if batchResult.ValidationResult != nil {
			log.Error("[%s] Validation failed: %s", batchResult.ValidationResult.FQN, strings.Join(batchResult.Errors, "; "))
		} else {
			log.Error("Validation failed: %s", strings.Join(batchResult.Errors, "; "))
		}
	} else {
		if batchResult.ValidationResult != nil {
			log.Info("[%s] Validation successful: %s (duration: %dms)", batchResult.ValidationResult.FQN, batchResult.CompatibilityType, duration.Milliseconds())
		} else {
			log.Info("Validation successful: %s (duration: %dms)", batchResult.CompatibilityType, duration.Milliseconds())
		}
	}

	// Extract ValidationResult for display
	if batchResult.ValidationResult == nil {
		return fmt.Errorf("internal error: validation result is nil")
	}

	if err := output.PrintValidationResult(*batchResult.ValidationResult, format, outputPath); err != nil {
		return err
	}

	if !batchResult.ValidationResult.IsCompatible {
		os.Exit(1)
	}

	return nil
}
