package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

// ValidateCmd represents the validate command
var ValidateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate AI rules configuration file",
	Long: `Validate an AI rules configuration file for syntax errors, 
schema compliance, and structural issues.`,
	Aliases: []string{"val", "v", "check"},
	Args:    cobra.MaximumNArgs(1),
	Run:     runValidate,
}

func runValidate(cmd *cobra.Command, args []string) {
	configPath := determineConfigPath(args)
	cfg := loadAndValidateConfig(configPath)
	warnings := performValidationChecks(cfg)
	displayValidationResults(configPath, cfg, warnings)
}

func determineConfigPath(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	if cfgFile != "" {
		return cfgFile
	}

	// Auto-discover config file
	configPath, err := config.FindConfigFile(".")
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	return configPath
}

func loadAndValidateConfig(configPath string) *config.Config {
	cfg, err := config.LoadConfigWithIncludes(context.Background(), configPath)
	if err != nil {
		logger.Error("❌ Configuration file is invalid", "path", configPath)
		fmtError(err)
		os.Exit(1)
	}
	
	// Perform configuration validation
	if err := cfg.Validate(); err != nil {
		logger.Error("❌ Configuration validation failed", "path", configPath)
		fmtError(err)
		os.Exit(1)
	}
	
	return cfg
}

func performValidationChecks(cfg *config.Config) []string {
	var warnings []string

	warnings = append(warnings, checkEmptyConfiguration(cfg)...)
	warnings = append(warnings, checkOutputs(cfg)...)
	warnings = append(warnings, checkDuplicateRules(cfg)...)
	warnings = append(warnings, checkDuplicateSections(cfg)...)
	warnings = append(warnings, checkDuplicateAgents(cfg)...)

	return warnings
}

func checkEmptyConfiguration(cfg *config.Config) []string {
	if len(cfg.Rules) == 0 && len(cfg.Sections) == 0 {
		return []string{"Configuration has no rules or sections defined"}
	}
	return nil
}

func checkOutputs(cfg *config.Config) []string {
	if len(cfg.Outputs) == 0 {
		return []string{"No output files configured - generation will have no effect"}
	}
	return nil
}

func checkDuplicateRules(cfg *config.Config) []string {
	return checkDuplicateNames(
		len(cfg.Rules),
		func(i int) string { return cfg.Rules[i].Name },
		"rule",
	)
}

func checkDuplicateSections(cfg *config.Config) []string {
	return checkDuplicateNames(
		len(cfg.Sections),
		func(i int) string { return cfg.Sections[i].Name },
		"section title",
	)
}

func checkDuplicateAgents(cfg *config.Config) []string {
	return checkDuplicateNames(
		len(cfg.Agents),
		func(i int) string { return cfg.Agents[i].Name },
		"agent name",
	)
}

func checkDuplicateNames(count int, getName func(int) string, itemType string) []string {
	names := make(map[string]bool)
	var warnings []string

	for i := 0; i < count; i++ {
		name := getName(i)
		if names[name] {
			warnings = append(warnings, fmt.Sprintf("Duplicate %s: '%s'", itemType, name))
		}
		names[name] = true
	}

	return warnings
}

func displayValidationResults(configPath string, cfg *config.Config, warnings []string) {
	logger.Success("✅ Configuration file is valid", "path", configPath)
	displayConfigurationSummary(cfg)
	displayWarnings(warnings)
}

func displayConfigurationSummary(cfg *config.Config) {
	logger.Info("\nConfiguration summary:")
	logger.Info("  - Rules:", "count", len(cfg.Rules))
	logger.Info("  - Sections:", "count", len(cfg.Sections))
	logger.Info("  - Agents:", "count", len(cfg.Agents))
	logger.Info("  - Outputs:", "count", len(cfg.Outputs))
	if len(cfg.Includes) > 0 {
		logger.Info("  - Includes:", "count", len(cfg.Includes))
	}
}

func displayWarnings(warnings []string) {
	if len(warnings) > 0 {
		logger.Warn("\n⚠️  Warnings:")
		for _, warning := range warnings {
			logger.Warn("  - %s", warning)
		}
	}
}
