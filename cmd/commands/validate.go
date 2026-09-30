package commands

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/schema"
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

		cfg, err := loadConfigForCommand(ctx, args)
		if err != nil {
			logger.Error("Failed to load config")
			fmtError(err)
			os.Exit(1)
		}

		// Validate the raw file against the JSON schema first so a key the
		// struct would silently drop (and a value outside an enum) is reported.
		// V3 configs keep the looser Go validation only: the schema is V4-shaped.
		if !cfg.IsV3() && cfg.ConfigFile != "" {
			configPath := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
			if err := schema.ValidateFile(configPath); err != nil {
				logger.Error("Configuration failed schema validation", "path", configPath)
				fmtError(err)
				os.Exit(1)
			}
		}

		if err := cfg.Validate(); err != nil {
			logger.Error("Configuration validation failed", "path", cfg.ConfigDir)
			fmtError(err)
			os.Exit(1)
		}

		logger.Success("Configuration is valid", "path", cfg.ConfigDir)
		presets.WarnDuplicateContent(cfg.Content)
		displayConfigurationSummary(cfg)
	},
}

func init() {
	ValidateCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
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
