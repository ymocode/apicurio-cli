package schema

import (
	"encoding/json"
	"testing"

	"github.com/ymocode/apicurio-client/internal/config"
)

// schemaFromJSON builds an AvroSchema from a JSON document for comparison tests.
// Only the fields exercised by CompareSchemas are populated.
func schemaFromJSON(t *testing.T, doc string) *AvroSchema {
	t.Helper()

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(doc), &raw); err != nil {
		t.Fatalf("invalid test schema JSON: %v", err)
	}

	namespace, _ := raw["namespace"].(string)
	name, _ := raw["name"].(string)
	version, _ := raw["version"].(string)

	return &AvroSchema{
		Version:   version,
		Namespace: namespace,
		Name:      name,
		RawData:   raw,
	}
}

func TestCompareSchemas_ChangeLevels(t *testing.T) {
	tests := []struct {
		name          string
		oldDoc        string
		newDoc        string
		wantLevel     string
		wantDiffCount int // -1 means "do not assert"
	}{
		{
			name: "optional record member nested doc change is patch",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "residenceAddress", "type": ["null", {
						"type": "record", "name": "Address",
						"fields": [
							{"name": "qualityLevel", "type": "string", "doc": "old"}
						]
					}]}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "residenceAddress", "type": ["null", {
						"type": "record", "name": "Address",
						"fields": [
							{"name": "qualityLevel", "type": "string", "doc": "new"}
						]
					}]}
				]
			}`,
			wantLevel:     config.ChangeLevelPatch,
			wantDiffCount: 1,
		},
		{
			name: "optional scalar field-level doc change is patch",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "nickname", "type": ["null", "string"], "doc": "old"}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "nickname", "type": ["null", "string"], "doc": "new"}
				]
			}`,
			wantLevel:     config.ChangeLevelPatch,
			wantDiffCount: 1,
		},
		{
			name: "union member primitive type change is breaking",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "age", "type": ["null", "string"]}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "age", "type": ["null", "int"]}
				]
			}`,
			wantLevel:     config.ChangeLevelMajor,
			wantDiffCount: -1,
		},
		{
			name: "union member removed is breaking",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": ["null", "string", "int"]}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": ["null", "string"]}
				]
			}`,
			wantLevel:     config.ChangeLevelMajor,
			wantDiffCount: -1,
		},
		{
			name: "optional union member added is minor",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": ["null", "string"]}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": ["null", "string", "int"]}
				]
			}`,
			wantLevel:     config.ChangeLevelMinor,
			wantDiffCount: -1,
		},
		{
			name: "required record field nested doc change is patch",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "address", "type": {
						"type": "record", "name": "Address",
						"fields": [
							{"name": "city", "type": "string", "doc": "old"}
						]
					}}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "address", "type": {
						"type": "record", "name": "Address",
						"fields": [
							{"name": "city", "type": "string", "doc": "new"}
						]
					}}
				]
			}`,
			wantLevel:     config.ChangeLevelPatch,
			wantDiffCount: 1,
		},
		{
			name: "identical schema has no change level",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": ["null", "string"]}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": ["null", "string"]}
				]
			}`,
			wantLevel:     "",
			wantDiffCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldSchema := schemaFromJSON(t, tc.oldDoc)
			newSchema := schemaFromJSON(t, tc.newDoc)

			level, diffs := CompareSchemas(oldSchema, newSchema)

			if level != tc.wantLevel {
				t.Errorf("change level = %q, want %q (diffs: %v)", level, tc.wantLevel, diffs)
			}
			if tc.wantDiffCount >= 0 && len(diffs) != tc.wantDiffCount {
				t.Errorf("diff count = %d, want %d (diffs: %v)", len(diffs), tc.wantDiffCount, diffs)
			}
		})
	}
}

func TestCompareSchemas_RegressionGuards(t *testing.T) {
	tests := []struct {
		name      string
		oldDoc    string
		newDoc    string
		wantLevel string
	}{
		{
			name: "enum symbol change is breaking",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "color", "type": {"type": "enum", "name": "Color", "symbols": ["RED", "GREEN"]}}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "color", "type": {"type": "enum", "name": "Color", "symbols": ["RED", "BLUE"]}}
				]
			}`,
			wantLevel: config.ChangeLevelMajor,
		},
		{
			name: "default change is minor",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "active", "type": "boolean", "default": true}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "active", "type": "boolean", "default": false}
				]
			}`,
			wantLevel: config.ChangeLevelMinor,
		},
		{
			name: "required field added is breaking",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": "string"}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": "string"},
					{"name": "name", "type": "string"}
				]
			}`,
			wantLevel: config.ChangeLevelMajor,
		},
		{
			name: "optional field added is minor",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": "string"}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": "string"},
					{"name": "name", "type": ["null", "string"], "default": null}
				]
			}`,
			wantLevel: config.ChangeLevelMinor,
		},
		{
			name: "field removed is breaking",
			oldDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": "string"},
					{"name": "name", "type": "string"}
				]
			}`,
			newDoc: `{
				"namespace": "com.example", "name": "Person", "version": "1.0.0",
				"type": "record",
				"fields": [
					{"name": "id", "type": "string"}
				]
			}`,
			wantLevel: config.ChangeLevelMajor,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldSchema := schemaFromJSON(t, tc.oldDoc)
			newSchema := schemaFromJSON(t, tc.newDoc)

			level, diffs := CompareSchemas(oldSchema, newSchema)
			if level != tc.wantLevel {
				t.Errorf("change level = %q, want %q (diffs: %v)", level, tc.wantLevel, diffs)
			}
		})
	}
}

