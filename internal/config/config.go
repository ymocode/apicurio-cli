package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type APIVersion string

const (
	APIVersionV2      APIVersion = "v2"
	APIVersionV3      APIVersion = "v3"
	APIVersionCCOMPAT APIVersion = "ccompat"
)

type Config struct {
	RegistryURL    string
	APIVersion     APIVersion
	Group          string
	ArtifactID     string
	AuthMode       string
	Username       string
	Password       string
	KeycloakURL    string
	ClientID       string
	ClientSecret   string
	Realm          string
	Insecure       bool
	SchemaFile     string
}

func InitConfig() {
	viper.SetConfigName(".apicurio-client")
	viper.SetConfigType("yaml")

	home, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(home)
	}
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("APICURIO")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
		}
	}
}

func LoadConfig() (*Config, error) {
	apiVersion := viper.GetString("api_version")
	if apiVersion == "" {
		apiVersion = string(APIVersionV2)
	}

	return &Config{
		RegistryURL:    viper.GetString("registry_url"),
		APIVersion:     APIVersion(apiVersion),
		Group:          viper.GetString("group"),
		ArtifactID:     viper.GetString("artifact_id"),
		AuthMode:       viper.GetString("auth"),
		Username:       viper.GetString("username"),
		Password:       viper.GetString("password"),
		KeycloakURL:    viper.GetString("keycloak_url"),
		ClientID:       viper.GetString("client_id"),
		ClientSecret:   viper.GetString("client_secret"),
		Realm:          viper.GetString("realm"),
		Insecure:       viper.GetBool("insecure"),
		SchemaFile:     viper.GetString("file"),
	}, nil
}

func (c *Config) Validate() error {
	if c.RegistryURL == "" {
		return fmt.Errorf("registry URL is required")
	}

	if c.APIVersion == "" {
		c.APIVersion = APIVersionV2
	}

	switch c.APIVersion {
	case APIVersionV2, APIVersionV3, APIVersionCCOMPAT:
	default:
		return fmt.Errorf("invalid API version: %s (must be v2, v3, or ccompat)", c.APIVersion)
	}

	if c.AuthMode == "" {
		c.AuthMode = "none"
	}

	switch c.AuthMode {
	case "none":
	case "basic":
		if c.Username == "" || c.Password == "" {
			return fmt.Errorf("username and password are required for basic auth")
		}
	case "oidc":
		if c.KeycloakURL == "" || c.ClientID == "" || c.ClientSecret == "" || c.Realm == "" {
			return fmt.Errorf("keycloak-url, client-id, client-secret, and realm are required for OIDC auth")
		}
	default:
		return fmt.Errorf("invalid auth mode: %s (must be none, basic, or oidc)", c.AuthMode)
	}

	return nil
}

func (c *Config) GetFQN(namespace, name string) string {
	group := c.Group
	if group == "" {
		group = namespace
	}

	artifactID := c.ArtifactID
	if artifactID == "" {
		artifactID = name
	}

	if group == "default" || group == "" {
		return fmt.Sprintf("%s.%s", namespace, name)
	}

	return fmt.Sprintf("%s.%s", group, artifactID)
}

func (c *Config) GetGroup(namespace string) string {
	if c.Group != "" {
		return c.Group
	}
	return namespace
}

func (c *Config) GetArtifactID(name string) string {
	if c.ArtifactID != "" {
		return c.ArtifactID
	}
	return name
}

func GetConfigFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".apicurio-client.yaml"
	}
	return filepath.Join(home, ".apicurio-client.yaml")
}
