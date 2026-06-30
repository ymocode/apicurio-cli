// Package operations provides core business logic for schema operations.
package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hamba/avro/v2"
	"github.com/ymocode/apicurio-client/internal/batch"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
	"github.com/ymocode/apicurio-client/internal/registry"
	"github.com/ymocode/apicurio-client/internal/schema"
)

// RegistrationResult represents the result of registering a schema
type RegistrationResult struct {
	Error         error
	Version       string
	CreatedOn     string
	GlobalID      int64
	ContentID     int64
	Success       bool
	IsNewArtifact bool
	LabelsApplied map[string]string // labels written to the created version
	LabelsSkipped bool              // labels requested but version was unchanged
}

// InternalValidationResult represents the result of validating a schema
type InternalValidationResult struct {
	CurrentVersion    string
	ExpectedVersion   string
	CurrentNamespace  string
	ProposedNamespace string
	ExpectedNamespace string
	CompatibilityType schema.CompatibilityType
	ChangeLevel       string
	Differences       []string
	ValidationErrors  []string
	Success           bool
	IsNew             bool
	IsCompatible      bool
}

// RegisterSchema is the core registration logic shared by single and batch operations.
// Labels, when non-empty, are applied only to a newly created version; an unchanged
// schema that resolves to an existing version keeps its labels untouched.
func RegisterSchema(ctx context.Context, client registry.RegistryClient, cfg *config.Config, s *schema.AvroSchema, skipValidation bool, labels map[string]string) RegistrationResult {
	result := RegistrationResult{Success: false}

	group := cfg.GetGroup(s.Namespace)
	artifactID := cfg.GetArtifactID(s.Name)

	// Validate first (unless skipped)
	var validationResult InternalValidationResult
	if !skipValidation {
		validationResult = ValidateSchema(ctx, client, cfg, s)
		if !validationResult.Success || !validationResult.IsCompatible {
			errors := strings.Join(validationResult.ValidationErrors, "; ")
			result.Error = fmt.Errorf("validation failed: %s", errors)
			return result
		}

		// If schema is identical to what's in registry, just return existing metadata
		if validationResult.CompatibilityType == schema.CompatibilityIdentical && !validationResult.IsNew {
			// Get existing metadata
			existingMetadata, err := client.GetArtifactMetadata(ctx, group, artifactID)
			if err != nil {
				result.Error = fmt.Errorf("failed to get existing metadata: %w", err)
				return result
			}
			result.Success = true
			result.IsNewArtifact = false
			result.Version = existingMetadata.Version
			result.GlobalID = existingMetadata.GlobalID
			result.ContentID = existingMetadata.ContentID
			result.CreatedOn = existingMetadata.CreatedOn
			result.LabelsSkipped = len(labels) > 0
			return result
		}
	}

	// Try to create artifact first (V3 with FIND_OR_CREATE_VERSION will handle deduplication)
	metadata, err := client.CreateArtifact(ctx, group, artifactID, s, labels)
	if err != nil {
		// If artifact exists (409), try creating a new version
		// Note: V3 with FIND_OR_CREATE_VERSION won't reach here for duplicates
		if isConflictError(err) {
			var versionMetadata *registry.VersionMetadata
			versionMetadata, err = client.CreateVersion(ctx, group, artifactID, s, labels)
			if err != nil {
				result.Error = fmt.Errorf("failed to create new version: %w", err)
				return result
			}
			result.Success = true
			result.IsNewArtifact = false
			result.Version = versionMetadata.Version
			result.GlobalID = versionMetadata.GlobalID
			result.ContentID = versionMetadata.ContentID
			result.CreatedOn = versionMetadata.CreatedOn
			result.LabelsApplied = labels
			return result
		}
		result.Error = fmt.Errorf("failed to register artifact: %w", err)
		return result
	}

	result.Success = true
	result.IsNewArtifact = true
	result.Version = metadata.Version
	result.GlobalID = metadata.GlobalID
	result.ContentID = metadata.ContentID
	result.CreatedOn = metadata.CreatedOn
	result.LabelsApplied = labels
	return result
}

