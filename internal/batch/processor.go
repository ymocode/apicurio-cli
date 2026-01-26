package batch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/output"
	"github.com/ymocode/apicurio-client/internal/schema"
	"github.com/ymocode/apicurio-client/internal/templates"
)

// BatchResult represents the result of processing a single schema
type BatchResult struct {
	FilePath          string            `json:"file_path"`
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	Version           string            `json:"version"`
	Status            string            `json:"status"` // success, failed, skipped
	Action            string            `json:"action"` // validated, registered, etc.
	CompatibilityType string            `json:"compatibility_type,omitempty"`
	ChangeLevel       string                  `json:"change_level,omitempty"` // patch, minor, major
	Errors            []string                `json:"errors,omitempty"`
	Message           string                  `json:"message,omitempty"`
	ValidationResult  *schema.ValidationResult `json:"validation_result,omitempty"` // Full validation details
}

// BatchSummary represents the overall summary of batch processing
type BatchSummary struct {
	Total       int            `json:"total"`
	Success     int            `json:"success"`
	Failed      int            `json:"failed"`
	Skipped     int            `json:"skipped"`
	Results     []BatchResult  `json:"results"`
	FailedFiles []string       `json:"failed_files,omitempty"`
}

// BatchOptions contains options for batch processing
type BatchOptions struct {
	Directory       string
	Pattern         string
	Recursive       bool
	Parallel        int
	ContinueOnError bool
	DryRun          bool
}

// DiscoverSchemas finds all schema files in a directory
func DiscoverSchemas(dir string, pattern string, recursive bool) ([]string, error) {
	var schemas []string

	// Normalize pattern
	if pattern == "" {
		pattern = "*.avsc"
	}

	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to access directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	// Walk the directory
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// If not recursive and not the root directory, skip
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file matches pattern
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return err
		}

		if matched {
			schemas = append(schemas, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to discover schemas: %w", err)
	}

	return schemas, nil
}

// ProcessBatch processes multiple schemas in parallel
func ProcessBatch(ctx context.Context, files []string, opts BatchOptions, processor func(context.Context, string) BatchResult) *BatchSummary {
	summary := &BatchSummary{
		Total:   len(files),
		Results: make([]BatchResult, 0, len(files)),
	}

	// Set default parallelism
	if opts.Parallel <= 0 {
		opts.Parallel = 4
	}

	// Create channels for work distribution
	jobs := make(chan string, len(files))
	results := make(chan BatchResult, len(files))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < opts.Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				result := processor(ctx, file)
				results <- result
			}
		}()
	}

	// Send jobs
	go func() {
		for _, file := range files {
			jobs <- file
		}
		close(jobs)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Aggregate results
	for result := range results {
		summary.Results = append(summary.Results, result)

		switch result.Status {
		case config.StatusSuccess:
			summary.Success++
		case config.StatusFailed:
			summary.Failed++
			summary.FailedFiles = append(summary.FailedFiles, result.FilePath)
		case config.StatusSkipped:
			summary.Skipped++
		}
	}

	return summary
}

// FormatBatchSummary formats batch summary for display
func FormatBatchSummary(summary *BatchSummary, format string) (string, error) {
	switch format {
	case config.FormatJSON:
		return formatBatchJSON(summary)
	case config.FormatTable:
		return formatBatchTable(summary)
	case config.FormatSummary:
		return formatBatchSummaryText(summary)
	case config.FormatMarkdown:
		return formatBatchMarkdown(summary)
	default:
		return formatBatchSummaryText(summary)
	}
}

func formatBatchJSON(summary *BatchSummary) (string, error) {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format JSON: %w", err)
	}
	return string(data), nil
}

func formatBatchTable(summary *BatchSummary) (string, error) {
	var sb strings.Builder

	// Overall summary header
	sb.WriteString("Batch Processing Summary\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("Total: %d | Success: %d | Failed: %d | Skipped: %d\n",
		summary.Total, summary.Success, summary.Failed, summary.Skipped))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	// Detailed results for each schema
	if len(summary.Results) > 0 {
		for i, result := range summary.Results {
			if i > 0 {
				sb.WriteString("\n")
			}

			// Show validation details if available
			if result.ValidationResult != nil {
				validationOutput, _ := output.FormatValidationTable(*result.ValidationResult)
				sb.WriteString(validationOutput + "\n")
			} else {
				// Fallback for non-validation results (e.g., registration)
				sb.WriteString(fmt.Sprintf("File: %s\n", result.FilePath))
				statusIcon := getStatusIcon(result.Status)
				sb.WriteString(fmt.Sprintf("%s %s.%s (v%s) - %s\n",
					statusIcon, result.Namespace, result.Name, result.Version, result.Message))

				if len(result.Errors) > 0 {
					sb.WriteString("Errors:\n")
					for _, err := range result.Errors {
						sb.WriteString(fmt.Sprintf("  - %s\n", err))
					}
				}
			}
		}
	}

	return sb.String(), nil
}

