package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/ymocode/apicurio-client/internal/config"
	"github.com/ymocode/apicurio-client/internal/logger"
	"github.com/ymocode/apicurio-client/internal/output"
	"github.com/ymocode/apicurio-client/internal/registry"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Get registry system information",
	Long:  `Retrieves system information from the Apicurio Registry including name, version, and build details.`,
	RunE:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)

	infoCmd.Flags().String("format", "json", "Output format: json, table, summary, markdown")
	infoCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
}

func runInfo(cmd *cobra.Command, args []string) error {
	log := logger.GetLogger()
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")

	log.Info("Retrieving system info")
	startTime := time.Now()

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Info("Connecting to registry: %s (API: %s)", cfg.RegistryURL, cfg.APIVersion)

	client, err := registry.NewRegistryClient(cfg)
	if err != nil {
		log.Error("Failed to create registry client: %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultOperationTimeout)
	defer cancel()

	// Use the SDK's system/info endpoint
	systemInfo, err := client.GetSystemInfo(ctx)
	duration := time.Since(startTime)

	if err != nil {
		log.Error("Failed to retrieve system info: %v", err)
		return err
	}

	// Successfully retrieved system info
	log.Info("System info retrieved (response time: %dms)", duration.Milliseconds())
	log.Info("Registry: %s v%s", systemInfo.Name, systemInfo.Version)

	infoOutput := output.SystemInfoOutput{
		RegistryURL:    cfg.RegistryURL,
		APIVersion:     string(cfg.APIVersion),
		ResponseTimeMs: duration.Milliseconds(),
		Name:           systemInfo.Name,
		Version:        systemInfo.Version,
		Description:    systemInfo.Description,
		BuiltOn:        systemInfo.BuiltOn,
	}
	return output.PrintSystemInfoResult(infoOutput, format, outputPath)
}