// ValidateSchema is the core validation logic shared by single and batch operations
func ValidateSchema(ctx context.Context, client registry.RegistryClient, cfg *config.Config, s *schema.AvroSchema) InternalValidationResult {
	result := InternalValidationResult{
		Success:          false,
		IsNew:            false,
		ValidationErrors: []string{},
		Differences:      []string{},
	}

	group := cfg.GetGroup(s.Namespace)
	artifactID := cfg.GetArtifactID(s.Name)

	// Get current version from registry
	currentVersion, currentSchema, err := getCurrentVersion(ctx, client, group, artifactID)
	if err != nil {
		if isNotFoundError(err) {
			// This is a new schema - derive expected version from namespace
			// e.g., com.example.v1.User → 1.0.0, com.example.v2.User → 2.0.0
			expectedVersion := config.InitialVersion
			if nsVer := extractNamespaceVersion(s.Namespace); nsVer != "" && nsVer != "v1" {
				var num int
				_, _ = fmt.Sscanf(nsVer[1:], "%d", &num)
				if num > 1 {
					expectedVersion = fmt.Sprintf("%d.0.0", num)
				}
			}

			result.Success = true
			result.IsNew = true
			result.CurrentVersion = "none"
			result.ExpectedVersion = expectedVersion
			result.IsCompatible = true
			result.CompatibilityType = schema.CompatibilityInitial
			result.ChangeLevel = config.ChangeLevelInitial
			result.Differences = []string{"Initial schema registration"}

			// Validate that new schema has correct version for its namespace
			if s.Version != expectedVersion {
				result.IsCompatible = false
				result.ValidationErrors = append(result.ValidationErrors,
					fmt.Sprintf("New schema must have version %s, got %s", expectedVersion, s.Version))
			}
			return result
		}
		result.ValidationErrors = append(result.ValidationErrors, fmt.Sprintf("Failed to get current version: %v", err))
		return result
	}

	result.CurrentVersion = currentVersion

	// STEP 1: Get differences from local comparison (for user info only)
	// This does NOT determine compatibility - only extracts what changed
	suggestedChangeLevel, differences := schema.CompareSchemas(currentSchema, s)
	result.Differences = differences

	// STEP 2: Test compatibility with REGISTRY - this is the ONLY source of truth!
	// CRITICAL: We MUST trust the registry's compatibility decision to avoid corrupting it
	// The registry knows its own compatibility settings (BACKWARD, FORWARD, FULL, etc.)
	// Our local diff is ONLY for showing what changed, NOT for determining compatibility
	content, err := s.GetContentString()
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, fmt.Sprintf("Failed to get content: %v", err))
		return result
	}

	isCompatible, compatErr := client.TestCompatibility(ctx, group, artifactID, content, s.Version)

	// STEP 3: Determine final compatibility based SOLELY on registry result
	// If registry says incompatible → ALWAYS breaking/major
	// If registry says compatible → NEVER breaking (trust registry)
	var changeLevel string
	if compatErr != nil {
		// Registry returned error - this is INCOMPATIBLE (breaking change)
		result.IsCompatible = false
		result.CompatibilityType = schema.CompatibilityBreaking
		result.ChangeLevel = config.ChangeLevelMajor
		changeLevel = config.ChangeLevelMajor

		// Add registry error to validation errors
		result.ValidationErrors = append(result.ValidationErrors, fmt.Sprintf("Compatibility check failed: %v", compatErr))
	} else if !isCompatible {
		// Registry says INCOMPATIBLE - always breaking/major
		result.IsCompatible = false
		result.CompatibilityType = schema.CompatibilityBreaking
		result.ChangeLevel = config.ChangeLevelMajor
		changeLevel = config.ChangeLevelMajor
	} else {
		// Registry says COMPATIBLE - trust the registry!
		result.IsCompatible = true
		result.CompatibilityType = schema.CompatibilityBackward

		// Use suggested change level from diff analysis (patch/minor)
		// But NEVER major since registry said compatible
		if suggestedChangeLevel == config.ChangeLevelMajor {
			// Registry says compatible but our diff suggests major changes
			// Trust registry - downgrade to minor
			changeLevel = config.ChangeLevelMinor
		} else if suggestedChangeLevel == "" {
			// Identical schemas
			result.CompatibilityType = schema.CompatibilityIdentical
			changeLevel = ""
		} else {
			// Use suggested level (patch or minor)
			changeLevel = suggestedChangeLevel
		}
		result.ChangeLevel = changeLevel
	}

	// STEP 3: Calculate expected version based on change level
	expectedVersion, err := schema.CalculateExpectedVersionFromLevel(currentVersion, changeLevel)
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, fmt.Sprintf("Failed to calculate expected version: %v", err))
		result.Success = true
		return result
	}
	result.ExpectedVersion = expectedVersion

	// STEP 4: For major changes, check if namespace version needs to be bumped
	if changeLevel == config.ChangeLevelMajor {
		currentNamespaceVersion := extractNamespaceVersion(currentSchema.Namespace)
		proposedNamespaceVersion := extractNamespaceVersion(s.Namespace)
		expectedNamespaceVersion := incrementNamespaceVersion(currentNamespaceVersion)

		// Populate namespace fields for reporting
		result.CurrentNamespace = currentSchema.Namespace
		result.ProposedNamespace = s.Namespace

		if currentNamespaceVersion != "" {
			// Build expected full namespace
			result.ExpectedNamespace = strings.Replace(currentSchema.Namespace, currentNamespaceVersion, expectedNamespaceVersion, 1)

			// Check if namespace version was actually bumped
			if proposedNamespaceVersion == currentNamespaceVersion {
				result.ValidationErrors = append(result.ValidationErrors,
					fmt.Sprintf("Major version change requires namespace version bump: expected %s, got %s",
						result.ExpectedNamespace, s.Namespace))
			}
		}
	}

	// STEP 5: Check version mismatch (but not for identical schemas)
	if changeLevel != "" && s.Version != expectedVersion {
		result.ValidationErrors = append(result.ValidationErrors,
			fmt.Sprintf("Version mismatch: expected %s for %s change, got %s",
				expectedVersion, changeLevel, s.Version))
	}

	result.IsCompatible = result.IsCompatible && len(result.ValidationErrors) == 0
	result.Success = true
	return result
}

