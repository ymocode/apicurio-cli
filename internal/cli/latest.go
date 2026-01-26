package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/registry"
)

var latestCmd = &cobra.Command{
	Use:   "latest",
	Short: "Get the latest version of a schema",
	Long:  `Retrieve the latest version metadata and content for an artifact from Apicurio Registry.`,
	RunE:  runLatest,
}

func init() {
	rootCmd.AddCommand(latestCmd)

	latestCmd.Flags().String("namespace", "", "Namespace (overrides group if set)")
	latestCmd.Flags().String("name", "", "Schema name (overrides artifact-id if set)")
}

func runLatest(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	namespace, _ := cmd.Flags().GetString("namespace")
	name, _ := cmd.Flags().GetString("name")

	var group, artifactID, fqn string

	if namespace != "" && name != "" {
		group = cfg.GetGroup(namespace)
		artifactID = cfg.GetArtifactID(name)
		fqn = cfg.GetFQN(namespace, name)
	} else if cfg.Group != "" && cfg.ArtifactID != "" {
		group = cfg.Group
		artifactID = cfg.ArtifactID
		fqn = fmt.Sprintf("%s.%s", group, artifactID)
	} else {
		return fmt.Errorf("either --namespace and --name flags or --group and --artifact-id config must be set")
	}

	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultOperationTimeout)
	defer cancel()

	metadata, err := client.GetArtifactMetadata(ctx, group, artifactID)
	if err != nil {
		return fmt.Errorf("failed to get artifact metadata: %w", err)
	}

	contentResp, err := client.GetArtifactContent(ctx, group, artifactID)
	if err != nil {
		return fmt.Errorf("failed to get artifact content: %w", err)
	}

	// GetArtifactContent returns interface{} which is a string containing the schema JSON
	var schemaContent map[string]interface{}

	// Handle different content types
	switch content := contentResp.(type) {
	case string:
		// Content is already a string (most common case)
		if err := json.Unmarshal([]byte(content), &schemaContent); err != nil {
			return fmt.Errorf("failed to parse schema content: %w", err)
		}
	case []byte:
		// Content is bytes
		if err := json.Unmarshal(content, &schemaContent); err != nil {
			return fmt.Errorf("failed to parse schema content: %w", err)
		}
	default:
		// Content is already structured, use as-is
		contentBytes, err := json.Marshal(contentResp)
		if err != nil {
			return fmt.Errorf("failed to marshal content: %w", err)
		}
		if err := json.Unmarshal(contentBytes, &schemaContent); err != nil {
			return fmt.Errorf("failed to parse schema content: %w", err)
		}
	}

	result := map[string]interface{}{
		"fqn":         fqn,
		"group":       group,
		"artifact_id": artifactID,
		"metadata":    buildMetadata(metadata),
		"schema":      schemaContent,
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Println(string(output))

	return nil
}

func buildMetadata(metadata interface{}) map[string]interface{} {
	metadataMap := make(map[string]interface{})

	data, err := json.Marshal(metadata)
	if err != nil {
		return metadataMap
	}

	if err := json.Unmarshal(data, &metadataMap); err != nil {
		return metadataMap
	}

	return metadataMap
}