func formatBatchSummaryText(summary *BatchSummary) (string, error) {
	var sb strings.Builder

	// Overall summary header
	sb.WriteString("Batch Processing Summary\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("Total schemas:    %d\n", summary.Total))
	sb.WriteString(fmt.Sprintf("Success:          %d\n", summary.Success))
	sb.WriteString(fmt.Sprintf("Failed:           %d\n", summary.Failed))
	sb.WriteString(fmt.Sprintf("Skipped:          %d\n", summary.Skipped))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	// Detailed results for each schema
	if len(summary.Results) > 0 {
		for i, result := range summary.Results {
			if i > 0 {
				sb.WriteString("\n")
			}

			// Show validation details if available
			if result.ValidationResult != nil {
				validationOutput, _ := output.FormatValidationSummary(*result.ValidationResult)
				sb.WriteString(validationOutput + "\n")
			} else {
				// Fallback for non-validation results (e.g., registration)
				statusIcon := getStatusIcon(result.Status)
				sb.WriteString(fmt.Sprintf("%s %s.%s (v%s) - %s\n",
					statusIcon, result.Namespace, result.Name, result.Version, result.Message))

				if len(result.Errors) > 0 {
					for _, err := range result.Errors {
						sb.WriteString(fmt.Sprintf("    - %s\n", err))
					}
				}
			}
		}
	}

	return sb.String(), nil
}

func formatBatchMarkdown(summary *BatchSummary) (string, error) {
	// Calculate additional metrics
	successRate := 0.0
	if summary.Total > 0 {
		successRate = (float64(summary.Success) / float64(summary.Total)) * 100
	}

	// Calculate durations (we don't have duration tracking yet, so we'll use 0)
	totalDurationMs := int64(0)
	avgDurationMs := 0.0

	// Determine action from first result
	action := "validated"
	if len(summary.Results) > 0 {
		action = summary.Results[0].Action
	}

	// Map results to template format - include ALL fields from BatchResult
	type TemplateResult struct {
		File             string
		FQN              string
		Namespace        string
		Name             string
		Version          string
		Status           string
		Action           string
		CompatibilityType string
		ChangeLevel      string
		Message          string
		Errors           []string
		ValidationResult *schema.ValidationResult
		DurationMs       int64
	}

	templateResults := make([]TemplateResult, len(summary.Results))
	for i, r := range summary.Results {
		fqn := fmt.Sprintf("%s.%s", r.Namespace, r.Name)

		templateResults[i] = TemplateResult{
			File:              r.FilePath,
			FQN:               fqn,
			Namespace:         r.Namespace,
			Name:              r.Name,
			Version:           r.Version,
			Status:            r.Status,
			Action:            r.Action,
			CompatibilityType: string(r.CompatibilityType),
			ChangeLevel:       r.ChangeLevel,
			Message:           r.Message,
			Errors:            r.Errors,
			ValidationResult:  r.ValidationResult,
			DurationMs:        0, // Not tracked yet
		}
	}

	// Create data structure for template
	data := struct {
		Action           string
		TotalFiles       int
		SuccessCount     int
		FailedCount      int
		SkippedCount     int
		SuccessRate      float64
		TotalDurationMs  int64
		AvgDurationMs    float64
		Results          []TemplateResult
	}{
		Action:           action,
		TotalFiles:       summary.Total,
		SuccessCount:     summary.Success,
		FailedCount:      summary.Failed,
		SkippedCount:     summary.Skipped,
		SuccessRate:      successRate,
		TotalDurationMs:  totalDurationMs,
		AvgDurationMs:    avgDurationMs,
		Results:          templateResults,
	}

	var buf bytes.Buffer
	if err := templates.BatchMarkdown.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render markdown template: %w", err)
	}

	return buf.String(), nil
}

func getStatusIcon(status string) string {
	switch status {
	case config.StatusSuccess:
		return "✓"
	case config.StatusFailed:
		return "✗"
	case config.StatusSkipped:
		return "⊘"
	default:
		return "?"
	}
}
