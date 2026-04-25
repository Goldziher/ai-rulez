package commands

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var ValidateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate AI rules configuration file",
	Long: `Validate an AI rules configuration file for syntax errors,
schema compliance, and structural issues.`,
	Aliases: []string{"val", "v", "check"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		workingDir := "."
		if len(args) > 0 {
			workingDir = filepath.Dir(args[0])
		}

		cfg, err := config.LoadConfig(ctx, workingDir)
		if err != nil {
			logger.Error("Failed to load config", "path", workingDir)
			fmtError(err)
			os.Exit(1)
		}

		if err := cfg.Validate(); err != nil {
			logger.Error("Configuration validation failed", "path", workingDir)
			fmtError(err)
			os.Exit(1)
		}

		configPath := filepath.Join(workingDir, ".ai-rulez")
		logger.Success("Configuration is valid", "path", configPath)
		displayConfigurationSummary(cfg)
	},
}

func displayConfigurationSummary(cfg *config.Config) {
	logger.Info("\nConfiguration summary:")

	if cfg.Content != nil {
		logger.Info("  - Rules:", "count", len(cfg.Content.Rules))
		logger.Info("  - Context files:", "count", len(cfg.Content.Context))
		logger.Info("  - Skills:", "count", len(cfg.Content.Skills))
		logger.Info("  - Domains:", "count", len(cfg.Content.Domains))

		var totalDomainRules, totalDomainContext, totalDomainSkills int
		for _, domain := range cfg.Content.Domains {
			totalDomainRules += len(domain.Rules)
			totalDomainContext += len(domain.Context)
			totalDomainSkills += len(domain.Skills)
		}
		if totalDomainRules > 0 {
			logger.Info("    - Domain rules:", "count", totalDomainRules)
		}
		if totalDomainContext > 0 {
			logger.Info("    - Domain context:", "count", totalDomainContext)
		}
		if totalDomainSkills > 0 {
			logger.Info("    - Domain skills:", "count", totalDomainSkills)
		}
	}

	logger.Info("  - Presets:", "count", len(cfg.Presets))
}
