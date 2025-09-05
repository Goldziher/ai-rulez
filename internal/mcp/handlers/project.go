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

// Helper function to convert provider names to presets
func getPresetsFromProviders(providers []interface{}, allProviders, popularProviders bool) ([]string, bool) {
	if allProviders {
		return []string{"claude", "cursor", "windsurf", "copilot", "gemini", "amp", "codex", "cline", "continue-dev"}, true
	}
	if popularProviders {
		return []string{"popular"}, false
	}
	
	var presets []string
	var hasContinueDev bool
	
	// Map of provider names to preset names (most are the same)
	providerMap := map[string]string{
		"claude": "claude",
		"cursor": "cursor",
		"windsurf": "windsurf",
		"copilot": "copilot",
		"gemini": "gemini",
		"amp": "amp",
		"codex": "codex",
		"cline": "cline",
		"continue-dev": "continue-dev",
	}
	
	for _, p := range providers {
		if provider, ok := p.(string); ok {
			if preset, exists := providerMap[provider]; exists {
				presets = append(presets, preset)
				if provider == "continue-dev" {
					hasContinueDev = true
				}
			}
		}
	}
	
	return presets, hasContinueDev
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

	presets, hasContinueDev := getPresetsFromProviders(providers, allProviders, popularProviders)

	var configContent string
	if len(presets) > 0 {
		configContent = templates.GenerateConfigWithPresets(projectName, presets)
	} else {
		configContent = templates.GenerateConfigWithPresets(projectName, []string{"claude"})
	}

	if err := os.WriteFile("ai_rulez.yaml", []byte(configContent), 0o644); err != nil {
		return crud.ToolError(fmt.Errorf("failed to write config file: %w", err))
	}

	if hasContinueDev {
		if err := crud.CreateContinueDevConfig(); err != nil {
			return crud.ToolError(fmt.Errorf("failed to create continue.dev config: %w", err))
		}
	}

	return crud.ToolSuccess(map[string]interface{}{
		"message": "Project initialized successfully",
		"path":    "ai_rulez.yaml",
	})
}
