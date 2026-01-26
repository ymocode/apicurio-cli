package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ymocode/apicurio-client/internal/auth"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/schema"

	registryclientv2 "github.com/apicurio/apicurio-registry/go-sdk/v3/pkg/registryclient-v2"
	"github.com/apicurio/apicurio-registry/go-sdk/v3/pkg/registryclient-v2/groups"
	"github.com/apicurio/apicurio-registry/go-sdk/v3/pkg/registryclient-v2/models"
	abstractions "github.com/microsoft/kiota-abstractions-go"
)

// V2Client implements the RegistryClient interface using the V2 API SDK
type V2Client struct {
	client *registryclientv2.ApiClient
	config *config.Config
}

// NewV2Client creates a new V2 API client
func NewV2Client(cfg *config.Config) (*V2Client, error) {
	adapter, err := auth.CreateKiotaAdapter(cfg, "/apis/registry/v2")
	if err != nil {
		return nil, fmt.Errorf("failed to create request adapter: %w", err)
	}

	client := registryclientv2.NewApiClient(adapter)

	return &V2Client{
		client: client,
		config: cfg,
	}, nil
}

// CreateArtifact creates a new artifact using V2 API
// V2 uses headers for metadata (X-Registry-ArtifactId, X-Registry-ArtifactType, X-Registry-Version)
func (c *V2Client) CreateArtifact(ctx context.Context, group, artifactID string, s *schema.AvroSchema) (*ArtifactMetadata, error) {
	// Get minified content
	content, err := s.GetMinifiedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get minified content: %w", err)
	}

	// Create artifact content
	artifactContent := models.NewArtifactContent()
	artifactContent.SetContent(&content)

	// Configure query parameters for semver handling
	// IfExists=RETURN_OR_UPDATE: returns existing version if content matches, otherwise creates new version
	ifExists := models.RETURN_OR_UPDATE_IFEXISTS
	canonical := true
	reqConfig := &groups.ItemArtifactsRequestBuilderPostRequestConfiguration{
		Headers: abstractions.NewRequestHeaders(),
		QueryParameters: &groups.ItemArtifactsRequestBuilderPostQueryParameters{
			IfExistsAsIfExists: &ifExists,
			Canonical:          &canonical,
		},
	}
	reqConfig.Headers.Add("X-Registry-ArtifactId", artifactID)
	reqConfig.Headers.Add("X-Registry-ArtifactType", "AVRO")
	reqConfig.Headers.Add("X-Registry-Version", s.Version)

	// V2 also supports name and description headers
	if s.Name != "" {
		reqConfig.Headers.Add("X-Registry-Name", s.Name)
	}
	if doc := s.GetDoc(); doc != "" {
		reqConfig.Headers.Add("X-Registry-Description", doc)
	}

	// Create artifact
	resp, err := c.client.Groups().ByGroupId(group).Artifacts().Post(ctx, artifactContent, reqConfig)
	if err != nil {
		return nil, formatV2Error("failed to create artifact", err, group, artifactID)
	}

	return convertV2ArtifactMetadata(resp), nil
}

// CreateVersion creates a new version using V2 API
func (c *V2Client) CreateVersion(ctx context.Context, group, artifactID string, s *schema.AvroSchema) (*VersionMetadata, error) {
	// Get minified content
	content, err := s.GetMinifiedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get minified content: %w", err)
	}

	// Create version content
	artifactContent := models.NewArtifactContent()
	artifactContent.SetContent(&content)

	// Configure request with version header
	reqConfig := &groups.ItemArtifactsItemVersionsRequestBuilderPostRequestConfiguration{
		Headers: abstractions.NewRequestHeaders(),
	}
	reqConfig.Headers.Add("X-Registry-Version", s.Version)

	// Add description header if available
	if doc := s.GetDoc(); doc != "" {
		reqConfig.Headers.Add("X-Registry-Description", doc)
	}

	// Create version
	resp, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().Post(ctx, artifactContent, reqConfig)
	if err != nil {
		return nil, formatV2Error("failed to create version", err, group, artifactID)
	}

	return convertV2VersionMetadata(resp), nil
}

