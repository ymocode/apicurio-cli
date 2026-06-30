package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/labels"
)

// addLabelFlags registers the repeatable --labels flag (with --label alias) on a command.
func addLabelFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("labels", nil, "Labels to attach to the created version as key=value (repeatable or comma-separated, e.g. --labels bundleVersion=1.2.0)")
	cmd.Flags().StringArray("label", nil, "Alias for --labels")
}

// parseLabelFlags reads and merges the --labels and --label values into a label map.
func parseLabelFlags(cmd *cobra.Command) (map[string]string, error) {
	raw, _ := cmd.Flags().GetStringArray("labels")
	alias, _ := cmd.Flags().GetStringArray("label")
	return labels.Parse(append(raw, alias...))
}

// guardLabelsAPIVersion rejects labels on API versions that do not support them.
func guardLabelsAPIVersion(cfg *config.Config, parsed map[string]string) error {
	if len(parsed) > 0 && cfg.APIVersion != config.APIVersionV3 {
		return fmt.Errorf("--labels requires --api-version v3 (got %s)", cfg.APIVersion)
	}
	return nil
}
