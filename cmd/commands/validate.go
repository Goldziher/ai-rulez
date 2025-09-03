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
	warnings = append(warnings, checkDuplicateMCPServers(cfg)...)
	warnings = append(warnings, checkDuplicateCommands(cfg)...)
	warnings = append(warnings, checkMCPServerLogic(cfg)...)
	warnings = append(warnings, checkTargetExistence(cfg)...)

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
		"section name",
	)
}

func checkDuplicateAgents(cfg *config.Config) []string {
	return checkDuplicateNames(
		len(cfg.Agents),
		func(i int) string { return cfg.Agents[i].Name },
		"agent name",
	)
}

func checkDuplicateMCPServers(cfg *config.Config) []string {
	return checkDuplicateNames(
		len(cfg.MCPServers),
		func(i int) string { return cfg.MCPServers[i].Name },
		"mcp_server name",
	)
}

func checkDuplicateCommands(cfg *config.Config) []string {
	return checkDuplicateNames(
		len(cfg.Commands),
		func(i int) string { return cfg.Commands[i].Name },
		"command name",
	)
}

func checkMCPServerLogic(cfg *config.Config) []string {
	var warnings []string
	for _, server := range cfg.MCPServers {
		transport := server.GetTransport()
		if transport == "http" || transport == "sse" {
			if server.URL == "" {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' has transport '%s' but is missing a 'url'", server.Name, transport))
			}
		} else if transport == "stdio" {
			if server.Command == "" {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' has transport 'stdio' but is missing a 'command'", server.Name))
			}
		}
	}
	return warnings
}

func checkTargetExistence(cfg *config.Config) []string {
	var warnings []string
	outputPaths := make(map[string]bool)
	for _, output := range cfg.Outputs {
		outputPaths[output.Path] = true
	}

	// Check rule targets
	for _, rule := range cfg.Rules {
		for _, target := range rule.Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("Rule '%s' targets non-existent output '%s'", rule.Name, target))
			}
		}
	}

	// Check agent targets
	for _, agent := range cfg.Agents {
		for _, target := range agent.Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("Agent '%s' targets non-existent output '%s'", agent.Name, target))
			}
		}
	}

	// Check MCP server targets
	for _, server := range cfg.MCPServers {
		for _, target := range server.Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' targets non-existent output '%s'", server.Name, target))
			}
		}
	}

	// Check command targets
	for _, command := range cfg.Commands {
		for _, target := range command.Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("Command '%s' targets non-existent output '%s'", command.Name, target))
			}
		}
	}

	return warnings
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
	logger.Info("  - MCP Servers:", "count", len(cfg.MCPServers))
	logger.Info("  - Commands:", "count", len(cfg.Commands))
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
