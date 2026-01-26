// Package cli provides cobra commands for the apicurio-client CLI.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/asyncapi"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
	"github.com/ymocode/apicurio-client/internal/output"
	"github.com/ymocode/apicurio-client/internal/registry"
	"gopkg.in/yaml.v3"
)

var asyncapiCmd = &cobra.Command{
	Use:   "asyncapi",
	Short: "AsyncAPI document operations",
	Long:  `Register and manage AsyncAPI documents in Apicurio Registry.`,
}

var asyncapiValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate an AsyncAPI document against the registry",
	Long: `Validate an AsyncAPI document before registration.

Checks:
  - Document structure (x-namespace, x-domain, info.version)
  - Version is valid semver
  - Version discipline: if content changed, version must be higher
  - Referenced schemas exist in registry

Note: Validation requires API version V3.`,
	RunE: runAsyncAPIValidate,
}

var asyncapiRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register an AsyncAPI document to Apicurio Registry",
	Long: `Register an AsyncAPI document to Apicurio Registry.

The document must contain:
  - x-namespace: Used as group ID (can be overridden with --group)
  - x-domain: Used as artifact ID (can be overridden with --artifact-id)
  - info.version: Used as version (can be overridden with --version)

Schema references in components.messages.*.payload.schema.$ref are automatically
extracted and registered as artifact references.

Validation is performed before registration unless --skip-validation is used.

Note: AsyncAPI registration requires API version V3.`,
	RunE: runAsyncAPIRegister,
}

var asyncapiGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get an AsyncAPI document from Apicurio Registry with dereferenced schemas",
	Long: `Retrieve an AsyncAPI document from Apicurio Registry with all external
schema references resolved inline.

This command fetches the document with DEREFERENCE mode and automatically fixes
any Avro schemas that are not properly wrapped in Multi-Format Schema Objects
(a known Apicurio bug for AsyncAPI 3.0).

The output is a valid, self-contained AsyncAPI document.

Note: This command requires API version V3.`,
	RunE: runAsyncAPIGet,
}

func init() {
	rootCmd.AddCommand(asyncapiCmd)
	asyncapiCmd.AddCommand(asyncapiValidateCmd)
	asyncapiCmd.AddCommand(asyncapiRegisterCmd)
	asyncapiCmd.AddCommand(asyncapiGetCmd)

	// AsyncAPI validate flags
	asyncapiValidateCmd.Flags().String("file", "", "Path to AsyncAPI YAML file (required)")
	asyncapiValidateCmd.Flags().String("version", "", "Override version from document")
	asyncapiValidateCmd.Flags().String("format", "json", "Output format: json, table, summary")
	asyncapiValidateCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	_ = asyncapiValidateCmd.MarkFlagRequired("file")

	// AsyncAPI register flags
	asyncapiRegisterCmd.Flags().String("file", "", "Path to AsyncAPI YAML file (required)")
	asyncapiRegisterCmd.Flags().String("version", "", "Override version from document")
	asyncapiRegisterCmd.Flags().String("format", "json", "Output format: json, table, summary")
	asyncapiRegisterCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	asyncapiRegisterCmd.Flags().Bool("skip-validation", false, "Skip validation before registration")
	_ = asyncapiRegisterCmd.MarkFlagRequired("file")

	// AsyncAPI get flags
	asyncapiGetCmd.Flags().String("version", "branch=latest", "Version to retrieve (default: branch=latest)")
	asyncapiGetCmd.Flags().String("format", "yaml", "Output format: json, yaml")
	asyncapiGetCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	asyncapiGetCmd.Flags().Bool("no-fix", false, "Do not fix broken Avro schema wrapping")
}

