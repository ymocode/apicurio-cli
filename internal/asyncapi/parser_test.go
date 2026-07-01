package asyncapi

import (
	"reflect"
	"testing"

	"github.com/ymocode/apicurio-client/internal/registry"
)

const (
	coordNaturalPerson = "ch.centrisag.avro.phs.person.v1/NaturalPersonCreated:1.0.0"
	coordLegalPerson   = "ch.centrisag.avro.phs.person.v1/LegalPersonCreated:1.0.0"
)

// docHeader holds the required AsyncAPI metadata fields shared by the fixtures.
const docHeader = `asyncapi: 3.0.0
x-namespace: ch.centrisag.asyncapi.phs
x-domain: person
info:
  title: Person API
  version: 1.0.0
`

// directPayloadDoc places references directly at components.messages.*.payload.$ref.
const directPayloadDoc = docHeader + `channels:
  natural-person-created:
    messages:
      natural-person-created:
        $ref: '#/components/messages/NaturalPersonCreated'
operations:
  sendNaturalPersonCreated:
    action: send
    messages:
    - $ref: '#/channels/natural-person-created/messages/natural-person-created'
components:
  messages:
    NaturalPersonCreated:
      payload:
        $ref: ch.centrisag.avro.phs.person.v1/NaturalPersonCreated:1.0.0
    LegalPersonCreated:
      payload:
        $ref: ch.centrisag.avro.phs.person.v1/LegalPersonCreated:1.0.0
`

// wrappedPayloadDoc places references at components.messages.*.payload.schema.$ref.
const wrappedPayloadDoc = docHeader + `components:
  messages:
    NaturalPersonCreated:
      payload:
        schemaFormat: application/vnd.apache.avro;version=1.9.0
        schema:
          $ref: ch.centrisag.avro.phs.person.v1/NaturalPersonCreated:1.0.0
    LegalPersonCreated:
      payload:
        schemaFormat: application/vnd.apache.avro;version=1.9.0
        schema:
          $ref: ch.centrisag.avro.phs.person.v1/LegalPersonCreated:1.0.0
`

// mixedPayloadDoc combines both forms and repeats a coordinate via an inline
// channel message to exercise de-duplication.
const mixedPayloadDoc = docHeader + `channels:
  natural-person-created:
    messages:
      natural-person-created:
        payload:
          $ref: ch.centrisag.avro.phs.person.v1/NaturalPersonCreated:1.0.0
components:
  messages:
    NaturalPersonCreated:
      payload:
        $ref: ch.centrisag.avro.phs.person.v1/NaturalPersonCreated:1.0.0
    LegalPersonCreated:
      payload:
        schema:
          $ref: ch.centrisag.avro.phs.person.v1/LegalPersonCreated:1.0.0
`

func coordinates(refs []registry.ArtifactReference) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Name
	}
	return out
}

func TestParse_ExtractsReferencesFromAllForms(t *testing.T) {
	want := []string{coordLegalPerson, coordNaturalPerson} // sorted by coordinate

	tests := []struct {
		name string
		doc  string
	}{
		{name: "direct payload $ref", doc: directPayloadDoc},
		{name: "wrapped payload.schema.$ref", doc: wrappedPayloadDoc},
		{name: "mixed forms with duplicate", doc: mixedPayloadDoc},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Parse([]byte(tc.doc))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := coordinates(result.References); !reflect.DeepEqual(got, want) {
				t.Errorf("references = %v, want %v", got, want)
			}
		})
	}
}

func TestParse_ReferenceCoordinateParsing(t *testing.T) {
	result, err := Parse([]byte(directPayloadDoc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var natural *registry.ArtifactReference
	for i := range result.References {
		if result.References[i].Name == coordNaturalPerson {
			natural = &result.References[i]
		}
	}
	if natural == nil {
		t.Fatalf("reference %q not found in %v", coordNaturalPerson, coordinates(result.References))
	}

	if natural.GroupID != "ch.centrisag.avro.phs.person.v1" {
		t.Errorf("GroupID = %q", natural.GroupID)
	}
	if natural.ArtifactID != "NaturalPersonCreated" {
		t.Errorf("ArtifactID = %q", natural.ArtifactID)
	}
	if natural.Version != "1.0.0" {
		t.Errorf("Version = %q", natural.Version)
	}
}

func TestParse_IgnoresInternalReferences(t *testing.T) {
	const doc = docHeader + `components:
  messages:
    OnlyInternal:
      payload:
        $ref: '#/components/schemas/Whatever'
`
	result, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.References) != 0 {
		t.Errorf("expected no references, got %v", coordinates(result.References))
	}
}
