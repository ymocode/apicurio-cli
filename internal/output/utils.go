package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ymocode/apicurio-client/internal/logger"
)

// TruncateString truncates a string to maxLen characters
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// WriteOutput writes output to file or stdout
func WriteOutput(content string, outputPath string) error {
	if outputPath == "" || outputPath == "-" {
		// Write to stdout
		fmt.Println(content)
		return nil
	}

	// Write to file
	log := logger.GetLogger()
	log.Info("Writing output to file: %s", outputPath)

	err := os.WriteFile(outputPath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	log.Info("Output written successfully: %s (%d bytes)", outputPath, len(content))
	return nil
}

// WriteJSONOutput writes JSON output to file or stdout
func WriteJSONOutput(data interface{}, outputPath string) error {
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return WriteOutput(string(output), outputPath)
}

// BoolToWord converts boolean to "compatible"/"incompatible"
func BoolToWord(b bool) string {
	if b {
		return "compatible"
	}
	return "incompatible"
}
