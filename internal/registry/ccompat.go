// Package registry provides clients for Apicurio Registry API versions.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/ymocode/apicurio-client/internal/auth"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/schema"
)

type CCompatClient struct {
	httpClient *http.Client
	config     *config.Config
	baseURL    string
}

// Confluent Schema Registry compatible request/response structures based on openapi_ccompat.json
type ccompatRegisterSchemaRequest struct {
	Schema     string                   `json:"schema"`
	SchemaType string                   `json:"schemaType,omitempty"`
	References []ccompatSchemaReference `json:"references,omitempty"`
}

type ccompatSchemaReference struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Version int    `json:"version"`
}

type ccompatSchemaID struct {
	ID int `json:"id"`
}

type ccompatSchema struct {
	Subject    string                   `json:"subject"`
	Schema     string                   `json:"schema"`
	SchemaType string                   `json:"schemaType,omitempty"`
	References []ccompatSchemaReference `json:"references,omitempty"`
	Version    int                      `json:"version"`
	ID         int                      `json:"id"`
}

type ccompatCompatibilityCheckResponse struct {
	IsCompatible bool `json:"is_compatible"`
}

func NewCCompatClient(cfg *config.Config) (*CCompatClient, error) {
	httpClient, err := auth.CreateHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	baseURL := strings.TrimSuffix(cfg.RegistryURL, "/") + "/apis/ccompat/v7"

	return &CCompatClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		config:     cfg,
	}, nil
}

func (c *CCompatClient) CreateArtifact(ctx context.Context, group, artifactID string, s *schema.AvroSchema) (*ArtifactMetadata, error) {
	// Get minified content
	content, err := s.GetMinifiedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get minified content: %w", err)
	}

	// In Confluent API, subject = artifactID (or can be group.artifactID)
	subject := c.buildSubject(group, artifactID)
	url := fmt.Sprintf("%s/subjects/%s/versions", c.baseURL, subject)

	reqBody := ccompatRegisterSchemaRequest{
		Schema:     content,
		SchemaType: "AVRO",
		References: []ccompatSchemaReference{},
	}

	respData, err := c.doRequest(ctx, "POST", url, reqBody, group)
	if err != nil {
		return nil, err
	}

	var resp ccompatSchemaID
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Get full metadata
	return c.GetArtifactMetadata(ctx, group, artifactID)
}

func (c *CCompatClient) CreateVersion(ctx context.Context, group, artifactID string, s *schema.AvroSchema) (*VersionMetadata, error) {
	// Get minified content
	content, err := s.GetMinifiedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get minified content: %w", err)
	}

	subject := c.buildSubject(group, artifactID)
	url := fmt.Sprintf("%s/subjects/%s/versions", c.baseURL, subject)

	reqBody := ccompatRegisterSchemaRequest{
		Schema:     content,
		SchemaType: "AVRO",
		References: []ccompatSchemaReference{},
	}

	respData, err := c.doRequest(ctx, "POST", url, reqBody, group)
	if err != nil {
		return nil, err
	}

	var resp ccompatSchemaID
	err = json.Unmarshal(respData, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Get the version that was created
	metadata, err := c.GetArtifactMetadata(ctx, group, artifactID)
	if err != nil {
		return nil, err
	}

	return &VersionMetadata{
		Version:    metadata.Version,
		GlobalID:   metadata.GlobalID,
		ContentID:  metadata.ContentID,
		CreatedOn:  metadata.CreatedOn,
		ModifiedOn: "",
	}, nil
}

