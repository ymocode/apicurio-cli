// Package output provides formatters for CLI output in various formats.
package output

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/schema"
	"github.com/ymocode/apicurio-client/internal/templates"
)

// RegistrationOutput represents registration result output
type RegistrationOutput struct {
	Action        string `json:"action"`
	FQN           string `json:"fqn"`
	Group         string `json:"group"`
	ArtifactID    string `json:"artifact_id"`
	Version       string `json:"version"`
	CreatedOn     string `json:"created_on"`
	GlobalID      int64  `json:"global_id"`
	ContentID     int64  `json:"content_id"`
	DurationMs    int64  `json:"duration_ms"`
	Success       bool   `json:"success"`
	IsNewArtifact bool   `json:"-"`
}

// DryRunOutput represents dry-run result output
type DryRunOutput struct {
	FQN        string   `json:"fqn"`
	Group      string   `json:"group"`
	ArtifactID string   `json:"artifact_id"`
	Version    string   `json:"version"`
	Errors     []string `json:"errors,omitempty"`
	DurationMs int64    `json:"duration_ms"`
	DryRun     bool     `json:"dry_run"`
	Success    bool     `json:"success"`
}

// SystemInfoOutput represents system information output
type SystemInfoOutput struct {
	RegistryURL    string `json:"registry_url"`
	APIVersion     string `json:"api_version"`
	Name           string `json:"name"`
	Version        string `json:"version"`
	Description    string `json:"description"`
	BuiltOn        string `json:"built_on"`
	ResponseTimeMs int64  `json:"response_time_ms"`
}

// PrintRegistrationResult prints registration result in specified format
func PrintRegistrationResult(output RegistrationOutput, format string, outputPath string) error {
	var content string

	switch format {
	case config.FormatTable:
		content = formatRegistrationTable(output)
	case config.FormatSummary:
		content = formatRegistrationSummary(output)
	case config.FormatMarkdown:
		var buf bytes.Buffer
		if err := templates.RegistrationMarkdown.Execute(&buf, output); err != nil {
			return fmt.Errorf("failed to render markdown template: %w", err)
		}
		content = buf.String()
	case config.FormatJSON:
		fallthrough
	default:
		return WriteJSONOutput(output, outputPath)
	}

	return WriteOutput(content, outputPath)
}

func formatRegistrationTable(output RegistrationOutput) string {
	actionLabel := "Created Artifact"
	switch output.Action {
	case "created_version":
		actionLabel = "Created Version"
	case "updated":
		actionLabel = "Updated Version"
	case "unchanged":
		actionLabel = "Unchanged (identical)"
	}

	var sb strings.Builder
	sb.WriteString("==================================================\n")
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Status:", "✓ SUCCESS"))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Action:", actionLabel))
	sb.WriteString("--------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "FQN:", output.FQN))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Group:", output.Group))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Artifact ID:", output.ArtifactID))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Version:", output.Version))

	// Only show IDs if they are set (not for unchanged)
	if output.GlobalID > 0 {
		sb.WriteString(fmt.Sprintf("%-15s %d\n", "Global ID:", output.GlobalID))
	}
	if output.ContentID > 0 {
		sb.WriteString(fmt.Sprintf("%-15s %d\n", "Content ID:", output.ContentID))
	}
	if output.CreatedOn != "" {
		sb.WriteString(fmt.Sprintf("%-15s %s\n", "Created On:", output.CreatedOn))
	}
	sb.WriteString(fmt.Sprintf("%-15s %dms\n", "Duration:", output.DurationMs))
	sb.WriteString("==================================================")
	return sb.String()
}

func formatRegistrationSummary(output RegistrationOutput) string {
	actionLabel := "Created artifact"
	switch output.Action {
	case "created_version":
		actionLabel = "Created version"
	case "updated":
		actionLabel = "Updated version"
	case "unchanged":
		return fmt.Sprintf("✓ Unchanged: %s v%s (identical content, %dms)",
			output.FQN, output.Version, output.DurationMs)
	}

	return fmt.Sprintf("✓ %s: %s v%s (global ID: %d, %dms)",
		actionLabel, output.FQN, output.Version, output.GlobalID, output.DurationMs)
}

