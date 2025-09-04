package validator

import (
	"context"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Validator handles configuration validation
type Validator struct {
	configFile string
}

// NewValidator creates a new validator instance
func NewValidator(configFile string) (*Validator, error) {
	return &Validator{
		configFile: configFile,
	}, nil
}

// Validate validates the configuration and returns warnings
func (v *Validator) Validate(ctx context.Context) ([]string, error) {
	cfg, err := config.LoadConfigWithIncludes(ctx, v.configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	var warnings []string

	// Check for empty configuration
	if len(cfg.Rules) == 0 && len(cfg.Sections) == 0 {
		warnings = append(warnings, "Configuration has no rules or sections defined")
	}

	// Check for outputs
	if len(cfg.Outputs) == 0 {
		warnings = append(warnings, "No output files configured - generation will have no effect")
	}

	// Check for duplicate rules
	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Rules),
		func(i int) string { return cfg.Rules[i].Name },
		"rule",
	)...)

	// Check for duplicate sections
	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Sections),
		func(i int) string { return cfg.Sections[i].Name },
		"section",
	)...)

	// Check for duplicate agents
	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Agents),
		func(i int) string { return cfg.Agents[i].Name },
		"agent",
	)...)

	// Check for duplicate MCP servers
	warnings = append(warnings, checkDuplicateNames(
		len(cfg.MCPServers),
		func(i int) string { return cfg.MCPServers[i].Name },
		"mcp_server",
	)...)

	// Check for duplicate commands
	warnings = append(warnings, checkDuplicateNames(
		len(cfg.Commands),
		func(i int) string { return cfg.Commands[i].Name },
		"command",
	)...)

	// Check MCP server logic
	for _, server := range cfg.MCPServers {
		transport := server.GetTransport()
		switch transport {
		case "http", "sse":
			if server.URL == "" {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' has transport '%s' but is missing a 'url'", server.Name, transport))
			}
		case "stdio":
			if server.Command == "" {
				warnings = append(warnings, fmt.Sprintf("MCP server '%s' has transport 'stdio' but is missing a 'command'", server.Name))
			}
		}
	}

	// Check target existence
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

	return warnings, nil
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