func (c *CCompatClient) GetArtifactMetadata(ctx context.Context, group, artifactID string) (*ArtifactMetadata, error) {
	subject := c.buildSubject(group, artifactID)
	url := fmt.Sprintf("%s/subjects/%s/versions/latest", c.baseURL, subject)

	respData, err := c.doRequest(ctx, "GET", url, nil, group)
	if err != nil {
		return nil, err
	}

	var schema ccompatSchema
	if err := json.Unmarshal(respData, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &ArtifactMetadata{
		ID:        artifactID,
		Version:   fmt.Sprintf("%d", schema.Version),
		GlobalID:  int64(schema.ID),
		ContentID: int64(schema.ID),
		GroupID:   group,
		Name:      artifactID,
	}, nil
}

func (c *CCompatClient) GetArtifactContent(ctx context.Context, group, artifactID string) (interface{}, error) {
	subject := c.buildSubject(group, artifactID)
	url := fmt.Sprintf("%s/subjects/%s/versions/latest", c.baseURL, subject)

	respData, err := c.doRequest(ctx, "GET", url, nil, group)
	if err != nil {
		return nil, err
	}

	var schema ccompatSchema
	if err := json.Unmarshal(respData, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Parse the schema string as JSON
	var content interface{}
	if err := json.Unmarshal([]byte(schema.Schema), &content); err != nil {
		// If it's not JSON, return the raw string
		return schema.Schema, nil
	}

	return content, nil
}

func (c *CCompatClient) TestCompatibility(ctx context.Context, group, artifactID, content, version string) (bool, error) {
	subject := c.buildSubject(group, artifactID)
	// CCOMPAT: POST /compatibility/subjects/{subject}/versions/{version}
	// Using "latest" to check against the current version
	url := fmt.Sprintf("%s/compatibility/subjects/%s/versions/latest", c.baseURL, subject)

	reqBody := ccompatRegisterSchemaRequest{
		Schema:     content,
		SchemaType: "AVRO",
		References: []ccompatSchemaReference{},
	}

	respData, err := c.doRequest(ctx, "POST", url, reqBody, group)
	if err != nil {
		// Check if it's a compatibility error
		if strings.Contains(err.Error(), "incompatible") || strings.Contains(err.Error(), "409") {
			return false, err
		}
		return false, err
	}

	var resp ccompatCompatibilityCheckResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.IsCompatible {
		return false, fmt.Errorf("incompatible schema change detected by registry")
	}

	return true, nil
}

// GetSystemInfo retrieves system information from the registry
// Note: CCOMPAT API doesn't have a native system info endpoint,
// so we call the V3 system/info endpoint directly
func (c *CCompatClient) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	// CCOMPAT doesn't have a native system info endpoint
	// Use the V3 endpoint: /apis/registry/v3/system/info
	baseURL := strings.TrimSuffix(c.config.RegistryURL, "/")
	url := fmt.Sprintf("%s/apis/registry/v3/system/info", baseURL)

	respData, err := c.doRequestRaw(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	// Parse the V3 system info response
	var systemInfoResp struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		BuiltOn     string `json:"builtOn"`
	}
	if err := json.Unmarshal(respData, &systemInfoResp); err != nil {
		return nil, fmt.Errorf("failed to parse system info: %w", err)
	}

	return &SystemInfo{
		Name:        systemInfoResp.Name,
		Description: systemInfoResp.Description,
		Version:     systemInfoResp.Version,
		BuiltOn:     systemInfoResp.BuiltOn,
	}, nil
}

func (c *CCompatClient) GetAPIVersion() config.APIVersion {
	return config.APIVersionCCOMPAT
}

// CreateArtifactWithReferences is not supported in CCOMPAT API
// AsyncAPI registration with references requires V3 API
func (c *CCompatClient) CreateArtifactWithReferences(ctx context.Context, group, artifactID, artifactType, version, name, description, content, contentType string, references []ArtifactReference) (*ArtifactMetadata, error) {
	return nil, fmt.Errorf("artifact references not supported in CCOMPAT API, use --api-version v3")
}

// GetArtifactContentDereferenced is not supported in CCOMPAT API
func (c *CCompatClient) GetArtifactContentDereferenced(ctx context.Context, group, artifactID, version, references string) ([]byte, error) {
	return nil, fmt.Errorf("dereferenced content retrieval not supported in CCOMPAT API, use --api-version v3")
}

func (c *CCompatClient) buildSubject(group, artifactID string) string {
	// In Confluent API, subjects are typically just the artifact ID
	// But we can namespace them with group if needed
	if group != "" && group != "default" {
		return fmt.Sprintf("%s-%s", group, artifactID)
	}
	return artifactID
}

func (c *CCompatClient) doRequest(ctx context.Context, method, url string, body interface{}, group string) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.schemaregistry.v1+json")
	}

	// Add X-Registry-GroupId header if group is not default
	if group != "" && group != "default" {
		req.Header.Set("X-Registry-GroupId", group)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Check for timeout errors
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("operation timed out after exceeding context deadline: %w", err)
		}

		// Check for network/connection errors
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				return nil, fmt.Errorf("network timeout error: %w", err)
			}
			return nil, fmt.Errorf("network error: %w", err)
		}

		// Check if error message contains timeout keywords
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "timeout") ||
			strings.Contains(strings.ToLower(errMsg), "deadline exceeded") ||
			strings.Contains(strings.ToLower(errMsg), "context canceled") {
			return nil, fmt.Errorf("timeout error: %w", err)
		}

		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respData))
	}

	return respData, nil
}

// doRequestRaw makes a raw HTTP request without CCOMPAT-specific headers
func (c *CCompatClient) doRequestRaw(ctx context.Context, method, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Check for timeout errors
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("operation timed out after exceeding context deadline: %w", err)
		}

		// Check for network/connection errors
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				return nil, fmt.Errorf("network timeout error: %w", err)
			}
			return nil, fmt.Errorf("network error: %w", err)
		}

		// Check if error message contains timeout keywords
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "timeout") ||
			strings.Contains(strings.ToLower(errMsg), "deadline exceeded") ||
			strings.Contains(strings.ToLower(errMsg), "context canceled") {
			return nil, fmt.Errorf("timeout error: %w", err)
		}

		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respData))
	}

	return respData, nil
}
