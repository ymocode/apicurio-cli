package registry

import (
	"context"
	"strings"

	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/schema"
)

// ArtifactMetadata represents metadata for an artifact, normalized across API versions
type ArtifactMetadata struct {
	ID          string
	Version     string
	GlobalID    int64
	ContentID   int64
	CreatedOn   string
	ModifiedOn  string
	Name        string
	Description string
	GroupID     string
}

// VersionMetadata represents metadata for a specific version
type VersionMetadata struct {
	Version    string
	GlobalID   int64
	ContentID  int64
	CreatedOn  string
	ModifiedOn string
}

// SystemInfo represents system information from the registry
type SystemInfo struct {
	Name        string
	Description string
	Version     string
	BuiltOn     string
}

// ArtifactReference represents a reference to another artifact (used for AsyncAPI referencing Avro schemas)
type ArtifactReference struct {
	GroupID    string
	ArtifactID string
	Version    string
	Name       string // Full reference name: "groupId/artifactId:version"
}

// RegistryClient is an interface that abstracts registry operations across V2, V3, and CCOMPAT APIs
type RegistryClient interface {
	// CreateArtifact creates a new artifact with initial content
	// Schema parameter provides access to name, description, and content
	CreateArtifact(ctx context.Context, group, artifactID string, schema *schema.AvroSchema) (*ArtifactMetadata, error)

	// CreateVersion creates a new version of an existing artifact
	CreateVersion(ctx context.Context, group, artifactID string, schema *schema.AvroSchema) (*VersionMetadata, error)

	// GetArtifactMetadata retrieves metadata for the latest version of an artifact
	GetArtifactMetadata(ctx context.Context, group, artifactID string) (*ArtifactMetadata, error)

	// GetArtifactContent retrieves the content of the latest version of an artifact
	GetArtifactContent(ctx context.Context, group, artifactID string) (interface{}, error)

	// TestCompatibility tests if the given content is compatible with the existing artifact (dry-run)
	// Returns (isCompatible, error)
	TestCompatibility(ctx context.Context, group, artifactID, content, version string) (bool, error)

	// GetSystemInfo retrieves system information from the registry
	GetSystemInfo(ctx context.Context) (*SystemInfo, error)

	// GetAPIVersion returns the API version this client is using
	GetAPIVersion() config.APIVersion

	// CreateArtifactWithReferences creates an artifact with references to other artifacts (V3 only)
	// Used for AsyncAPI documents that reference Avro schemas
	// Returns ErrNotSupported for V2/CCompat clients
	CreateArtifactWithReferences(ctx context.Context, group, artifactID, artifactType, version, name, description, content, contentType string, references []ArtifactReference) (*ArtifactMetadata, error)

	// GetArtifactContentDereferenced retrieves artifact content with references dereferenced (V3 only)
	// The references parameter can be "PRESERVE", "REWRITE", or "DEREFERENCE"
	// Returns ErrNotSupported for V2/CCompat clients
	GetArtifactContentDereferenced(ctx context.Context, group, artifactID, version, references string) ([]byte, error)
}

// NewRegistryClient creates a RegistryClient based on the provided configuration
func NewRegistryClient(cfg *config.Config) (RegistryClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	switch cfg.APIVersion {
	case config.APIVersionV2:
		return NewV2Client(cfg)
	case config.APIVersionV3:
		return NewV3Client(cfg)
	case config.APIVersionCCOMPAT:
		return NewCCompatClient(cfg)
	default:
		return NewV2Client(cfg) // Default to V2
	}
}

// IsNotFoundError checks if an error is a 404 Not Found error
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "http 404")
}