// PrintDryRunResult prints dry-run result in specified format
func PrintDryRunResult(output DryRunOutput, format string, outputPath string) error {
	var content string

	switch format {
	case config.FormatTable:
		content = formatDryRunTable(output)
	case config.FormatSummary:
		content = formatDryRunSummary(output)
	case config.FormatMarkdown:
		// Map DryRunOutput to validation template structure
		validationData := struct {
			FQN        string
			Group      string
			ArtifactID string
			Version    string
			Errors     []string
			DurationMs int64
			Valid      bool
		}{
			Valid:      output.Success,
			FQN:        output.FQN,
			Group:      output.Group,
			ArtifactID: output.ArtifactID,
			Version:    output.Version,
			Errors:     output.Errors,
			DurationMs: output.DurationMs,
		}
		var buf bytes.Buffer
		if err := templates.ValidationMarkdown.Execute(&buf, validationData); err != nil {
			return fmt.Errorf("failed to render markdown template: %w", err)
		}
		content = buf.String()
	case config.FormatJSON:
		fallthrough
	default:
		return WriteJSONOutput(output, outputPath)
	}

	return WriteOutput(content, outputPath)
}

func formatDryRunTable(output DryRunOutput) string {
	status := "✓ SUCCESS"
	if !output.Success {
		status = "✗ FAILED"
	}

	var sb strings.Builder
	sb.WriteString("==================================================\n")
	sb.WriteString("DRY RUN MODE\n")
	sb.WriteString("--------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Status:", status))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "FQN:", output.FQN))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Group:", output.Group))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Artifact ID:", output.ArtifactID))
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "Version:", output.Version))
	sb.WriteString(fmt.Sprintf("%-15s %dms\n", "Duration:", output.DurationMs))

	if !output.Success && len(output.Errors) > 0 {
		sb.WriteString("--------------------------------------------------\n")
		sb.WriteString("Errors:\n")
		for _, err := range output.Errors {
			sb.WriteString(fmt.Sprintf("  ✗ %s\n", err))
		}
	}
	sb.WriteString("==================================================")
	return sb.String()
}

func formatDryRunSummary(output DryRunOutput) string {
	symbol := "✓"
	status := "Would register successfully"
	if !output.Success {
		symbol = "✗"
		status = "Would fail validation"
	}

	result := fmt.Sprintf("[DRY RUN] %s %s: %s v%s (%dms)",
		symbol, status, output.FQN, output.Version, output.DurationMs)

	if !output.Success && len(output.Errors) > 0 {
		for _, err := range output.Errors {
			result += fmt.Sprintf("\n  - %s", err)
		}
	}

	return result
}

// PrintSystemInfoResult prints system info result in specified format.
func PrintSystemInfoResult(output SystemInfoOutput, format string, outputPath string) error {
	var content string

	switch format {
	case config.FormatTable:
		content = formatSystemInfoTable(output)
	case config.FormatSummary:
		content = formatSystemInfoSummary(output)
	case config.FormatMarkdown:
		var buf bytes.Buffer
		if err := templates.SystemInfoMarkdown.Execute(&buf, output); err != nil {
			return fmt.Errorf("failed to render markdown template: %w", err)
		}
		content = buf.String()
	case config.FormatJSON:
		fallthrough
	default:
		return WriteJSONOutput(output, outputPath)
	}

	return WriteOutput(content, outputPath)
}

func formatSystemInfoTable(output SystemInfoOutput) string {
	var sb strings.Builder
	sb.WriteString("==================================================\n")
	sb.WriteString("REGISTRY SYSTEM INFORMATION\n")
	sb.WriteString("==================================================\n")
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "Name:", output.Name))
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "Version:", output.Version))
	if output.Description != "" {
		sb.WriteString(fmt.Sprintf("%-20s %s\n", "Description:", output.Description))
	}
	if output.BuiltOn != "" {
		sb.WriteString(fmt.Sprintf("%-20s %s\n", "Built On:", output.BuiltOn))
	}
	sb.WriteString("--------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "Registry URL:", output.RegistryURL))
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "API Version:", output.APIVersion))
	sb.WriteString(fmt.Sprintf("%-20s %dms\n", "Response Time:", output.ResponseTimeMs))
	sb.WriteString("==================================================")
	return sb.String()
}

func formatSystemInfoSummary(output SystemInfoOutput) string {
	return fmt.Sprintf("%s v%s (%s, %dms)", output.Name, output.Version, output.RegistryURL, output.ResponseTimeMs)
}

