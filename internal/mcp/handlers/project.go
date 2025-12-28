package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/Goldziher/ai-rulez/internal/validator"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func GenerateOutputsHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		var err error
		configFile, err = config.FindConfigFile(".")
		if err != nil {
			return ToolError(err)
		}
	}

	// Detect config version and load appropriately
	dir, err := filepath.Abs(".")
	if err != nil {
		return ToolError(fmt.Errorf("failed to get current directory: %w", err))
	}
	version, err := config.DetectConfigVersion(dir)
	if err != nil {
		return ToolError(err)
	}

	if version == "v3" {
		// Load and generate V3 config
		v3cfg, err := config.LoadConfigV3(ctx, dir)
		if err != nil {
			return ToolError(err)
		}
		if err := v3cfg.ValidateV3(); err != nil {
			return ToolError(err)
		}
		gen := generator.NewGeneratorV3(v3cfg)
		if err := gen.Generate(""); err != nil {
			return ToolError(err)
		}
		return ToolSuccess(map[string]interface{}{
			"message": "Outputs generated successfully",
			"config":  configFile,
		})
	}
	// Load and generate V2 config
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return ToolError(err)
	}
	gen := generator.NewWithConfigFile(configFile)
	if err := gen.GenerateAll(cfg); err != nil {
		return ToolError(err)
	}
	results := make([]string, len(cfg.Outputs))
	for i, output := range cfg.Outputs {
		results[i] = output.Path
	}
	return ToolSuccess(map[string]interface{}{
		"message": "Outputs generated successfully",
		"config":  configFile,
		"results": results,
	})
}

func ValidateConfigHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	configFile := request.GetString("config_file", "")
	if configFile == "" {
		var err error
		configFile, err = config.FindConfigFile(".")
		if err != nil {
			return ToolError(err)
		}
	}

	// Detect config version and validate appropriately
	dir, err := filepath.Abs(".")
	if err != nil {
		return ToolError(fmt.Errorf("failed to get current directory: %w", err))
	}
	version, err := config.DetectConfigVersion(dir)
	if err != nil {
		return ToolError(err)
	}

	if version == "v3" {
		// Validate V3 config
		v3cfg, err := config.LoadConfigV3(ctx, dir)
		if err != nil {
			result := map[string]interface{}{
				"valid": false,
				"error": err.Error(),
			}
			return ToolSuccess(result)
		}
		if err := v3cfg.ValidateV3(); err != nil {
			result := map[string]interface{}{
				"valid": false,
				"error": err.Error(),
			}
			return ToolSuccess(result)
		}
		result := map[string]interface{}{
			"valid":    true,
			"warnings": []string{},
		}
		return ToolSuccess(result)
	}
	// Validate V2 config
	val, err := validator.NewValidator(configFile)
	if err != nil {
		return ToolError(err)
	}

	warnings, err := val.Validate(ctx)
	if err != nil {
		result := map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		}
		return ToolSuccess(result)
	}

	result := map[string]interface{}{
		"valid":    true,
		"warnings": warnings,
	}

	return ToolSuccess(result)
}

func getPresetsFromProviders(providers []interface{}, allProviders, popularProviders bool) ([]string, bool) {
	if allProviders {
		return []string{"claude", "cursor", "windsurf", "copilot", "gemini", "amp", "codex", "cline", "continue-dev"}, true
	}
	if popularProviders {
		return []string{"popular"}, false
	}

	var presets []string
	var hasContinueDev bool

	providerMap := map[string]string{
		"claude":       "claude",
		"cursor":       "cursor",
		"windsurf":     "windsurf",
		"copilot":      "copilot",
		"gemini":       "gemini",
		"amp":          "amp",
		"codex":        "codex",
		"cline":        "cline",
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

func InitProjectHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
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

	// V3 structure: create .ai-rulez/config.yaml
	aiRulesDir := ".ai-rulez"
	if err := os.MkdirAll(aiRulesDir, 0o755); err != nil {
		return ToolError(fmt.Errorf("failed to create .ai-rulez directory: %w", err))
	}

	configPath := fmt.Sprintf("%s/config.yaml", aiRulesDir)
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return ToolError(fmt.Errorf("failed to write config file: %w", err))
	}

	if hasContinueDev {
		if err := CreateContinueDevConfig(); err != nil {
			return ToolError(fmt.Errorf("failed to create continue.dev config: %w", err))
		}
	}

	return ToolSuccess(map[string]interface{}{
		"message": "Project initialized successfully",
		"path":    configPath,
	})
}
