package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ymocode/apicurio-client/internal/auth"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
	"github.com/ymocode/apicurio-client/internal/output"
	"github.com/ymocode/apicurio-client/internal/schema"

	registryclientv3 "github.com/apicurio/apicurio-registry/go-sdk/v3/pkg/registryclient-v3"
	"github.com/apicurio/apicurio-registry/go-sdk/v3/pkg/registryclient-v3/groups"
	"github.com/apicurio/apicurio-registry/go-sdk/v3/pkg/registryclient-v3/models"
)

// V3Client implements the RegistryClient interface using the V3 API SDK
type V3Client struct {
	client *registryclientv3.ApiClient
	config *config.Config
}

// NewV3Client creates a new V3 API client
func NewV3Client(cfg *config.Config) (*V3Client, error) {
	adapter, err := auth.CreateKiotaAdapter(cfg, "/apis/registry/v3")
	if err != nil {
		return nil, fmt.Errorf("failed to create request adapter: %w", err)
	}

	client := registryclientv3.NewApiClient(adapter)

	return &V3Client{
		client: client,
		config: cfg,
	}, nil
}

// CreateArtifact creates a new artifact using V3 API
// V3 uses structured request body with firstVersion nested structure
func (c *V3Client) CreateArtifact(ctx context.Context, group, artifactID string, s *schema.AvroSchema) (*ArtifactMetadata, error) {
	log := logger.GetLogger()
	log.Info("Creating artifact: group=%s, artifactId=%s, version=%s", group, artifactID, s.Version)

	// Get minified content
	content, err := s.GetMinifiedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get minified content: %w", err)
	}
	log.Debug("Schema content: %d bytes (minified)", len(content))

	// Create artifact request with nested structure
	createReq := models.NewCreateArtifact()
	createReq.SetArtifactId(&artifactID)
	artifactType := "AVRO"
	createReq.SetArtifactType(&artifactType)

	// Set name from schema record name
	if s.Name != "" {
		createReq.SetName(&s.Name)
		log.Debug("Setting artifact name: %s", s.Name)
	}

	// Set description from schema doc field
	if doc := s.GetDoc(); doc != "" {
		createReq.SetDescription(&doc)
		log.Debug("Setting artifact description: %s", output.TruncateString(doc, 50))
	}

	// Create first version
	firstVersion := models.NewCreateVersion()
	firstVersion.SetVersion(&s.Version)

	// Create version content
	versionContent := models.NewVersionContent()
	versionContent.SetContent(&content)
	contentType := "application/json"
	versionContent.SetContentType(&contentType)

	firstVersion.SetContent(versionContent)
	createReq.SetFirstVersion(firstVersion)

	// Configure query parameters for smart version handling
	ifExists := models.FIND_OR_CREATE_VERSION_IFARTIFACTEXISTS
	canonical := true
	reqConfig := &groups.ItemArtifactsRequestBuilderPostRequestConfiguration{
		QueryParameters: &groups.ItemArtifactsRequestBuilderPostQueryParameters{
			IfExistsAsIfArtifactExists: &ifExists,
			Canonical:                  &canonical,
		},
	}
	log.Debug("Using ifExists=FIND_OR_CREATE_VERSION, canonical=true")

	// Create artifact with smart version handling
	timer := log.StartTimer("POST /apis/registry/v3/groups/" + group + "/artifacts")
	resp, err := c.client.Groups().ByGroupId(group).Artifacts().Post(ctx, createReq, reqConfig)
	timer.Stop()

	if err != nil {
		formattedErr := formatV3Error("failed to create artifact", err, group, artifactID)
		log.Error("%v", formattedErr)
		return nil, formattedErr
	}

	metadata := convertV3CreateArtifactResponse(resp)
	log.Info("Artifact created successfully: globalId=%d, contentId=%d", metadata.GlobalID, metadata.ContentID)
	return metadata, nil
}