// GetArtifactMetadata retrieves artifact metadata using V2 API
func (c *V2Client) GetArtifactMetadata(ctx context.Context, group, artifactID string) (*ArtifactMetadata, error) {
	// Get artifact metadata
	artifactMeta, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Meta().Get(ctx, nil)
	if err != nil {
		// V2 Error embeds ApiError which has GetStatusCode()
		if v2Error, ok := err.(*models.Error); ok {
			statusCode := v2Error.GetStatusCode()
			detail := ""
			if v2Error.GetDetail() != nil {
				detail = *v2Error.GetDetail()
			}
			if detail != "" {
				return nil, fmt.Errorf("failed to get artifact metadata (HTTP %d): %s", statusCode, detail)
			}
			return nil, fmt.Errorf("failed to get artifact metadata (HTTP %d)", statusCode)
		}
		return nil, fmt.Errorf("failed to get artifact metadata: %w", err)
	}

	// Get latest version metadata
	versionMeta, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().ByVersion("latest").Meta().Get(ctx, nil)
	if err != nil {
		// V2 Error embeds ApiError which has GetStatusCode()
		if v2Error, ok := err.(*models.Error); ok {
			statusCode := v2Error.GetStatusCode()
			detail := ""
			if v2Error.GetDetail() != nil {
				detail = *v2Error.GetDetail()
			}
			if detail != "" {
				return nil, fmt.Errorf("failed to get version metadata (HTTP %d): %s", statusCode, detail)
			}
			return nil, fmt.Errorf("failed to get version metadata (HTTP %d)", statusCode)
		}
		return nil, fmt.Errorf("failed to get version metadata: %w", err)
	}

	// Combine artifact and version metadata
	metadata := convertV2ArtifactMetadata(artifactMeta)
	metadata.Version = safeString(versionMeta.GetVersion())
	if versionMeta.GetGlobalId() != nil {
		metadata.GlobalID = *versionMeta.GetGlobalId()
	}
	if versionMeta.GetContentId() != nil {
		metadata.ContentID = *versionMeta.GetContentId()
	}
	if versionMeta.GetCreatedOn() != nil {
		metadata.CreatedOn = formatKiotaTime(versionMeta.GetCreatedOn())
	}

	return metadata, nil
}

// GetArtifactContent retrieves artifact content using V2 API
func (c *V2Client) GetArtifactContent(ctx context.Context, group, artifactID string) (interface{}, error) {
	// Get latest version content (returns raw bytes)
	content, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().ByVersion("latest").Get(ctx, nil)
	if err != nil {
		// V2 Error embeds ApiError which has GetStatusCode()
		if v2Error, ok := err.(*models.Error); ok {
			statusCode := v2Error.GetStatusCode()
			detail := ""
			if v2Error.GetDetail() != nil {
				detail = *v2Error.GetDetail()
			}
			if detail != "" {
				return nil, fmt.Errorf("failed to get artifact content (HTTP %d): %s", statusCode, detail)
			}
			return nil, fmt.Errorf("failed to get artifact content (HTTP %d)", statusCode)
		}
		return nil, fmt.Errorf("failed to get artifact content: %w", err)
	}

	return string(content), nil
}

// TestCompatibility tests schema compatibility using V2 API /test endpoint
func (c *V2Client) TestCompatibility(ctx context.Context, group, artifactID, content, version string) (bool, error) {
	// V2 /test endpoint doesn't require version parameter
	// Test compatibility using /test endpoint (takes []byte, *string contentType)
	contentBytes := []byte(content)
	contentType := "application/json"

	err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Test().Put(ctx, contentBytes, &contentType, nil)
	if err != nil {
		if isRuleViolationError(err) {
			return false, fmt.Errorf("incompatible schema change detected by registry")
		}
		return false, err
	}

	return true, nil
}

