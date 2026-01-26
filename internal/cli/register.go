package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
	"github.com/ymocode/apicurio-client/internal/operations"
	"github.com/ymocode/apicurio-client/internal/output"
	"github.com/ymocode/apicurio-client/internal/registry"
	"github.com/ymocode/apicurio-client/internal/schema"
)

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register an Avro schema to Apicurio Registry",
	Long:  `Register an Avro schema to Apicurio Registry with automatic versioning and validation.`,
	RunE:  runRegister,
}

func init() {
	rootCmd.AddCommand(registerCmd)

	registerCmd.Flags().String("file", "", "Path to Avro schema file (required)")
	registerCmd.Flags().Bool("dry-run", false, "Preview registration without actually registering")
	registerCmd.Flags().Bool("skip-validation", false, "Skip validation before registration")
	registerCmd.Flags().String("format", "json", "Output format: json, table, summary, markdown")
	registerCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")

	_ = registerCmd.MarkFlagRequired("file")
}

func runRegister(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()
	startTime := log.StartTimer("register operation")

	// Get flags
	schemaFile, _ := cmd.Flags().GetString("file")
	if schemaFile == "" {
		return fmt.Errorf("--file is required")
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipValidation, _ := cmd.Flags().GetBool("skip-validation")
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")

	log.Info("Loading configuration")
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	log.Debug("Registry URL: %s, API version: %s", cfg.RegistryURL, cfg.APIVersion)

	log.Info("Parsing schema file: %s", schemaFile)
	avroSchema, err := schema.ParseAvroSchema(schemaFile)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}
	log.Info("Schema parsed: %s.%s (version %s)", avroSchema.Namespace, avroSchema.Name, avroSchema.Version)

	log.Info("Creating registry client (API: %s)", cfg.APIVersion)
	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultOperationTimeout)
	defer cancel()

	group := cfg.GetGroup(avroSchema.Namespace)
	artifactID := cfg.GetArtifactID(avroSchema.Name)
	fqn := cfg.GetFQN(avroSchema.Namespace, avroSchema.Name)

	// Dry-run mode: validate only
	if dryRun {
		log.Info("Dry-run mode: validating without registration")
		var errors []string
		success := true

		if !skipValidation {
			valResult := operations.ValidateSchema(ctx, client, cfg, avroSchema)
			if !valResult.Success || !valResult.IsCompatible {
				success = false
				errors = valResult.ValidationErrors
			}
		}

		duration := startTime.Stop()
		dryRunOutput := output.DryRunOutput{
			DryRun:     true,
			Success:    success,
			FQN:        fqn,
			Group:      group,
			ArtifactID: artifactID,
			Version:    avroSchema.Version,
			DurationMs: duration.Milliseconds(),
			Errors:     errors,
		}

		if err := output.PrintDryRunResult(dryRunOutput, format, outputPath); err != nil {
			return err
		}

		if !success {
			return fmt.Errorf("dry-run validation failed")
		}
		return nil
	}

	// Use the shared registration function
	result := operations.RegisterSchema(ctx, client, cfg, avroSchema, skipValidation)
	if !result.Success {
		return result.Error
	}

	duration := startTime.Stop()

	// Determine action
	action := "created_artifact"
	if !result.IsNewArtifact {
		action = "created_version"
	}

	regOutput := output.RegistrationOutput{
		Success:       true,
		Action:        action,
		FQN:           fqn,
		Group:         group,
		ArtifactID:    artifactID,
		Version:       result.Version,
		GlobalID:      result.GlobalID,
		ContentID:     result.ContentID,
		CreatedOn:     result.CreatedOn,
		DurationMs:    duration.Milliseconds(),
		IsNewArtifact: result.IsNewArtifact,
	}

	return output.PrintRegistrationResult(regOutput, format, outputPath)
}