// TestCompareSchemas_DocOnlyNestedRecords reproduces a documentation-only edit
// applied to nested fields of multiple optional records and asserts that the
// resulting expected version is a patch bump.
func TestCompareSchemas_DocOnlyNestedRecords(t *testing.T) {
	oldDoc := `{
		"namespace": "com.example", "name": "NatPerson", "version": "1.0.0",
		"type": "record",
		"fields": [
			{"name": "residenceAddress", "type": ["null", {
				"type": "record", "name": "ResidenceAddress",
				"fields": [
					{"name": "qualityLevel", "type": ["null", "string"], "doc": "Coded value"}
				]
			}]},
			{"name": "updateNatPerson", "type": ["null", {
				"type": "record", "name": "UpdateNatPerson",
				"fields": [
					{"name": "qualityLevel", "type": ["null", "string"], "doc": "Coded value"}
				]
			}]}
		]
	}`
	newDoc := `{
		"namespace": "com.example", "name": "NatPerson", "version": "1.0.0",
		"type": "record",
		"fields": [
			{"name": "residenceAddress", "type": ["null", {
				"type": "record", "name": "ResidenceAddress",
				"fields": [
					{"name": "qualityLevel", "type": ["null", "string"], "doc": "Coded value."}
				]
			}]},
			{"name": "updateNatPerson", "type": ["null", {
				"type": "record", "name": "UpdateNatPerson",
				"fields": [
					{"name": "qualityLevel", "type": ["null", "string"], "doc": "Coded value."}
				]
			}]}
		]
	}`

	oldSchema := schemaFromJSON(t, oldDoc)
	newSchema := schemaFromJSON(t, newDoc)

	level, diffs := CompareSchemas(oldSchema, newSchema)
	if level != config.ChangeLevelPatch {
		t.Fatalf("change level = %q, want %q (diffs: %v)", level, config.ChangeLevelPatch, diffs)
	}

	expected, err := CalculateExpectedVersionFromLevel("1.0.0", level)
	if err != nil {
		t.Fatalf("CalculateExpectedVersionFromLevel: %v", err)
	}
	if expected != "1.0.1" {
		t.Errorf("expected version = %q, want %q", expected, "1.0.1")
	}
}