// runAsyncAPIValidate validates an AsyncAPI document against the registry
func runAsyncAPIValidate(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()

	// Get flags
	file, _ := cmd.Flags().GetString("file")
	versionOverride, _ := cmd.Flags().GetString("version")
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")

	log.Info("Loading configuration")
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check API version - AsyncAPI requires V3
	if cfg.APIVersion != config.APIVersionV3 {
		return fmt.Errorf("AsyncAPI validation requires API version V3, current: %s. Use --api-version v3", cfg.APIVersion)
	}

	log.Info("Parsing AsyncAPI document: %s", file)
	spec, err := asyncapi.ParseFile(file)
	if err != nil {
		return fmt.Errorf("failed to parse AsyncAPI document: %w", err)
	}

	// Apply overrides
	groupID := cfg.GetGroup(spec.GroupID)
	artifactID := cfg.GetArtifactID(spec.ArtifactID)
	if versionOverride != "" {
		spec.Version = versionOverride
	}

	log.Info("AsyncAPI document parsed: group=%s, artifactId=%s, version=%s, refs=%d",
		groupID, artifactID, spec.Version, len(spec.References))

	log.Info("Creating registry client (API: %s)", cfg.APIVersion)
	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultOperationTimeout)
	defer cancel()

	log.Info("Validating against registry")
	result, err := asyncapi.Validate(ctx, client, spec, groupID, artifactID)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Print result
	valOutput := output.AsyncAPIValidationOutput{
		Valid:           result.Valid,
		Group:           groupID,
		ArtifactID:      artifactID,
		ProposedVersion: result.ProposedVersion,
		CurrentVersion:  result.CurrentVersion,
		IsNew:           result.IsNew,
		IsIdentical:     result.IsIdentical,
		Errors:          result.Errors,
		Warnings:        result.Warnings,
		MissingRefs:     result.MissingRefs,
	}
	if err := output.PrintAsyncAPIValidationResult(valOutput, format, outputPath); err != nil {
		return err
	}

	if !result.Valid {
		os.Exit(1)
	}

	return nil
}

func runAsyncAPIRegister(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()
	startTime := time.Now()

	// Get flags
	file, _ := cmd.Flags().GetString("file")
	versionOverride, _ := cmd.Flags().GetString("version")
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")
	skipValidation, _ := cmd.Flags().GetBool("skip-validation")

	log.Info("Loading configuration")
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check API version - AsyncAPI requires V3
	if cfg.APIVersion != config.APIVersionV3 {
		return fmt.Errorf("AsyncAPI registration requires API version V3, current: %s. Use --api-version v3", cfg.APIVersion)
	}

	log.Info("Parsing AsyncAPI document: %s", file)
	spec, err := asyncapi.ParseFile(file)
	if err != nil {
		return fmt.Errorf("failed to parse AsyncAPI document: %w", err)
	}

	// Apply overrides from flags (--group and --artifact-id are root persistent flags)
	groupID := cfg.GetGroup(spec.GroupID)
	artifactID := cfg.GetArtifactID(spec.ArtifactID)
	version := spec.Version
	if versionOverride != "" {
		version = versionOverride
		spec.Version = versionOverride
	}

	log.Info("AsyncAPI document parsed: group=%s, artifactId=%s, version=%s, refs=%d",
		groupID, artifactID, version, len(spec.References))

	for _, ref := range spec.References {
		log.Debug("Reference: %s", ref.Name)
	}

	log.Info("Creating registry client (API: %s)", cfg.APIVersion)
	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultOperationTimeout)
	defer cancel()

	// Validate before registration (unless skipped)
	if !skipValidation {
		log.Info("Validating before registration")
		var valResult *asyncapi.ValidationResult
		valResult, err = asyncapi.Validate(ctx, client, spec, groupID, artifactID)
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}

		if !valResult.Valid {
			for _, e := range valResult.Errors {
				log.Error("Validation error: %s", e)
			}
			return fmt.Errorf("validation failed: %d error(s)", len(valResult.Errors))
		}

		// Skip registration if content is identical
		if valResult.IsIdentical {
			log.Info("Content is identical to existing version, skipping registration")
			regOutput := output.RegistrationOutput{
				Success:       true,
				Action:        "unchanged",
				FQN:           fmt.Sprintf("%s.%s", groupID, artifactID),
				Group:         groupID,
				ArtifactID:    artifactID,
				Version:       valResult.CurrentVersion,
				DurationMs:    time.Since(startTime).Milliseconds(),
				IsNewArtifact: false,
			}
			return output.PrintRegistrationResult(regOutput, format, outputPath)
		}

		for _, w := range valResult.Warnings {
			log.Warn("%s", w)
		}
	}

	log.Info("Registering AsyncAPI document")
	metadata, err := client.CreateArtifactWithReferences(
		ctx,
		groupID,
		artifactID,
		spec.GetArtifactType(),
		version,
		spec.Title,
		spec.Description,
		spec.GetContentString(),
		spec.GetContentType(),
		spec.References,
	)
	if err != nil {
		return fmt.Errorf("failed to register AsyncAPI document: %w", err)
	}

	duration := time.Since(startTime)

	// Determine action
	action := "created"
	if metadata.Version != version {
		action = "updated"
	}

	// Format output
	regOutput := output.RegistrationOutput{
		Success:       true,
		Action:        action,
		FQN:           fmt.Sprintf("%s.%s", groupID, artifactID),
		Group:         groupID,
		ArtifactID:    artifactID,
		Version:       metadata.Version,
		GlobalID:      metadata.GlobalID,
		ContentID:     metadata.ContentID,
		CreatedOn:     metadata.CreatedOn,
		DurationMs:    duration.Milliseconds(),
		IsNewArtifact: action == "created",
	}

	log.Info("AsyncAPI document registered: %s.%s v%s (globalId=%d)",
		groupID, artifactID, metadata.Version, metadata.GlobalID)

	return output.PrintRegistrationResult(regOutput, format, outputPath)
}

