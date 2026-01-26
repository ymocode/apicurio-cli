package cli

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
)

// mustBindPFlag binds a viper key to a cobra flag, panics on error.
func mustBindPFlag(key, flagName string) {
	if err := viper.BindPFlag(key, rootCmd.PersistentFlags().Lookup(flagName)); err != nil {
		log.Fatalf("failed to bind flag %s: %v", flagName, err)
	}
}

var rootCmd = &cobra.Command{
	Use:   "apicurio-client",
	Short: "CLI client for Apicurio Registry 3.1",
	Long:  `A production-ready CLI client for managing Avro schemas in Apicurio Registry 3.1.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Check required flags after Viper has loaded environment variables
		// This allows APICURIO_REGISTRY_URL to satisfy the requirement
		registryURL := viper.GetString("registry_url")
		if registryURL == "" {
			return fmt.Errorf("required flag \"registry-url\" not set (can also be set via APICURIO_REGISTRY_URL environment variable)")
		}
		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().String("registry-url", "", "Apicurio Registry URL (required)")
	rootCmd.PersistentFlags().String("api-version", "v2", "API version to use: v2, v3, or ccompat")
	rootCmd.PersistentFlags().String("group", "", "Group ID (defaults to namespace from schema)")
	rootCmd.PersistentFlags().String("artifact-id", "", "Artifact ID (defaults to name from schema)")

	rootCmd.PersistentFlags().String("auth", "none", "Authentication mode: none, basic, or oidc")
	rootCmd.PersistentFlags().String("username", "", "Username for basic auth")
	rootCmd.PersistentFlags().String("password", "", "Password for basic auth")
	rootCmd.PersistentFlags().String("keycloak-url", "", "Keycloak URL for OIDC auth")
	rootCmd.PersistentFlags().String("client-id", "", "Client ID for OIDC auth")
	rootCmd.PersistentFlags().String("client-secret", "", "Client secret for OIDC auth")
	rootCmd.PersistentFlags().String("realm", "", "Realm for OIDC auth")

	rootCmd.PersistentFlags().Bool("insecure", false, "Skip TLS certificate verification")
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable verbose logging")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug logging (implies --verbose)")

	mustBindPFlag("registry_url", "registry-url")
	mustBindPFlag("api_version", "api-version")
	mustBindPFlag("group", "group")
	mustBindPFlag("artifact_id", "artifact-id")
	mustBindPFlag("auth", "auth")
	mustBindPFlag("username", "username")
	mustBindPFlag("password", "password")
	mustBindPFlag("keycloak_url", "keycloak-url")
	mustBindPFlag("client_id", "client-id")
	mustBindPFlag("client_secret", "client-secret")
	mustBindPFlag("realm", "realm")
	mustBindPFlag("insecure", "insecure")
	mustBindPFlag("verbose", "verbose")
	mustBindPFlag("debug", "debug")
}

func initConfig() {
	config.InitConfig()

	// Initialize logger based on flags
	verbose := viper.GetBool("verbose")
	debug := viper.GetBool("debug")
	logger.InitLogger(verbose, debug)
}