// GetSystemInfo retrieves system information from the registry using V2 API
func (c *V2Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	// Call /apis/registry/v2/system/info endpoint
	systemInfo, err := c.client.System().Info().Get(ctx, nil)
	if err != nil {
		// Check for typed error with status code
		if problemDetails, ok := err.(*models.Error); ok {
			message := ""
			if problemDetails.GetMessage() != nil {
				message = *problemDetails.GetMessage()
			}
			return nil, fmt.Errorf("failed to get system info: %s", message)
		}
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	// Convert SDK response to our SystemInfo structure
	info := &SystemInfo{
		Name:        safeString(systemInfo.GetName()),
		Description: safeString(systemInfo.GetDescription()),
		Version:     safeString(systemInfo.GetVersion()),
	}

	// Format built-on timestamp
	if systemInfo.GetBuiltOn() != nil {
		info.BuiltOn = formatKiotaTime(systemInfo.GetBuiltOn())
	}

	return info, nil
}

// GetAPIVersion returns the API version
func (c *V2Client) GetAPIVersion() config.APIVersion {
	return config.APIVersionV2
}

// CreateArtifactWithReferences is not supported in V2 API
// AsyncAPI registration with references requires V3 API
func (c *V2Client) CreateArtifactWithReferences(ctx context.Context, group, artifactID, artifactType, version, name, description, content, contentType string, references []ArtifactReference) (*ArtifactMetadata, error) {
	return nil, fmt.Errorf("artifact references not supported in V2 API, use --api-version v3")
}

// GetArtifactContentDereferenced is not supported in V2 API
func (c *V2Client) GetArtifactContentDereferenced(ctx context.Context, group, artifactID, version, references string) ([]byte, error) {
	return nil, fmt.Errorf("dereferenced content retrieval not supported in V2 API, use --api-version v3")
}

// formatV2Error formats errors from V2 API with proper context
func formatV2Error(operation string, err error, group, artifactID string) error {
	if err == nil {
		return nil
	}

	// Check for timeout errors
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: operation timed out after exceeding context deadline (group=%s, artifactId=%s)", operation, group, artifactID)
	}

	// Check for network/connection errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("%s: network timeout error (group=%s, artifactId=%s): %w", operation, group, artifactID, err)
		}
		return fmt.Errorf("%s: network error (group=%s, artifactId=%s): %w", operation, group, artifactID, err)
	}

	// Check for V2 Error type
	if v2Error, ok := err.(*models.Error); ok {
		errorCode := int32(0)
		if v2Error.GetErrorCode() != nil {
			errorCode = *v2Error.GetErrorCode()
		}
		name := ""
		if v2Error.GetName() != nil {
			name = *v2Error.GetName()
		}
		message := ""
		if v2Error.GetMessage() != nil {
			message = *v2Error.GetMessage()
		}

		return fmt.Errorf("%s: HTTP %d - %s: %s (group=%s, artifactId=%s)",
			operation, errorCode, name, message, group, artifactID)
	}

	// Check if error message contains timeout keywords
	errMsg := err.Error()
	if strings.Contains(strings.ToLower(errMsg), "timeout") ||
		strings.Contains(strings.ToLower(errMsg), "deadline exceeded") ||
		strings.Contains(strings.ToLower(errMsg), "context canceled") {
		return fmt.Errorf("%s: timeout error (group=%s, artifactId=%s): %w", operation, group, artifactID, err)
	}

	// Fallback: include context but preserve original error
	return fmt.Errorf("%s (group=%s, artifactId=%s): %w", operation, group, artifactID, err)
}

// Helper functions for V2

func convertV2ArtifactMetadata(meta models.ArtifactMetaDataable) *ArtifactMetadata {
	metadata := &ArtifactMetadata{
		ID:          safeString(meta.GetId()),
		Version:     safeString(meta.GetVersion()),
		Name:        safeString(meta.GetName()),
		Description: safeString(meta.GetDescription()),
		GroupID:     safeString(meta.GetGroupId()),
	}

	if meta.GetGlobalId() != nil {
		metadata.GlobalID = *meta.GetGlobalId()
	}
	if meta.GetContentId() != nil {
		metadata.ContentID = *meta.GetContentId()
	}
	if meta.GetCreatedOn() != nil {
		metadata.CreatedOn = formatKiotaTime(meta.GetCreatedOn())
	}
	if meta.GetModifiedOn() != nil {
		metadata.ModifiedOn = formatKiotaTime(meta.GetModifiedOn())
	}

	return metadata
}

func convertV2VersionMetadata(meta models.VersionMetaDataable) *VersionMetadata {
	metadata := &VersionMetadata{
		Version: safeString(meta.GetVersion()),
	}

	if meta.GetGlobalId() != nil {
		metadata.GlobalID = *meta.GetGlobalId()
	}
	if meta.GetContentId() != nil {
		metadata.ContentID = *meta.GetContentId()
	}
	if meta.GetCreatedOn() != nil {
		metadata.CreatedOn = formatKiotaTime(meta.GetCreatedOn())
	}

	return metadata
}

func safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

// formatKiotaTime formats a Kiota Time pointer to string
// Kiota uses a custom Time type that wraps time.Time
func formatKiotaTime(t interface{}) string {
	if t == nil {
		return ""
	}
	// The Kiota Time type has a Format method similar to time.Time
	if formatter, ok := t.(interface{ Format(string) string }); ok {
		return formatter.Format(time.RFC3339)
	}
	// Fallback: if it implements String()
	if stringer, ok := t.(interface{ String() string }); ok {
		return stringer.String()
	}
	return ""
}

func isRuleViolationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "rule violation") ||
		strings.Contains(errStr, "ruleviolation") ||
		strings.Contains(errStr, "409") ||
		strings.Contains(errStr, "incompatible")
}