func runAsyncAPIGet(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()

	// Get flags
	version, _ := cmd.Flags().GetString("version")
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")
	noFix, _ := cmd.Flags().GetBool("no-fix")

	log.Info("Loading configuration")
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check API version - AsyncAPI requires V3
	if cfg.APIVersion != config.APIVersionV3 {
		return fmt.Errorf("AsyncAPI get requires API version V3, current: %s. Use --api-version v3", cfg.APIVersion)
	}

	// Get group and artifact ID from flags (required)
	groupID := cfg.Group
	artifactID := cfg.ArtifactID

	if groupID == "" {
		return fmt.Errorf("--group is required")
	}
	if artifactID == "" {
		return fmt.Errorf("--artifact-id is required")
	}

	log.Info("Creating registry client (API: %s)", cfg.APIVersion)
	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultOperationTimeout)
	defer cancel()

	log.Info("Fetching AsyncAPI document: group=%s, artifactId=%s, version=%s", groupID, artifactID, version)
	content, err := client.GetArtifactContentDereferenced(ctx, groupID, artifactID, version, "DEREFERENCE")
	if err != nil {
		return fmt.Errorf("failed to get AsyncAPI document: %w", err)
	}

	// Parse the content as JSON
	var doc map[string]interface{}
	err = json.Unmarshal(content, &doc)
	if err != nil {
		return fmt.Errorf("failed to parse dereferenced content: %w", err)
	}

	// Fix broken Avro schema wrapping if needed (unless --no-fix is set)
	if !noFix {
		fixed := asyncapi.FixAvroSchemas(doc)
		if fixed {
			log.Info("Fixed Avro schema wrapping in components.schemas")
		}
	}

	// Output in requested format
	var outputContent []byte
	switch format {
	case "json":
		outputContent, err = json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
	case "yaml":
		outputContent, err = yaml.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s (use json or yaml)", format)
	}

	// Write output
	if outputPath != "" {
		if err := os.WriteFile(outputPath, outputContent, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		log.Info("AsyncAPI document written to: %s", outputPath)
	} else {
		fmt.Println(string(outputContent))
	}

	return nil
}