// PrintValidationResult prints validation result in specified format
func PrintValidationResult(result schema.ValidationResult, format string, outputPath string) error {
	switch format {
	case config.FormatTable:
		content, _ := FormatValidationTable(result)
		return WriteOutput(content, outputPath)
	case config.FormatSummary:
		content, _ := FormatValidationSummary(result)
		return WriteOutput(content, outputPath)
	case config.FormatMarkdown:
		// Extract artifact ID from FQN (last part)
		fqnParts := strings.Split(result.FQN, ".")
		artifactID := result.FQN
		if len(fqnParts) > 0 {
			artifactID = fqnParts[len(fqnParts)-1]
		}

		// Extract group from FQN
		// Group is always the namespace unless explicitly set via --group flag
		// FQN format: namespace.ArtifactID, so group is everything except last part
		group := result.CurrentNamespace
		if group == "" && len(fqnParts) > 1 {
			// Extract namespace from FQN (e.g., "com.example.TestSchema" → "com.example")
			group = strings.Join(fqnParts[:len(fqnParts)-1], ".")
		}

		// Map ValidationResult to template structure with ALL fields from JSON
		validationData := struct {
			FQN               string
			Group             string
			ArtifactID        string
			CurrentVersion    string
			ProposedVersion   string
			ExpectedVersion   string
			CompatibilityType string
			Differences       []string
			ValidationErrors  []string
			DurationMs        int64
			Valid             bool
		}{
			Valid:             result.IsCompatible,
			FQN:               result.FQN,
			Group:             group,
			ArtifactID:        artifactID,
			CurrentVersion:    result.CurrentVersion,
			ProposedVersion:   result.ProposedVersion,
			ExpectedVersion:   result.ExpectedVersion,
			CompatibilityType: string(result.CompatibilityType),
			Differences:       result.Differences,
			ValidationErrors:  result.ValidationErrors,
			DurationMs:        0, // Not tracked in ValidationResult
		}
		var buf bytes.Buffer
		if err := templates.ValidationMarkdown.Execute(&buf, validationData); err != nil {
			return fmt.Errorf("failed to render markdown template: %w", err)
		}
		return WriteOutput(buf.String(), outputPath)
	case config.FormatJSON:
		fallthrough
	default:
		return WriteJSONOutput(result, outputPath)
	}
}

func FormatValidationTable(result schema.ValidationResult) (string, error) {
	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "FQN:", result.FQN))
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "Current Version:", result.CurrentVersion))
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "Proposed Version:", result.ProposedVersion))
	sb.WriteString(fmt.Sprintf("%-20s %s\n", "Expected Version:", result.ExpectedVersion))

	// Show namespace info for major changes
	if result.ExpectedNamespace != "" {
		sb.WriteString(fmt.Sprintf("%-20s %s\n", "Current Namespace:", result.CurrentNamespace))
		sb.WriteString(fmt.Sprintf("%-20s %s\n", "Proposed Namespace:", result.ProposedNamespace))
		sb.WriteString(fmt.Sprintf("%-20s %s\n", "Expected Namespace:", result.ExpectedNamespace))
	}

	sb.WriteString(fmt.Sprintf("%-20s %v\n", "Compatible:", result.IsCompatible))
	sb.WriteString(fmt.Sprintf("%-20s %v\n", "Compatibility Type:", result.CompatibilityType))
	if result.ChangeLevel != "" {
		sb.WriteString(fmt.Sprintf("%-20s %v\n", "Change Level:", result.ChangeLevel))
	}
	sb.WriteString(strings.Repeat("-", 80) + "\n")

	if len(result.Differences) > 0 {
		sb.WriteString("Differences:\n")
		for _, diff := range result.Differences {
			sb.WriteString(fmt.Sprintf("  %s\n", diff))
		}
		sb.WriteString(strings.Repeat("-", 80) + "\n")
	}

	if len(result.ValidationErrors) > 0 {
		sb.WriteString("Validation Errors:\n")
		for _, err := range result.ValidationErrors {
			sb.WriteString(fmt.Sprintf("  ✗ %s\n", err))
		}
		sb.WriteString(strings.Repeat("-", 80) + "\n")
	}

	status := "✓ PASS"
	if !result.IsCompatible {
		status = "✗ FAIL"
	}
	sb.WriteString(fmt.Sprintf("Status: %s\n", status))
	sb.WriteString(strings.Repeat("=", 80))
	return sb.String(), nil
}

