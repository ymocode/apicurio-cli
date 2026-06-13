// Package schema provides Avro schema parsing, validation and comparison.
package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/hamba/avro/v2" // Only for parsing, NOT for compatibility checking
	"github.com/ymocode/apicurio-client/internal/config"
)

// AvroSchema wraps the official avro.Schema with custom metadata
type AvroSchema struct {
	Schema    avro.Schema            `json:"-"`
	RawData   map[string]interface{} `json:"-"`
	Version   string                 `json:"version"`
	Namespace string                 `json:"namespace"`
	Name      string                 `json:"name"`
}

// AvroField represents an Avro record field. Deprecated: Use official avro.Schema field types instead.
type AvroField struct {
	Type    interface{} `json:"type"`
	Default interface{} `json:"default,omitempty"`
	Name    string      `json:"name"`
	Doc     string      `json:"doc,omitempty"`
}

type CompatibilityType string

const (
	CompatibilityInitial   CompatibilityType = "initial"   // New schema (first version)
	CompatibilityIdentical CompatibilityType = "identical" // No changes
	CompatibilityBackward  CompatibilityType = "backward"  // Backward compatible (can read old data)
	CompatibilityBreaking  CompatibilityType = "breaking"  // Breaking/incompatible changes
)

type ValidationResult struct {
	FQN               string            `json:"fqn"`
	CurrentVersion    string            `json:"current_version"`
	ProposedVersion   string            `json:"proposed_version"`
	ExpectedVersion   string            `json:"expected_version"`
	CurrentNamespace  string            `json:"current_namespace,omitempty"`
	ProposedNamespace string            `json:"proposed_namespace,omitempty"`
	ExpectedNamespace string            `json:"expected_namespace,omitempty"`
	CompatibilityType CompatibilityType `json:"compatibility_type"`
	ChangeLevel       string            `json:"change_level,omitempty"`
	Differences       []string          `json:"differences"`
	ValidationErrors  []string          `json:"validation_errors"`
	IsCompatible      bool              `json:"is_compatible"`
}

func ParseAvroSchema(filePath string) (*AvroSchema, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	// Parse raw JSON to extract custom fields (version, namespace, name)
	var rawData map[string]interface{}
	err = json.Unmarshal(data, &rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema JSON: %w", err)
	}

	// Extract custom fields
	version, _ := rawData["version"].(string)
	namespace, _ := rawData["namespace"].(string)
	name, _ := rawData["name"].(string)

	// Validate required custom fields
	if version == "" {
		return nil, fmt.Errorf("schema must contain a 'version' field with semantic version")
	}

	_, err = semver.NewVersion(version)
	if err != nil {
		return nil, fmt.Errorf("invalid semantic version '%s': %w", version, err)
	}

	if namespace == "" {
		return nil, fmt.Errorf("schema must contain a 'namespace' field")
	}

	if name == "" {
		return nil, fmt.Errorf("schema must contain a 'name' field")
	}

	// Parse using official Avro SDK (validates against Avro spec)
	avroSchema, err := avro.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Avro schema (invalid schema): %w", err)
	}

	return &AvroSchema{
		Version:   version,
		Namespace: namespace,
		Name:      name,
		Schema:    avroSchema,
		RawData:   rawData,
	}, nil
}

func (s *AvroSchema) GetContentString() (string, error) {
	data, err := json.Marshal(s.RawData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema: %w", err)
	}
	return string(data), nil
}

// GetMinifiedContent returns the schema content as minified (compact) JSON
func (s *AvroSchema) GetMinifiedContent() (string, error) {
	// Marshal without indentation for compact output
	data, err := json.Marshal(s.RawData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema: %w", err)
	}
	return string(data), nil
}

// GetDoc returns the documentation string from the schema
func (s *AvroSchema) GetDoc() string {
	doc, _ := s.RawData["doc"].(string)
	return doc
}

