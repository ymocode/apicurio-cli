package asyncapi

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/ymocode/apicurio-client/internal/registry"
)

// ValidationResult represents the result of validating an AsyncAPI document
type ValidationResult struct {
	CurrentVersion  string
	ProposedVersion string
	Errors          []string
	Warnings        []string
	MissingRefs     []string
	Valid           bool
	IsNew           bool
	IsIdentical     bool
}

// Validate validates an AsyncAPI document against the registry
// Checks: version discipline, referenced schemas exist
func Validate(ctx context.Context, client registry.RegistryClient, spec *AsyncAPIDocument, groupID, artifactID string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:           true,
		ProposedVersion: spec.Version,
	}

	// Check if artifact exists in registry
	existingMeta, err := client.GetArtifactMetadata(ctx, groupID, artifactID)
	if err != nil {
		if registry.IsNotFoundError(err) {
			// New artifact - no version discipline check needed
			result.IsNew = true
		} else {
			return nil, fmt.Errorf("failed to check existing artifact: %w", err)
		}
	}

	// If artifact exists, check version discipline
	if !result.IsNew && existingMeta != nil {
		result.CurrentVersion = existingMeta.Version

		// Check if content is identical
		existingContent, err := client.GetArtifactContent(ctx, groupID, artifactID)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing content: %w", err)
		}

		if isContentIdentical(spec.Content, existingContent) {
			result.IsIdentical = true
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Content is identical to existing version %s", existingMeta.Version))
		} else {
			// Content changed - version must be higher
			if err := checkVersionBump(existingMeta.Version, spec.Version); err != nil {
				result.Valid = false
				result.Errors = append(result.Errors, err.Error())
			}
		}
	}

	// Check if referenced schemas exist in registry
	missingRefs := checkReferencesExist(ctx, client, spec.References)
	if len(missingRefs) > 0 {
		result.MissingRefs = missingRefs
		result.Valid = false
		for _, ref := range missingRefs {
			result.Errors = append(result.Errors,
				fmt.Sprintf("Referenced schema not found: %s", ref))
		}
	}

	return result, nil
}

// checkVersionBump verifies that proposedVersion is greater than currentVersion
func checkVersionBump(currentVersion, proposedVersion string) error {
	current, err := semver.NewVersion(currentVersion)
	if err != nil {
		// Current version is not valid semver, allow any new version
		return nil
	}

	proposed, err := semver.NewVersion(proposedVersion)
	if err != nil {
		return fmt.Errorf("proposed version %q is not valid semver", proposedVersion)
	}

	if !proposed.GreaterThan(current) {
		return fmt.Errorf("content changed but version not bumped: current=%s, proposed=%s (must be higher)",
			currentVersion, proposedVersion)
	}

	return nil
}

// isContentIdentical compares content by hash
func isContentIdentical(newContent []byte, existingContent interface{}) bool {
	// Convert existing content to bytes for comparison
	var existingBytes []byte
	switch v := existingContent.(type) {
	case string:
		existingBytes = []byte(v)
	case []byte:
		existingBytes = v
	default:
		return false
	}

	// Compare by SHA256 hash
	newHash := sha256.Sum256(newContent)
	existingHash := sha256.Sum256(existingBytes)

	return newHash == existingHash
}

// checkReferencesExist verifies that all referenced schemas exist in the registry
func checkReferencesExist(ctx context.Context, client registry.RegistryClient, refs []registry.ArtifactReference) []string {
	var missing []string

	for _, ref := range refs {
		_, err := client.GetArtifactMetadata(ctx, ref.GroupID, ref.ArtifactID)
		if err != nil {
			if registry.IsNotFoundError(err) {
				missing = append(missing, ref.Name)
			}
			// Other errors are ignored (might be transient)
		}
	}

	return missing
}