func FormatValidationSummary(result schema.ValidationResult) (string, error) {
	status := "PASS"
	symbol := "✓"
	if !result.IsCompatible {
		status = "FAIL"
		symbol = "✗"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s - %s\n", symbol, status, result.FQN))
	sb.WriteString(fmt.Sprintf("  Version: %s → %s (expected: %s)\n",
		result.CurrentVersion, result.ProposedVersion, result.ExpectedVersion))

	if result.ChangeLevel != "" {
		sb.WriteString(fmt.Sprintf("  Compatibility: %v - %s (%s)\n",
			result.CompatibilityType, result.ChangeLevel, BoolToWord(result.IsCompatible)))
	} else {
		sb.WriteString(fmt.Sprintf("  Compatibility: %v (%s)\n",
			result.CompatibilityType, BoolToWord(result.IsCompatible)))
	}

	if len(result.Differences) > 0 {
		sb.WriteString(fmt.Sprintf("  Differences: %d\n", len(result.Differences)))
		for _, diff := range result.Differences {
			sb.WriteString(fmt.Sprintf("    %s\n", diff))
		}
	}

	if len(result.ValidationErrors) > 0 {
		sb.WriteString(fmt.Sprintf("  Errors: %d\n", len(result.ValidationErrors)))
		for _, err := range result.ValidationErrors {
			sb.WriteString(fmt.Sprintf("    - %s\n", err))
		}
	}

	return strings.TrimSuffix(sb.String(), "\n"), nil
}

// AsyncAPIValidationOutput represents AsyncAPI validation result output
type AsyncAPIValidationOutput struct {
	Group           string   `json:"group"`
	ArtifactID      string   `json:"artifact_id"`
	ProposedVersion string   `json:"proposed_version"`
	CurrentVersion  string   `json:"current_version,omitempty"`
	Errors          []string `json:"errors,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	MissingRefs     []string `json:"missing_refs,omitempty"`
	Valid           bool     `json:"valid"`
	IsNew           bool     `json:"is_new"`
	IsIdentical     bool     `json:"is_identical"`
}

// PrintAsyncAPIValidationResult prints AsyncAPI validation result in specified format
func PrintAsyncAPIValidationResult(output AsyncAPIValidationOutput, format string, outputPath string) error {
	var content string

	switch format {
	case config.FormatTable:
		content = formatAsyncAPIValidationTable(output)
	case config.FormatSummary:
		content = formatAsyncAPIValidationSummary(output)
	case config.FormatJSON:
		fallthrough
	default:
		return WriteJSONOutput(output, outputPath)
	}

	return WriteOutput(content, outputPath)
}

func formatAsyncAPIValidationTable(output AsyncAPIValidationOutput) string {
	status := "✓ VALID"
	if !output.Valid {
		status = "✗ INVALID"
	}

	var sb strings.Builder
	sb.WriteString("==================================================\n")
	sb.WriteString("ASYNCAPI VALIDATION RESULT\n")
	sb.WriteString("==================================================\n")
	sb.WriteString(fmt.Sprintf("%-18s %s\n", "Status:", status))
	sb.WriteString(fmt.Sprintf("%-18s %s\n", "Group:", output.Group))
	sb.WriteString(fmt.Sprintf("%-18s %s\n", "Artifact ID:", output.ArtifactID))
	sb.WriteString(fmt.Sprintf("%-18s %s\n", "Proposed Version:", output.ProposedVersion))

	if output.IsNew {
		sb.WriteString(fmt.Sprintf("%-18s %s\n", "State:", "New artifact"))
	} else {
		sb.WriteString(fmt.Sprintf("%-18s %s\n", "Current Version:", output.CurrentVersion))
		if output.IsIdentical {
			sb.WriteString(fmt.Sprintf("%-18s %s\n", "State:", "Content identical"))
		} else {
			sb.WriteString(fmt.Sprintf("%-18s %s\n", "State:", "Content changed"))
		}
	}

	if len(output.Errors) > 0 {
		sb.WriteString("--------------------------------------------------\n")
		sb.WriteString("Errors:\n")
		for _, err := range output.Errors {
			sb.WriteString(fmt.Sprintf("  ✗ %s\n", err))
		}
	}

	if len(output.Warnings) > 0 {
		sb.WriteString("--------------------------------------------------\n")
		sb.WriteString("Warnings:\n")
		for _, w := range output.Warnings {
			sb.WriteString(fmt.Sprintf("  ! %s\n", w))
		}
	}

	if len(output.MissingRefs) > 0 {
		sb.WriteString("--------------------------------------------------\n")
		sb.WriteString("Missing References:\n")
		for _, ref := range output.MissingRefs {
			sb.WriteString(fmt.Sprintf("  - %s\n", ref))
		}
	}

	sb.WriteString("==================================================")
	return sb.String()
}

func formatAsyncAPIValidationSummary(output AsyncAPIValidationOutput) string {
	symbol := "✓"
	status := "Valid"
	if !output.Valid {
		symbol = "✗"
		status = "Invalid"
	}

	fqn := fmt.Sprintf("%s.%s", output.Group, output.ArtifactID)
	result := fmt.Sprintf("%s %s: %s v%s", symbol, status, fqn, output.ProposedVersion)

	if output.IsNew {
		result += " (new)"
	} else if output.IsIdentical {
		result += " (identical)"
	}

	if len(output.Errors) > 0 {
		for _, err := range output.Errors {
			result += fmt.Sprintf("\n  ✗ %s", err)
		}
	}

	return result
}