// CompareSchemas compares two schemas and returns differences and change level
// IMPORTANT: This does NOT determine compatibility - that comes from registry!
// This only extracts differences for user information and suggests change level
func CompareSchemas(oldSchema, newSchema *AvroSchema) (string, []string) {
	differences := []string{}
	hasStructuralChanges := false
	hasBreakingIndicators := false

	// Check namespace/name changes (always breaking indicators)
	if oldSchema.Namespace != newSchema.Namespace {
		differences = append(differences, fmt.Sprintf("~ Schema.namespace: %v → %v",
			formatValue(oldSchema.Namespace), formatValue(newSchema.Namespace)))
		hasBreakingIndicators = true
	}

	if oldSchema.Name != newSchema.Name {
		differences = append(differences, fmt.Sprintf("~ Schema.name: %v → %v",
			formatValue(oldSchema.Name), formatValue(newSchema.Name)))
		hasBreakingIndicators = true
	}

	// Get raw field data to identify what changed
	oldRawFields := extractRawFieldsMap(oldSchema.RawData)
	newRawFields := extractRawFieldsMap(newSchema.RawData)

	// Compare doc field at schema level
	oldDoc, _ := oldSchema.RawData["doc"].(string)
	newDoc, _ := newSchema.RawData["doc"].(string)
	if oldDoc != newDoc {
		differences = append(differences, fmt.Sprintf("~ Schema.doc: %v → %v",
			formatValue(oldDoc), formatValue(newDoc)))
	}

	// Check for removed fields (breaking indicator)
	for name := range oldRawFields {
		if _, exists := newRawFields[name]; !exists {
			differences = append(differences, fmt.Sprintf("- Field %s: removed", formatValue(name)))
			hasBreakingIndicators = true
		}
	}

	// Check for added and modified fields
	for name, newFieldRaw := range newRawFields {
		if oldFieldRaw, exists := oldRawFields[name]; exists {
			// Field exists in both - compare for differences
			fieldDiffs, breaking, structural := compareFieldsRaw(name, oldFieldRaw, newFieldRaw, "")
			differences = append(differences, fieldDiffs...)
			if breaking {
				hasBreakingIndicators = true
			} else if structural {
				hasStructuralChanges = true
			}
		} else {
			// Field added
			isOptional := hasDefault(newFieldRaw) || isOptionalFieldType(newFieldRaw)
			if isOptional {
				differences = append(differences, fmt.Sprintf("+ Field %s: optional field added", formatValue(name)))
				hasStructuralChanges = true
			} else {
				differences = append(differences, fmt.Sprintf("+ Field %s: required field added (breaking)", formatValue(name)))
				hasBreakingIndicators = true
			}
		}
	}

	// Determine suggested change level (NOT compatibility - that's registry's job)
	var changeLevel string
	if len(differences) == 0 {
		changeLevel = "" // Identical
	} else if hasBreakingIndicators {
		changeLevel = config.ChangeLevelMajor // Likely breaking (but registry decides!)
	} else if hasStructuralChanges {
		changeLevel = config.ChangeLevelMinor // Structural but likely compatible
	} else {
		changeLevel = config.ChangeLevelPatch // Only metadata changes
	}

	return changeLevel, differences
}

