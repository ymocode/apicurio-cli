package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
)

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

	viper.BindPFlag("registry_url", rootCmd.PersistentFlags().Lookup("registry-url"))
	viper.BindPFlag("api_version", rootCmd.PersistentFlags().Lookup("api-version"))
	viper.BindPFlag("group", rootCmd.PersistentFlags().Lookup("group"))
	viper.BindPFlag("artifact_id", rootCmd.PersistentFlags().Lookup("artifact-id"))
	viper.BindPFlag("auth", rootCmd.PersistentFlags().Lookup("auth"))
	viper.BindPFlag("username", rootCmd.PersistentFlags().Lookup("username"))
	viper.BindPFlag("password", rootCmd.PersistentFlags().Lookup("password"))
	viper.BindPFlag("keycloak_url", rootCmd.PersistentFlags().Lookup("keycloak-url"))
	viper.BindPFlag("client_id", rootCmd.PersistentFlags().Lookup("client-id"))
	viper.BindPFlag("client_secret", rootCmd.PersistentFlags().Lookup("client-secret"))
	viper.BindPFlag("realm", rootCmd.PersistentFlags().Lookup("realm"))
	viper.BindPFlag("insecure", rootCmd.PersistentFlags().Lookup("insecure"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
}

func initConfig() {
	config.InitConfig()

	// Initialize logger based on flags
	verbose := viper.GetBool("verbose")
	debug := viper.GetBool("debug")
	logger.InitLogger(verbose, debug)
}
