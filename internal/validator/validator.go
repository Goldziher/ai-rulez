package validator

import (
	"context"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
)

type Validator struct {
	configFile string
}

func NewValidator(configFile string) (*Validator, error) {
	return &Validator{
		configFile: configFile,
	}, nil
}

func (v *Validator) Validate(ctx context.Context) ([]string, error) {
	cfg, err := config.LoadConfigWithIncludes(ctx, v.configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	warnings := make([]string, 0, 16)

	warnings = append(warnings, v.checkEmptyConfiguration(cfg)...)
	warnings = append(warnings, v.checkOutputsExist(cfg)...)
	warnings = append(warnings, v.checkDuplicateNames(cfg)...)
	warnings = append(warnings, v.checkMCPServerLogic(cfg)...)
	warnings = append(warnings, v.checkTargetExistence(cfg)...)

	return warnings, nil
}

func (v *Validator) checkEmptyConfiguration(cfg *config.Config) []string {
	var warnings []string
	if len(cfg.Rules) == 0 && len(cfg.Sections) == 0 {
		warnings = append(warnings, "Configuration has no rules or sections defined")
	}
	return warnings
}

func (v *Validator) checkOutputsExist(cfg *config.Config) []string {
	var warnings []string
	if len(cfg.Outputs) == 0 {
		warnings = append(warnings, "No output files configured - generation will have no effect")
	}
	return warnings
}

func (v *Validator) checkDuplicateNames(cfg *config.Config) []string {
	maxPossibleDuplicates := len(cfg.Rules) + len(cfg.Sections) + len(cfg.Agents) + len(cfg.MCPServers) + len(cfg.Commands)
	warnings := make([]string, 0, maxPossibleDuplicates)

	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Rules),
		func(i int) string { return cfg.Rules[i].Name },
		"rule",
	)...)

	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Sections),
		func(i int) string { return cfg.Sections[i].Name },
		"section",
	)...)

	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Agents),
		func(i int) string { return cfg.Agents[i].Name },
		"agent",
	)...)

	warnings = append(warnings, checkDuplicateNames(
		len(cfg.MCPServers),
		func(i int) string { return cfg.MCPServers[i].Name },
		"mcp_server",
	)...)

	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Commands),
		func(i int) string { return cfg.Commands[i].Name },
		"command",
	)...)

	return warnings
}

func (v *Validator) checkMCPServerLogic(cfg *config.Config) []string {
	var warnings []string
	for i := range cfg.MCPServers {
		transport := cfg.MCPServers[i].GetTransport()
		switch transport {
		case "http", "sse":
			if cfg.MCPServers[i].URL == "" {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' has transport '%s' but is missing a 'url'", cfg.MCPServers[i].Name, transport))
			}
		case "stdio":
			if cfg.MCPServers[i].Command == "" {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' has transport 'stdio' but is missing a 'command'", cfg.MCPServers[i].Name))
			}
		}
	}
	return warnings
}

func (v *Validator) checkTargetExistence(cfg *config.Config) []string {
	var warnings []string

	outputPaths := make(map[string]bool)
	for _, output := range cfg.Outputs {
		outputPaths[output.Path] = true
	}

	for _, rule := range cfg.Rules {
		for _, target := range rule.Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("Rule '%s' targets non-existent output '%s'", rule.Name, target))
			}
		}
	}

	for i := range cfg.Agents {
		for _, target := range cfg.Agents[i].Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("Agent '%s' targets non-existent output '%s'", cfg.Agents[i].Name, target))
			}
		}
	}

	for i := range cfg.MCPServers {
		for _, target := range cfg.MCPServers[i].Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' targets non-existent output '%s'", cfg.MCPServers[i].Name, target))
			}
		}
	}

	for i := range cfg.Commands {
		for _, target := range cfg.Commands[i].Targets {
			if !outputPaths[target] {
				warnings = append(warnings, fmt.Sprintf("Command '%s' targets non-existent output '%s'", cfg.Commands[i].Name, target))
			}
		}
	}

	return warnings
}

func checkDuplicateNames(count int, getName func(int) string, itemType string) []string {
	names := make(map[string]bool, count)
	warnings := make([]string, 0, count/4)

	for i := 0; i < count; i++ {
		name := getName(i)
		if names[name] {
			warnings = append(warnings, fmt.Sprintf("Duplicate %s: '%s'", itemType, name))
		}
		names[name] = true
	}

	return warnings
}
