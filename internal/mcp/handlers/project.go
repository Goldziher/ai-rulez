package handlers

import (
	"context"
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/Goldziher/ai-rulez/internal/validator"
	"github.com/mark3labs/mcp-go/mcp"
)

func GenerateOutputsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "ai_rulez.yaml")

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return crud.ToolError(err)
	}

	gen := generator.NewWithConfigFile(configFile)

	if err := gen.GenerateAll(cfg); err != nil {
		return crud.ToolError(err)
	}

	results := make([]string, len(cfg.Outputs))
	for i, output := range cfg.Outputs {
		results[i] = output.Path
	}

	return crud.ToolSuccess(map[string]interface{}{
		"message": "Outputs generated successfully",
		"config":  configFile,
		"results": results,
	})
}

func ValidateConfigHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "ai_rulez.yaml")

	val, err := validator.NewValidator(configFile)
	if err != nil {
		return crud.ToolError(err)
	}

	warnings, err := val.Validate(ctx)
	if err != nil {
		result := map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		}
		return crud.ToolSuccess(result)
	}

	result := map[string]interface{}{
		"valid":    true,
		"warnings": warnings,
	}

	return crud.ToolSuccess(result)
}

func InitProjectHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectName := request.GetString("project_name", "")
	providersInterface := request.GetArguments()["providers"]
	_ = request.GetBool("with_agents", false)
	allProviders := request.GetBool("all_providers", false)
	popularProviders := request.GetBool("popular_providers", false)

	var providers []interface{}
	if providersSlice, ok := providersInterface.([]interface{}); ok {
		providers = providersSlice
	}

	providerConfig := templates.ProviderConfig{}
	switch {
	case allProviders:
		providerConfig = templates.ProviderConfig{Claude: true, Cursor: true, Windsurf: true, Copilot: true, Gemini: true, Amp: true, Codex: true, Cline: true, ContinueDev: true}
	case popularProviders:
		providerConfig = templates.ProviderConfig{Claude: true, Cursor: true, Windsurf: true, Copilot: true}
	default:
		for _, p := range providers {
			if provider, ok := p.(string); ok {
				switch provider {
				case "claude":
					providerConfig.Claude = true
				case "cursor":
					providerConfig.Cursor = true
				case "windsurf":
					providerConfig.Windsurf = true
				case "copilot":
					providerConfig.Copilot = true
				case "continue-dev":
					providerConfig.ContinueDev = true
				}
			}
		}
	}

	configContent := templates.GenerateConfigTemplate(projectName, providerConfig)
	if err := os.WriteFile("ai_rulez.yaml", []byte(configContent), 0o644); err != nil {
		return crud.ToolError(fmt.Errorf("failed to write config file: %w", err))
	}

	if providerConfig.ContinueDev {
		if err := crud.CreateContinueDevConfig(); err != nil {
			return crud.ToolError(fmt.Errorf("failed to create continue.dev config: %w", err))
		}
	}

	return crud.ToolSuccess(map[string]interface{}{
		"message": "Project initialized successfully",
		"path":    "ai_rulez.yaml",
	})
}
