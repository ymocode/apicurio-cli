// Package asyncapi provides AsyncAPI document parsing and validation.
package asyncapi

import (
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/ymocode/apicurio-client/internal/registry"
	"gopkg.in/yaml.v3"
)

// asyncAPIDocument represents the structure of an AsyncAPI document
// Only fields we need are defined
type asyncAPIDocument struct {
	Components struct {
		Schemas map[string]schemaRef `yaml:"schemas"`
	} `yaml:"components"`
	Info struct {
		Title       string `yaml:"title"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
	} `yaml:"info"`
	AsyncAPI   string `yaml:"asyncapi"`
	XNamespace string `yaml:"x-namespace"`
	XDomain    string `yaml:"x-domain"`
}

// schemaRef represents a schema that may contain a $ref to an external artifact
// Supports both direct $ref and nested schema.$ref structures
type schemaRef struct {
	// Direct $ref at components.schemas.{name}.$ref
	Ref string `yaml:"$ref"`
	// Nested $ref at components.schemas.{name}.schema.$ref (AsyncAPI 3.0 multi-format schema)
	Schema struct {
		Ref string `yaml:"$ref"`
	} `yaml:"schema"`
}

// ParseFile parses an AsyncAPI YAML file and extracts metadata and references
func ParseFile(filePath string) (*AsyncAPIDocument, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return Parse(content)
}

// Parse parses AsyncAPI YAML content and extracts metadata and references
func Parse(content []byte) (*AsyncAPIDocument, error) {
	var doc asyncAPIDocument
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate required fields
	if doc.XNamespace == "" {
		return nil, fmt.Errorf("x-namespace is required in AsyncAPI document")
	}
	if doc.XDomain == "" {
		return nil, fmt.Errorf("x-domain is required in AsyncAPI document")
	}
	if doc.Info.Version == "" {
		return nil, fmt.Errorf("info.version is required in AsyncAPI document")
	}

	// Validate version is valid semver
	if _, err := semver.NewVersion(doc.Info.Version); err != nil {
		return nil, fmt.Errorf("info.version must be valid semver (e.g., 1.0.0): %w", err)
	}

	// Extract references from messages
	refs, err := extractReferences(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to extract references: %w", err)
	}

	return &AsyncAPIDocument{
		GroupID:     doc.XNamespace,
		ArtifactID:  doc.XDomain,
		Version:     doc.Info.Version,
		Title:       doc.Info.Title,
		Description: doc.Info.Description,
		Content:     content,
		References:  refs,
	}, nil
}

// extractReferences extracts schema references from AsyncAPI components.schemas
// Supports both:
//   - components.schemas.{name}.$ref (direct reference)
//   - components.schemas.{name}.schema.$ref (multi-format schema object)
func extractReferences(doc asyncAPIDocument) ([]registry.ArtifactReference, error) {
	var refs []registry.ArtifactReference

	for _, schema := range doc.Components.Schemas {
		// Check both direct $ref and nested schema.$ref
		refStr := schema.Ref
		if refStr == "" {
			refStr = schema.Schema.Ref
		}
		if refStr == "" {
			continue
		}

		// Skip internal references (e.g., #/components/schemas/...)
		if strings.HasPrefix(refStr, "#") {
			continue
		}

		ref, err := parseReference(refStr)
		if err != nil {
			return nil, fmt.Errorf("invalid reference %q: %w", refStr, err)
		}
		refs = append(refs, *ref)
	}

	return refs, nil
}

// parseReference parses a schema reference in format "groupId/artifactId:version"
// Example: "com.acme.avro.ph.policy.v1/RequestPolicyProductBasicInsuranceChange:1.0.0"
func parseReference(refStr string) (*registry.ArtifactReference, error) {
	// Split by "/" to get groupId and rest
	slashIdx := strings.LastIndex(refStr, "/")
	if slashIdx == -1 {
		return nil, fmt.Errorf("expected format 'groupId/artifactId:version'")
	}

	groupID := refStr[:slashIdx]
	rest := refStr[slashIdx+1:]

	// Split rest by ":" to get artifactId and version
	colonIdx := strings.LastIndex(rest, ":")
	if colonIdx == -1 {
		return nil, fmt.Errorf("expected format 'groupId/artifactId:version'")
	}

	artifactID := rest[:colonIdx]
	version := rest[colonIdx+1:]

	if groupID == "" || artifactID == "" || version == "" {
		return nil, fmt.Errorf("groupId, artifactId, and version must not be empty")
	}

	return &registry.ArtifactReference{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		Name:       refStr,
	}, nil
}
