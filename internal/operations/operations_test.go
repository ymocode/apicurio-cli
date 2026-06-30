package operations

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/registry"
	"github.com/ymocode/apicurio-client/internal/schema"
)

const testSchemaContent = `{
	"type": "record",
	"name": "User",
	"namespace": "com.example.v1",
	"version": "1.0.0",
	"fields": [{"name": "id", "type": "string"}]
}`

// fakeClient is a configurable registry.RegistryClient that records the labels
// passed to the create methods.
type fakeClient struct {
	existingContent  string // when set, GetArtifactMetadata/Content succeed with this content
	existingVersion  string
	conflictOnCreate bool // CreateArtifact returns a 409 so the version path is taken

	createArtifactCalls int
	createVersionCalls  int
	createArtifactLabel map[string]string
	createVersionLabel  map[string]string
}

func (f *fakeClient) CreateArtifact(_ context.Context, _, _ string, _ *schema.AvroSchema, labels map[string]string) (*registry.ArtifactMetadata, error) {
	f.createArtifactCalls++
	f.createArtifactLabel = labels
	if f.conflictOnCreate {
		return nil, errorConflict("artifact already exists (409)")
	}
	return &registry.ArtifactMetadata{Version: "1.0.0", GlobalID: 1, ContentID: 1}, nil
}

func (f *fakeClient) CreateVersion(_ context.Context, _, _ string, _ *schema.AvroSchema, labels map[string]string) (*registry.VersionMetadata, error) {
	f.createVersionCalls++
	f.createVersionLabel = labels
	return &registry.VersionMetadata{Version: "1.0.1", GlobalID: 2, ContentID: 2}, nil
}

func (f *fakeClient) GetArtifactMetadata(_ context.Context, _, _ string) (*registry.ArtifactMetadata, error) {
	if f.existingContent == "" {
		return nil, errorNotFound("not found (404)")
	}
	return &registry.ArtifactMetadata{Version: f.existingVersion, GlobalID: 1, ContentID: 1}, nil
}

func (f *fakeClient) GetArtifactContent(_ context.Context, _, _ string) (interface{}, error) {
	if f.existingContent == "" {
		return nil, errorNotFound("not found (404)")
	}
	return f.existingContent, nil
}

func (f *fakeClient) TestCompatibility(_ context.Context, _, _, _, _ string) (bool, error) {
	return true, nil
}

func (f *fakeClient) GetSystemInfo(_ context.Context) (*registry.SystemInfo, error) {
	return &registry.SystemInfo{}, nil
}

func (f *fakeClient) GetAPIVersion() config.APIVersion { return config.APIVersionV3 }

func (f *fakeClient) CreateArtifactWithReferences(_ context.Context, _, _, _, _, _, _, _, _ string, _ []registry.ArtifactReference) (*registry.ArtifactMetadata, error) {
	return nil, registry.ErrNotSupported
}

func (f *fakeClient) GetArtifactContentDereferenced(_ context.Context, _, _, _, _ string) ([]byte, error) {
	return nil, registry.ErrNotSupported
}

type errorNotFound string

func (e errorNotFound) Error() string { return string(e) }

type errorConflict string

func (e errorConflict) Error() string { return string(e) }

func newSchema(t *testing.T) *schema.AvroSchema {
	t.Helper()
	tmp := t.TempDir() + "/user.avsc"
	if err := os.WriteFile(tmp, []byte(testSchemaContent), 0o600); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	s, err := schema.ParseAvroSchema(tmp)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	return s
}

func TestRegisterSchema_LabelsAppliedToNewArtifact(t *testing.T) {
	client := &fakeClient{}
	cfg := &config.Config{APIVersion: config.APIVersionV3}
	labels := map[string]string{"bundleVersion": "1.2.0", "gitTag": "v1.2.0"}

	result := RegisterSchema(context.Background(), client, cfg, newSchema(t), true, labels)

	if !result.Success || !result.IsNewArtifact {
		t.Fatalf("expected successful new-artifact registration, got %+v", result)
	}
	if client.createArtifactCalls != 1 {
		t.Errorf("CreateArtifact calls = %d, want 1", client.createArtifactCalls)
	}
	if !reflect.DeepEqual(client.createArtifactLabel, labels) {
		t.Errorf("labels passed to CreateArtifact = %v, want %v", client.createArtifactLabel, labels)
	}
	if !reflect.DeepEqual(result.LabelsApplied, labels) {
		t.Errorf("LabelsApplied = %v, want %v", result.LabelsApplied, labels)
	}
	if result.LabelsSkipped {
		t.Error("LabelsSkipped = true, want false")
	}
}

func TestRegisterSchema_LabelsAppliedToNewVersion(t *testing.T) {
	client := &fakeClient{conflictOnCreate: true}
	cfg := &config.Config{APIVersion: config.APIVersionV3}
	labels := map[string]string{"bundleVersion": "1.2.0"}

	result := RegisterSchema(context.Background(), client, cfg, newSchema(t), true, labels)

	if !result.Success || result.IsNewArtifact {
		t.Fatalf("expected successful new-version registration, got %+v", result)
	}
	if client.createVersionCalls != 1 {
		t.Errorf("CreateVersion calls = %d, want 1", client.createVersionCalls)
	}
	if !reflect.DeepEqual(client.createVersionLabel, labels) {
		t.Errorf("labels passed to CreateVersion = %v, want %v", client.createVersionLabel, labels)
	}
	if !reflect.DeepEqual(result.LabelsApplied, labels) {
		t.Errorf("LabelsApplied = %v, want %v", result.LabelsApplied, labels)
	}
}

func TestRegisterSchema_LabelsSkippedWhenUnchanged(t *testing.T) {
	client := &fakeClient{existingContent: testSchemaContent, existingVersion: "1.0.0"}
	cfg := &config.Config{APIVersion: config.APIVersionV3}
	labels := map[string]string{"bundleVersion": "1.2.1"}

	result := RegisterSchema(context.Background(), client, cfg, newSchema(t), false, labels)

	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if client.createArtifactCalls != 0 || client.createVersionCalls != 0 {
		t.Errorf("no create call expected for unchanged content, got artifact=%d version=%d",
			client.createArtifactCalls, client.createVersionCalls)
	}
	if !result.LabelsSkipped {
		t.Error("LabelsSkipped = false, want true")
	}
	if len(result.LabelsApplied) != 0 {
		t.Errorf("LabelsApplied = %v, want empty", result.LabelsApplied)
	}
}
