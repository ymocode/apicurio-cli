package asyncapi

import "github.com/ymocode/apicurio-client/internal/registry"

// AsyncAPIDocument represents a parsed AsyncAPI document
type AsyncAPIDocument struct {
	GroupID     string                       // from x-namespace (or flag override)
	ArtifactID  string                       // from x-domain (or flag override)
	Version     string                       // from info.version (or flag override)
	Title       string                       // from info.title
	Description string                       // from info.description
	Content     []byte                       // raw YAML content
	References  []registry.ArtifactReference // extracted from components.messages.*.payload.schema.$ref
}

// GetContentString returns the content as a string
func (d *AsyncAPIDocument) GetContentString() string {
	return string(d.Content)
}

// GetContentType returns the MIME type for AsyncAPI documents
func (d *AsyncAPIDocument) GetContentType() string {
	return "application/x-yaml"
}

// GetArtifactType returns the Apicurio artifact type
func (d *AsyncAPIDocument) GetArtifactType() string {
	return "ASYNCAPI"
}

// AvroSchemaFormat is the MIME type for Avro schemas in AsyncAPI Multi-Format Schema Objects
// Uses the standard format recognized by AsyncAPI tooling (avro-schema-parser)
const AvroSchemaFormat = "application/vnd.apache.avro;version=1.9.0"

// FixAvroSchemas checks components.schemas for raw Avro schemas and wraps them
// in Multi-Format Schema Objects. This fixes a known Apicurio bug where dereferenced
// AsyncAPI 3.0 documents contain raw Avro schemas instead of properly wrapped ones.
// Returns true if any schemas were fixed.
func FixAvroSchemas(doc map[string]interface{}) bool {
	components, ok := doc["components"].(map[string]interface{})
	if !ok {
		return false
	}

	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		return false
	}

	fixed := false
	for name, schema := range schemas {
		schemaMap, ok := schema.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is a raw Avro schema (has "type": "record")
		// and is NOT already wrapped (doesn't have "schemaFormat")
		if schemaType, hasType := schemaMap["type"]; hasType {
			if typeStr, isString := schemaType.(string); isString && typeStr == "record" {
				// Check if already wrapped
				if _, hasSchemaFormat := schemaMap["schemaFormat"]; !hasSchemaFormat {
					// Wrap in Multi-Format Schema Object
					schemas[name] = map[string]interface{}{
						"schemaFormat": AvroSchemaFormat,
						"schema":       schemaMap,
					}
					fixed = true
				}
			}
		}
	}

	return fixed
}