// compareTypes compares two Avro types recursively
func compareTypes(path string, oldType, newType interface{}) ([]string, bool, bool) {
	differences := []string{}
	hasBreaking := false
	hasStructural := false

	// Convert to maps for structured comparison
	oldMap, oldIsMap := oldType.(map[string]interface{})
	newMap, newIsMap := newType.(map[string]interface{})

	// If both are maps (record, enum, etc.)
	if oldIsMap && newIsMap {
		oldTypeName, _ := oldMap["type"].(string)
		newTypeName, _ := newMap["type"].(string)

		if oldTypeName != newTypeName {
			differences = append(differences, fmt.Sprintf("~ %s: type changed from %v to %v",
				path, oldTypeName, newTypeName))
			return differences, true, false
		}

		// Compare record types
		if oldTypeName == "record" {
			// Compare doc
			if oldMap["doc"] != newMap["doc"] {
				differences = append(differences, fmt.Sprintf("~ %s.doc: %v → %v",
					path, formatValue(oldMap["doc"]), formatValue(newMap["doc"])))
			}

			// Compare fields recursively
			oldFields := extractFieldsMap(oldMap)
			newFields := extractFieldsMap(newMap)

			for name, newFieldData := range newFields {
				if oldFieldData, exists := oldFields[name]; exists {
					fieldDiffs, breaking, structural := compareNestedFields(path+"."+name, oldFieldData, newFieldData)
					differences = append(differences, fieldDiffs...)
					if breaking {
						hasBreaking = true
					}
					if structural {
						hasStructural = true
					}
				} else {
					// Check if the added field is optional or required
					isOptional := hasDefault(newFieldData) || isOptionalFieldType(newFieldData)
					if isOptional {
						differences = append(differences, fmt.Sprintf("+ %s.%s: optional field added", path, name))
						hasStructural = true
					} else {
						differences = append(differences, fmt.Sprintf("+ %s.%s: required field added", path, name))
						hasBreaking = true
					}
				}
			}

			for name := range oldFields {
				if _, exists := newFields[name]; !exists {
					differences = append(differences, fmt.Sprintf("- %s.%s: field removed", path, name))
					hasBreaking = true
				}
			}
		} else if oldTypeName == "enum" {
			// Compare doc
			if oldMap["doc"] != newMap["doc"] {
				differences = append(differences, fmt.Sprintf("~ %s.doc: %v → %v",
					path, formatValue(oldMap["doc"]), formatValue(newMap["doc"])))
			}

			// Compare symbols
			oldSymbols := fmt.Sprintf("%v", oldMap["symbols"])
			newSymbols := fmt.Sprintf("%v", newMap["symbols"])
			if oldSymbols != newSymbols {
				differences = append(differences, fmt.Sprintf("~ %s.symbols: %v → %v",
					path, oldSymbols, newSymbols))
				hasBreaking = true
			}
		}
	} else if oldUnion, oldIsUnion := oldType.([]interface{}); oldIsUnion {
		// Union type, e.g. ["null", "string"] or ["null", {"type":"record", ...}].
		if newUnion, newIsUnion := newType.([]interface{}); newIsUnion {
			return compareUnions(path, oldUnion, newUnion)
		}
		// Union replaced by a non-union type.
		differences = append(differences, fmt.Sprintf("~ %s: %v → %v",
			path, fmt.Sprintf("%v", oldType), fmt.Sprintf("%v", newType)))
		hasBreaking = true
	} else {
		// Primitive type, or a change in type kind.
		oldStr := fmt.Sprintf("%v", oldType)
		newStr := fmt.Sprintf("%v", newType)
		if oldStr != newStr {
			differences = append(differences, fmt.Sprintf("~ %s: %v → %v",
				path, oldStr, newStr))
			hasBreaking = true
		}
	}

	return differences, hasBreaking, hasStructural
}