// getCurrentVersion gets the current version and schema from the registry
func getCurrentVersion(ctx context.Context, client registry.RegistryClient, group, artifactID string) (string, *schema.AvroSchema, error) {
	metadata, err := client.GetArtifactMetadata(ctx, group, artifactID)
	if err != nil {
		return "", nil, err
	}

	version := metadata.Version

	contentResp, err := client.GetArtifactContent(ctx, group, artifactID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get artifact content: %w", err)
	}

	currentSchema, err := parseSchemaContent(contentResp, version)
	if err != nil {
		return "", nil, err
	}

	if currentSchema.Version == "" {
		currentSchema.Version = version
	}

	return currentSchema.Version, currentSchema, nil
}

// parseSchemaContent parses schema content returned from registry
func parseSchemaContent(contentResp interface{}, version string) (*schema.AvroSchema, error) {
	var contentBytes []byte
	var err error

	// Handle different content types
	switch content := contentResp.(type) {
	case string:
		contentBytes = []byte(content)
	case []byte:
		contentBytes = content
	default:
		contentBytes, err = json.Marshal(contentResp)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content: %w", err)
		}
	}

	// Parse raw JSON to extract fields
	var rawData map[string]interface{}
	err = json.Unmarshal(contentBytes, &rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse raw data: %w", err)
	}

	// Extract custom fields (namespace, name, version)
	namespace, _ := rawData["namespace"].(string)
	name, _ := rawData["name"].(string)
	schemaVersion, _ := rawData["version"].(string)

	// If version not in schema, use provided version
	if schemaVersion == "" {
		schemaVersion = version
		rawData["version"] = version
	}

	// Parse using official Avro SDK
	avroSchema, err := avro.Parse(string(contentBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Avro schema: %w", err)
	}

	return &schema.AvroSchema{
		Version:   schemaVersion,
		Namespace: namespace,
		Name:      name,
		Schema:    avroSchema,
		RawData:   rawData,
	}, nil
}

