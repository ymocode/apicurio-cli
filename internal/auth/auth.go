// Package auth provides authentication mechanisms for Apicurio Registry API.
package auth

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/ymocode/apicurio-client/internal/config"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	auth "github.com/microsoft/kiota-abstractions-go/authentication"
	kiotaHttp "github.com/microsoft/kiota-http-go"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// BasicAuthProvider implements Kiota AuthenticationProvider for basic auth
type BasicAuthProvider struct {
	Username string
	Password string
}

func (p *BasicAuthProvider) AuthenticateRequest(ctx context.Context, request *abstractions.RequestInformation, additionalAuthenticationContext map[string]interface{}) error {
	if request == nil {
		return nil
	}
	// Set basic auth header with proper base64 encoding
	credentials := base64.StdEncoding.EncodeToString([]byte(p.Username + ":" + p.Password))
	basicAuth := "Basic " + credentials
	request.Headers.Add("Authorization", basicAuth)
	return nil
}

// BearerTokenProvider implements Kiota AuthenticationProvider for OAuth2 bearer tokens
type BearerTokenProvider struct {
}

func (p *BearerTokenProvider) AuthenticateRequest(ctx context.Context, request *abstractions.RequestInformation, additionalAuthenticationContext map[string]interface{}) error {
	// The HTTP client already has OAuth2 transport configured
	// Kiota will use this client for requests
	return nil
}

// CreateHTTPClient creates an HTTP client with appropriate authentication and TLS configuration
func CreateHTTPClient(cfg *config.Config) (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
			MinVersion:         tls.VersionTLS12,
		},
	}

	var httpClient *http.Client

	switch cfg.AuthMode {
	case "none":
		httpClient = &http.Client{
			Transport: transport,
		}

	case "basic":
		httpClient = &http.Client{
			Transport: &basicAuthTransport{
				Transport: transport,
				Username:  cfg.Username,
				Password:  cfg.Password,
			},
		}

	case "oidc":
		tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
			strings.TrimSuffix(cfg.KeycloakURL, "/"), cfg.Realm)

		oauthConfig := &clientcredentials.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			TokenURL:     tokenURL,
		}

		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
			Transport: transport,
		})

		httpClient = oauthConfig.Client(ctx)

		if existingTransport, ok := httpClient.Transport.(*oauth2.Transport); ok {
			existingTransport.Base = transport
		}

	default:
		return nil, fmt.Errorf("unsupported auth mode: %s", cfg.AuthMode)
	}

	return httpClient, nil
}

// createHTTPClientForKiota creates an HTTP client suitable for Kiota with proper TLS configuration
func createHTTPClientForKiota(cfg *config.Config) *http.Client {
	// Create a properly configured HTTP transport with TLS settings
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
			MinVersion:         tls.VersionTLS12,
		},
	}

	// Create HTTP client with our custom transport
	// We're using a simple client instead of GetDefaultClient to ensure we have full control over TLS
	// Kiota middleware can be problematic when trying to configure the underlying transport
	return &http.Client{
		Transport: transport,
	}
}

// CreateKiotaAdapter creates a reusable Kiota request adapter with authentication
// This is the primary factory function for SDK clients (V2 and V3)
func CreateKiotaAdapter(cfg *config.Config, apiPath string) (*kiotaHttp.NetHttpRequestAdapter, error) {
	// Select authentication provider
	var authProvider auth.AuthenticationProvider

	switch cfg.AuthMode {
	case "basic":
		authProvider = &BasicAuthProvider{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	case "oidc":
		// For OIDC, we use anonymous provider but configure the HTTP client with OAuth2
		authProvider = &BearerTokenProvider{}
	default:
		authProvider = &auth.AnonymousAuthenticationProvider{}
	}

	// Create HTTP client
	httpClient := createHTTPClientForKiota(cfg)

	// For OIDC, wrap with OAuth2 transport
	if cfg.AuthMode == "oidc" {
		oauthClient, err := CreateHTTPClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth2 client: %w", err)
		}
		httpClient = oauthClient
	}

	// Create Kiota adapter
	adapter, err := kiotaHttp.NewNetHttpRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		authProvider, nil, nil, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create request adapter: %w", err)
	}

	// Set base URL
	baseURL := strings.TrimSuffix(cfg.RegistryURL, "/") + apiPath
	adapter.SetBaseUrl(baseURL)

	return adapter, nil
}

type basicAuthTransport struct {
	Transport http.RoundTripper
	Username  string
	Password  string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.Username, t.Password)
	return t.Transport.RoundTrip(req)
}