// compareUnions compares two Avro union types member-by-member, matching members
// by type identity (see unionMemberKey) rather than by serialized content.
// Members present in both unions are compared recursively, so changes nested
// inside a union member (such as a record member's documentation) are graded by
// the same rules as any other type. A removed member is breaking, an added
// member is structural, and a member replaced by one of a different type is
// breaking.
func compareUnions(path string, oldUnion, newUnion []interface{}) ([]string, bool, bool) {
	differences := []string{}
	hasBreaking := false
	hasStructural := false

	oldMembers := indexUnionMembers(oldUnion)
	newMembers := indexUnionMembers(newUnion)

	var removed, added, matched []string
	for key := range oldMembers {
		if _, exists := newMembers[key]; exists {
			matched = append(matched, key)
		} else {
			removed = append(removed, key)
		}
	}
	for key := range newMembers {
		if _, exists := oldMembers[key]; !exists {
			added = append(added, key)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	sort.Strings(matched)

	if len(removed) == 1 && len(added) == 1 {
		differences = append(differences, fmt.Sprintf("~ %s: union member type changed from %s to %s",
			path, removed[0], added[0]))
		hasBreaking = true
	} else {
		for _, key := range removed {
			differences = append(differences, fmt.Sprintf("- %s: union member %s removed", path, key))
			hasBreaking = true
		}
		for _, key := range added {
			differences = append(differences, fmt.Sprintf("+ %s: union member %s added", path, key))
			hasStructural = true
		}
	}

	for _, key := range matched {
		memberDiffs, memberBreaking, memberStructural := compareTypes(path, oldMembers[key], newMembers[key])
		differences = append(differences, memberDiffs...)
		if memberBreaking {
			hasBreaking = true
		}
		if memberStructural {
			hasStructural = true
		}
	}

	return differences, hasBreaking, hasStructural
}

// indexUnionMembers maps each union member to its identity key (see unionMemberKey).
func indexUnionMembers(union []interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(union))
	for _, member := range union {
		result[unionMemberKey(member)] = member
	}
	return result
}

// unionMemberKey returns a stable identity for a union member. Named complex
// types (record, enum, fixed, error) are keyed by name so that edits to their
// internal definition keep the same key; logical types are keyed by their
// logicalType; all other types are keyed by their type name.
func unionMemberKey(member interface{}) string {
	switch m := member.(type) {
	case string:
		return m
	case map[string]interface{}:
		typeName, _ := m["type"].(string)
		switch typeName {
		case "record", "enum", "fixed", "error":
			if name, ok := m["name"].(string); ok {
				return typeName + ":" + name
			}
			return typeName
		default:
			if logical, ok := m["logicalType"].(string); ok {
				return "logical:" + logical
			}
			return typeName
		}
	default:
		return fmt.Sprintf("%v", member)
	}
}

// extractFieldsMap extracts fields from a record type as a map by name
func extractFieldsMap(recordMap map[string]interface{}) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})

	if fieldsArray, ok := recordMap["fields"].([]interface{}); ok {
		for _, field := range fieldsArray {
			if fieldMap, ok := field.(map[string]interface{}); ok {
				if name, ok := fieldMap["name"].(string); ok {
					result[name] = fieldMap
				}
			}
		}
	}

	return result
}

// extractRawFieldsMap extracts top-level fields from schema RawData as a map by name
func extractRawFieldsMap(rawData map[string]interface{}) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})

	if fieldsArray, ok := rawData["fields"].([]interface{}); ok {
		for _, field := range fieldsArray {
			if fieldMap, ok := field.(map[string]interface{}); ok {
				if name, ok := fieldMap["name"].(string); ok {
					result[name] = fieldMap
				}
			}
		}
	}

	return result
}

// compareFieldsRaw compares two raw field definitions including all attributes
func compareFieldsRaw(fieldName string, oldField, newField map[string]interface{}, pathPrefix string) ([]string, bool, bool) {
	differences := []string{}
	hasBreaking := false
	hasStructural := false

	fullPath := fieldName
	if pathPrefix != "" {
		fullPath = pathPrefix + "." + fieldName
	}

	// Compare doc
	if oldField["doc"] != newField["doc"] {
		differences = append(differences, fmt.Sprintf("~ %s.doc: %v → %v",
			fullPath, formatValue(oldField["doc"]), formatValue(newField["doc"])))
	}

	// Compare default
	if !equalValues(oldField["default"], newField["default"]) {
		differences = append(differences, fmt.Sprintf("~ %s.default: %v → %v",
			fullPath, formatValue(oldField["default"]), formatValue(newField["default"])))
		hasStructural = true
	}

	// Compare other structural attributes (pattern, aliases, order, etc.)
	structuralAttrs := []string{"pattern", "aliases", "order"}
	for _, attr := range structuralAttrs {
		oldVal, oldExists := oldField[attr]
		newVal, newExists := newField[attr]

		if oldExists != newExists || (oldExists && !equalValues(oldVal, newVal)) {
			differences = append(differences, fmt.Sprintf("~ %s.%s: %v → %v",
				fullPath, attr, formatValue(oldVal), formatValue(newVal)))
			hasStructural = true
		}
	}

	// Compare type
	typeDiffs, typeBreaking, typeStructural := compareTypes(fullPath, oldField["type"], newField["type"])
	differences = append(differences, typeDiffs...)
	if typeBreaking {
		hasBreaking = true
	}
	if typeStructural {
		hasStructural = true
	}

	return differences, hasBreaking, hasStructural
}