// isConflictError checks if error is a conflict (409) error
func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "409") ||
		strings.Contains(errStr, "conflict") ||
		strings.Contains(errStr, "already exists")
}

// isNotFoundError checks if error is a not found (404) error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "http 404")
}

// extractNamespaceVersion extracts version from namespace (e.g., "com.acme.avro.ph.policy.v1" → "v1")
func extractNamespaceVersion(namespace string) string {
	parts := strings.Split(namespace, ".")
	if len(parts) == 0 {
		return ""
	}
	lastPart := parts[len(parts)-1]
	// Check if last part is a version like v1, v2, etc.
	if strings.HasPrefix(lastPart, "v") && len(lastPart) > 1 {
		return lastPart
	}
	return ""
}

// incrementNamespaceVersion increments namespace version (e.g., "v1" → "v2")
func incrementNamespaceVersion(version string) string {
	if version == "" || !strings.HasPrefix(version, "v") {
		return version
	}

	// Extract number from version string
	numStr := version[1:]
	var num int
	_, _ = fmt.Sscanf(numStr, "%d", &num)

	// Increment and return
	return fmt.Sprintf("v%d", num+1)
}

// ProcessSingleValidation processes a single schema file and returns a BatchResult
// This is the SINGLE source of truth for validation - used by both single file and batch operations
func ProcessSingleValidation(ctx context.Context, client registry.RegistryClient, cfg *config.Config, filePath string) batch.BatchResult {
	log := logger.GetLogger()

	result := batch.BatchResult{
		FilePath: filePath,
		Action:   config.ActionValidated,
		Status:   config.StatusSkipped,
	}

	// Parse schema
	log.Debug("Parsing schema: %s", filePath)
	s, err := schema.ParseAvroSchema(filePath)
	if err != nil {
		log.Error("Failed to parse schema %s: %v", filePath, err)
		result.Status = config.StatusFailed
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to parse schema: %v", err))
		result.Message = "Parse error"
		return result
	}

	result.Namespace = s.Namespace
	result.Name = s.Name
	result.Version = s.Version

	// Get FQN
	fqn := cfg.GetFQN(s.Namespace, s.Name)

	// Use the shared validation function
	log.Debug("Running validation for %s: version=%s", fqn, s.Version)
	valResult := ValidateSchema(ctx, client, cfg, s)
	if !valResult.Success {
		log.Error("Validation failed (internal error)")
		result.Status = config.StatusFailed
		result.Errors = valResult.ValidationErrors
		result.Message = "Validation failed"
	} else {
		result.CompatibilityType = string(valResult.CompatibilityType)
		result.ChangeLevel = valResult.ChangeLevel

		// Determine final status
		if !valResult.IsCompatible {
			log.Debug("Validation complete: incompatible (%d errors)", len(valResult.ValidationErrors))
			result.Status = config.StatusFailed
			result.Errors = valResult.ValidationErrors
			result.Message = fmt.Sprintf("Incompatible (%d error(s), %d change(s))", len(valResult.ValidationErrors), len(valResult.Differences))
		} else {
			log.Debug("Validation complete: compatible (%s)", valResult.CompatibilityType)
			result.Status = config.StatusSuccess
			if valResult.IsNew {
				result.Message = "New schema (will create initial version)"
			} else {
				result.Message = fmt.Sprintf("Compatible (%s change, %d difference(s))", valResult.CompatibilityType, len(valResult.Differences))
			}
		}
	}

	// Store full validation result for detailed output
	result.ValidationResult = &schema.ValidationResult{
		FQN:               fqn,
		CurrentVersion:    valResult.CurrentVersion,
		ProposedVersion:   s.Version,
		ExpectedVersion:   valResult.ExpectedVersion,
		CurrentNamespace:  valResult.CurrentNamespace,
		ProposedNamespace: valResult.ProposedNamespace,
		ExpectedNamespace: valResult.ExpectedNamespace,
		IsCompatible:      valResult.IsCompatible,
		CompatibilityType: valResult.CompatibilityType,
		ChangeLevel:       valResult.ChangeLevel,
		Differences:       valResult.Differences,
		ValidationErrors:  valResult.ValidationErrors,
	}

	return result
}