// CreateVersion creates a new version using V3 API
func (c *V3Client) CreateVersion(ctx context.Context, group, artifactID string, s *schema.AvroSchema) (*VersionMetadata, error) {
	// Get minified content
	content, err := s.GetMinifiedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get minified content: %w", err)
	}

	// Create version request
	createReq := models.NewCreateVersion()
	createReq.SetVersion(&s.Version)

	// Set version description from schema doc
	if doc := s.GetDoc(); doc != "" {
		createReq.SetDescription(&doc)
	}

	// Create version content
	versionContent := models.NewVersionContent()
	versionContent.SetContent(&content)
	contentType := "application/json"
	versionContent.SetContentType(&contentType)

	createReq.SetContent(versionContent)

	// Create version
	resp, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().Post(ctx, createReq, nil)
	if err != nil {
		formattedErr := formatV3Error("failed to create version", err, group, artifactID)
		return nil, formattedErr
	}

	return convertV3VersionMetadata(resp), nil
}

// GetArtifactMetadata retrieves artifact metadata using V3 API
func (c *V3Client) GetArtifactMetadata(ctx context.Context, group, artifactID string) (*ArtifactMetadata, error) {
	// Get artifact metadata
	artifactMeta, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Get(ctx, nil)
	if err != nil {
		// Check for typed error with status code
		if problemDetails, ok := err.(*models.ProblemDetails); ok {
			status := problemDetails.GetStatus()
			if status != nil {
				statusCode := *status
				detail := ""
				if problemDetails.GetDetail() != nil {
					detail = *problemDetails.GetDetail()
				}
				return nil, fmt.Errorf("failed to get artifact metadata (HTTP %d): %s", statusCode, detail)
			}
		}
		return nil, fmt.Errorf("failed to get artifact metadata: %w", err)
	}

	// Get latest version metadata
	versionMeta, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().ByVersionExpression("branch=latest").Get(ctx, nil)
	if err != nil {
		// Check for typed error with status code
		if problemDetails, ok := err.(*models.ProblemDetails); ok {
			status := problemDetails.GetStatus()
			if status != nil {
				statusCode := *status
				detail := ""
				if problemDetails.GetDetail() != nil {
					detail = *problemDetails.GetDetail()
				}
				return nil, fmt.Errorf("failed to get version metadata (HTTP %d): %s", statusCode, detail)
			}
		}
		return nil, fmt.Errorf("failed to get version metadata: %w", err)
	}

	// Combine artifact and version metadata
	metadata := convertV3ArtifactMetadata(artifactMeta)
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

// GetArtifactContent retrieves artifact content using V3 API
func (c *V3Client) GetArtifactContent(ctx context.Context, group, artifactID string) (interface{}, error) {
	// Get latest version content (returns raw bytes)
	content, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().ByVersionExpression("branch=latest").Content().Get(ctx, nil)
	if err != nil {
		// Check for typed error with status code
		if problemDetails, ok := err.(*models.ProblemDetails); ok {
			status := problemDetails.GetStatus()
			if status != nil {
				statusCode := *status
				detail := ""
				if problemDetails.GetDetail() != nil {
					detail = *problemDetails.GetDetail()
				}
				return nil, fmt.Errorf("failed to get artifact content (HTTP %d): %s", statusCode, detail)
			}
		}
		return nil, fmt.Errorf("failed to get artifact content: %w", err)
	}

	return string(content), nil
}

// TestCompatibility tests schema compatibility using V3 API with dryRun parameter
func (c *V3Client) TestCompatibility(ctx context.Context, group, artifactID, content, version string) (bool, error) {
	// Create version for dry-run
	createReq := models.NewCreateVersion()
	createReq.SetVersion(&version)

	versionContent := models.NewVersionContent()
	versionContent.SetContent(&content)
	contentType := "application/json"
	versionContent.SetContentType(&contentType)

	createReq.SetContent(versionContent)

	// Configure request with dryRun parameter
	config := &groups.ItemArtifactsItemVersionsRequestBuilderPostRequestConfiguration{}
	config.QueryParameters = &groups.ItemArtifactsItemVersionsRequestBuilderPostQueryParameters{}
	dryRun := true
	config.QueryParameters.DryRun = &dryRun

	// Test compatibility using dryRun
	_, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().Post(ctx, createReq, config)

	if err != nil {
		// Try RuleViolationProblemDetails first (most common for compatibility checks)
		if ruleViolation, ok := err.(*models.RuleViolationProblemDetails); ok {
			status := ruleViolation.GetStatus()
			detail := ruleViolation.GetDetail()
			title := ruleViolation.GetTitle()
			causes := ruleViolation.GetCauses()

			statusCode := int32(0)
			if status != nil {
				statusCode = *status
			}

			detailStr := ""
			if detail != nil {
				detailStr = *detail
			}

			titleStr := ""
			if title != nil {
				titleStr = *title
			}

			// 409 Conflict can mean two things:
			// 1. Content already exists (identical schema) - compatible, no causes
			// 2. Rule violation (incompatible changes) - incompatible, has causes
			// Use the structured 'causes' field to distinguish
			if statusCode == 409 {
				// If there are causes, it's a rule violation (incompatible)
				if len(causes) > 0 {
					return false, fmt.Errorf("incompatible schema (HTTP %d): %s - %s", statusCode, titleStr, detailStr)
				}
				// No causes means identical content (compatible)
				return true, nil
			}

			// RuleViolationProblemDetails with other status codes indicates incompatibility
			return false, fmt.Errorf("incompatible schema (HTTP %d): %s - %s", statusCode, titleStr, detailStr)
		}

		// Try ProblemDetails for other errors
		if problemDetails, ok := err.(*models.ProblemDetails); ok {
			status := problemDetails.GetStatus()
			detail := problemDetails.GetDetail()
			title := problemDetails.GetTitle()

			statusCode := int32(0)
			if status != nil {
				statusCode = *status
			}

			detailStr := ""
			if detail != nil {
				detailStr = *detail
			}

			titleStr := ""
			if title != nil {
				titleStr = *title
			}

			// 409 Conflict - content already exists (identical schema)
			if statusCode == 409 {
				return true, nil
			}

			// Return detailed error with status code
			return false, fmt.Errorf("compatibility check failed (HTTP %d): %s - %s", statusCode, titleStr, detailStr)
		}

		// Fallback to string-based checking if type assertion fails
		if isRuleViolationError(err) {
			return false, fmt.Errorf("incompatible schema change detected by registry")
		}

		return false, fmt.Errorf("compatibility check API error: %w", err)
	}

	return true, nil
}

// GetSystemInfo retrieves system information from the registry using V3 API
func (c *V3Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	log := logger.GetLogger()
	log.Debug("Fetching system info from registry")

	// Call /apis/registry/v3/system/info endpoint
	timer := log.StartTimer("GET /apis/registry/v3/system/info")
	systemInfo, err := c.client.System().Info().Get(ctx, nil)
	timer.Stop()

	if err != nil {
		// Check for typed error with status code
		if problemDetails, ok := err.(*models.ProblemDetails); ok {
			status := problemDetails.GetStatus()
			if status != nil {
				statusCode := *status
				detail := ""
				if problemDetails.GetDetail() != nil {
					detail = *problemDetails.GetDetail()
				}
				return nil, fmt.Errorf("failed to get system info (HTTP %d): %s", statusCode, detail)
			}
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

	log.Debug("System info: name=%s, version=%s", info.Name, info.Version)
	return info, nil
}

// GetAPIVersion returns the API version
func (c *V3Client) GetAPIVersion() config.APIVersion {
	return config.APIVersionV3
}

// CreateArtifactWithReferences creates an artifact with references to other artifacts
// Used for AsyncAPI documents that reference Avro schemas
func (c *V3Client) CreateArtifactWithReferences(ctx context.Context, group, artifactID, artifactType, version, name, description, content, contentType string, references []ArtifactReference) (*ArtifactMetadata, error) {
	log := logger.GetLogger()
	log.Info("Creating artifact with references: group=%s, artifactId=%s, type=%s, version=%s, refs=%d",
		group, artifactID, artifactType, version, len(references))

	// Create artifact request
	createReq := models.NewCreateArtifact()
	createReq.SetArtifactId(&artifactID)
	createReq.SetArtifactType(&artifactType)

	if name != "" {
		createReq.SetName(&name)
	}
	if description != "" {
		createReq.SetDescription(&description)
	}

	// Create first version
	firstVersion := models.NewCreateVersion()
	firstVersion.SetVersion(&version)

	// Create version content
	versionContent := models.NewVersionContent()
	versionContent.SetContent(&content)
	versionContent.SetContentType(&contentType)

	// Add references if provided
	if len(references) > 0 {
		refs := make([]models.ArtifactReferenceable, len(references))
		for i, ref := range references {
			r := models.NewArtifactReference()
			gid := ref.GroupID
			aid := ref.ArtifactID
			ver := ref.Version
			nam := ref.Name
			r.SetGroupId(&gid)
			r.SetArtifactId(&aid)
			r.SetVersion(&ver)
			r.SetName(&nam)
			refs[i] = r
			log.Debug("Adding reference: %s", ref.Name)
		}
		versionContent.SetReferences(refs)
	}

	firstVersion.SetContent(versionContent)
	createReq.SetFirstVersion(firstVersion)

	// Configure query parameters - CREATE_VERSION if artifact exists
	ifExists := models.CREATE_VERSION_IFARTIFACTEXISTS
	reqConfig := &groups.ItemArtifactsRequestBuilderPostRequestConfiguration{
		QueryParameters: &groups.ItemArtifactsRequestBuilderPostQueryParameters{
			IfExistsAsIfArtifactExists: &ifExists,
		},
	}

	// Create artifact
	timer := log.StartTimer("POST /apis/registry/v3/groups/" + group + "/artifacts")
	resp, err := c.client.Groups().ByGroupId(group).Artifacts().Post(ctx, createReq, reqConfig)
	timer.Stop()

	if err != nil {
		formattedErr := formatV3Error("failed to create artifact", err, group, artifactID)
		log.Error("%v", formattedErr)
		return nil, formattedErr
	}

	metadata := convertV3CreateArtifactResponse(resp)
	log.Info("Artifact created successfully: globalId=%d, contentId=%d", metadata.GlobalID, metadata.ContentID)
	return metadata, nil
}

// GetArtifactContentDereferenced retrieves artifact content with references handling
// The references parameter can be "PRESERVE", "REWRITE", or "DEREFERENCE"
func (c *V3Client) GetArtifactContentDereferenced(ctx context.Context, group, artifactID, version, references string) ([]byte, error) {
	log := logger.GetLogger()
	log.Info("Getting artifact content: group=%s, artifactId=%s, version=%s, references=%s",
		group, artifactID, version, references)

	// Map string to HandleReferencesType
	var refType models.HandleReferencesType
	switch strings.ToUpper(references) {
	case "DEREFERENCE":
		refType = models.DEREFERENCE_HANDLEREFERENCESTYPE
	case "REWRITE":
		refType = models.REWRITE_HANDLEREFERENCESTYPE
	case "PRESERVE":
		refType = models.PRESERVE_HANDLEREFERENCESTYPE
	default:
		refType = models.DEREFERENCE_HANDLEREFERENCESTYPE
	}

	// Configure request with references parameter
	config := &groups.ItemArtifactsItemVersionsItemContentRequestBuilderGetRequestConfiguration{
		QueryParameters: &groups.ItemArtifactsItemVersionsItemContentRequestBuilderGetQueryParameters{
			ReferencesAsHandleReferencesType: &refType,
		},
	}

	// Get content with dereferencing
	content, err := c.client.Groups().ByGroupId(group).Artifacts().ByArtifactId(artifactID).Versions().ByVersionExpression(version).Content().Get(ctx, config)
	if err != nil {
		return nil, formatV3Error("get artifact content", err, group, artifactID)
	}

	log.Debug("Retrieved %d bytes of content", len(content))
	return content, nil
}

// formatV3Error formats errors from V3 API with proper context
func formatV3Error(operation string, err error, group, artifactID string) error {
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

	// Check for RuleViolationProblemDetails (most specific error type)
	if ruleViolation, ok := err.(*models.RuleViolationProblemDetails); ok {
		status := int32(0)
		if ruleViolation.GetStatus() != nil {
			status = *ruleViolation.GetStatus()
		}
		title := ""
		if ruleViolation.GetTitle() != nil {
			title = *ruleViolation.GetTitle()
		}
		detail := ""
		if ruleViolation.GetDetail() != nil {
			detail = *ruleViolation.GetDetail()
		}

		// Format causes if present
		causes := ruleViolation.GetCauses()
		if len(causes) > 0 {
			var causesStr []string
			for _, cause := range causes {
				causeDesc := ""
				if cause.GetDescription() != nil {
					causeDesc = *cause.GetDescription()
				}
				causeCtx := ""
				if cause.GetContext() != nil {
					causeCtx = *cause.GetContext()
				}
				if causeDesc != "" {
					if causeCtx != "" {
						causesStr = append(causesStr, fmt.Sprintf("%s (context: %s)", causeDesc, causeCtx))
					} else {
						causesStr = append(causesStr, causeDesc)
					}
				}
			}
			return fmt.Errorf("%s: HTTP %d - %s: %s (group=%s, artifactId=%s, causes: %s)",
				operation, status, title, detail, group, artifactID, strings.Join(causesStr, "; "))
		}

		return fmt.Errorf("%s: HTTP %d - %s: %s (group=%s, artifactId=%s)",
			operation, status, title, detail, group, artifactID)
	}

	// Check for generic ProblemDetails
	if problemDetails, ok := err.(*models.ProblemDetails); ok {
		status := int32(0)
		if problemDetails.GetStatus() != nil {
			status = *problemDetails.GetStatus()
		}
		title := ""
		if problemDetails.GetTitle() != nil {
			title = *problemDetails.GetTitle()
		}
		detail := ""
		if problemDetails.GetDetail() != nil {
			detail = *problemDetails.GetDetail()
		}

		return fmt.Errorf("%s: HTTP %d - %s: %s (group=%s, artifactId=%s)",
			operation, status, title, detail, group, artifactID)
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

// Helper functions for V3

func convertV3CreateArtifactResponse(resp models.CreateArtifactResponseable) *ArtifactMetadata {
	artifact := resp.GetArtifact()
	version := resp.GetVersion()

	metadata := &ArtifactMetadata{
		ID:          safeString(artifact.GetArtifactId()),
		Version:     safeString(version.GetVersion()),
		Name:        safeString(artifact.GetName()),
		Description: safeString(artifact.GetDescription()),
		GroupID:     safeString(artifact.GetGroupId()),
	}

	if version.GetGlobalId() != nil {
		metadata.GlobalID = *version.GetGlobalId()
	}
	if version.GetContentId() != nil {
		metadata.ContentID = *version.GetContentId()
	}
	if artifact.GetCreatedOn() != nil {
		metadata.CreatedOn = formatKiotaTime(artifact.GetCreatedOn())
	}
	if artifact.GetModifiedOn() != nil {
		metadata.ModifiedOn = formatKiotaTime(artifact.GetModifiedOn())
	}

	return metadata
}

func convertV3ArtifactMetadata(meta models.ArtifactMetaDataable) *ArtifactMetadata {
	metadata := &ArtifactMetadata{
		ID:          safeString(meta.GetArtifactId()),
		Name:        safeString(meta.GetName()),
		Description: safeString(meta.GetDescription()),
		GroupID:     safeString(meta.GetGroupId()),
	}

	if meta.GetCreatedOn() != nil {
		metadata.CreatedOn = formatKiotaTime(meta.GetCreatedOn())
	}
	if meta.GetModifiedOn() != nil {
		metadata.ModifiedOn = formatKiotaTime(meta.GetModifiedOn())
	}

	return metadata
}

func convertV3VersionMetadata(meta models.VersionMetaDataable) *VersionMetadata {
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