// compareNestedFields compares nested field definitions
// This is a convenience wrapper around compareFieldsRaw for nested fields
func compareNestedFields(path string, oldField, newField map[string]interface{}) ([]string, bool, bool) {
	// Call compareFieldsRaw with path as fieldName and empty pathPrefix
	// This results in fullPath = path (since pathPrefix is empty)
	return compareFieldsRaw(path, oldField, newField, "")
}

// equalValues checks if two values are equal
func equalValues(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// hasDefault checks if a field map has a default value
func hasDefault(fieldMap map[string]interface{}) bool {
	_, exists := fieldMap["default"]
	return exists
}

// isOptionalFieldType checks if a field's type is optional (union with null)
func isOptionalFieldType(fieldMap map[string]interface{}) bool {
	fieldType, ok := fieldMap["type"]
	if !ok {
		return false
	}
	return isOptionalType(fieldType)
}

// formatValue formats a value for display, truncating long strings
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		if len(v) > 100 {
			return fmt.Sprintf("%q...", v[:97])
		}
		return fmt.Sprintf("%q", v)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func isOptionalType(fieldType interface{}) bool {
	if typeArray, ok := fieldType.([]interface{}); ok {
		for _, t := range typeArray {
			if t == "null" {
				return true
			}
		}
	}
	return false
}

func CalculateExpectedVersion(currentVersion string, compatType CompatibilityType) (string, error) {
	ver, err := semver.NewVersion(currentVersion)
	if err != nil {
		return "", fmt.Errorf("invalid current version: %w", err)
	}

	switch compatType {
	case CompatibilityIdentical:
		// No changes, keep the same version
		return currentVersion, nil
	case CompatibilityBackward:
		// For backward compatible, default to minor
		newVer := ver.IncMinor()
		return newVer.String(), nil
	case CompatibilityBreaking:
		newVer := ver.IncMajor()
		return newVer.String(), nil
	default:
		return "", fmt.Errorf("unknown compatibility type: %s", compatType)
	}
}

func CalculateExpectedVersionFromLevel(currentVersion, changeLevel string) (string, error) {
	if changeLevel == "" {
		return currentVersion, nil
	}

	ver, err := semver.NewVersion(currentVersion)
	if err != nil {
		return "", fmt.Errorf("invalid current version: %w", err)
	}

	switch changeLevel {
	case config.ChangeLevelPatch:
		newVer := ver.IncPatch()
		return newVer.String(), nil
	case config.ChangeLevelMinor:
		newVer := ver.IncMinor()
		return newVer.String(), nil
	case config.ChangeLevelMajor:
		newVer := ver.IncMajor()
		return newVer.String(), nil
	default:
		return "", fmt.Errorf("unknown change level: %s", changeLevel)
	}
}

func ValidateVersionBump(currentVersion, proposedVersion string, compatType CompatibilityType) error {
	expectedVersion, err := CalculateExpectedVersion(currentVersion, compatType)
	if err != nil {
		return err
	}

	if proposedVersion != expectedVersion {
		return fmt.Errorf("version mismatch: expected %s for %s change, got %s",
			expectedVersion, compatType, proposedVersion)
	}

	return nil
}

func FormatDiff(differences []string) string {
	if len(differences) == 0 {
		return "No changes"
	}
	return strings.Join(differences, "\n")
}
