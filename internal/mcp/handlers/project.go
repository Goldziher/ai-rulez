package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/samber/oops"
)

// GenerateOutputHandler handles the generate_output MCP tool
func GenerateOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := parseGenerateParams(request)
	if err != nil {
		return nil, err
	}

	configFiles, err := findConfigFiles(params)
	if err != nil {
		return nil, err
	}

	results := processConfigFiles(ctx, configFiles, params)
	return createGenerateResponse(results)
}

func parseGenerateParams(request mcp.CallToolRequest) (*generateParams, error) {
	params := &generateParams{}
	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			_ = json.Unmarshal(data, params) //nolint:errcheck // Ignore errors, use defaults
		}
	}
	return params, nil
}

func findConfigFiles(params *generateParams) ([]string, error) {
	if params.Recursive {
		return findConfigFilesRecursively()
	}
	return findSingleConfigFile(params.ConfigFile)
}

func findConfigFilesRecursively() ([]string, error) {
	var configFiles []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isConfigFileName(filepath.Base(path)) {
			configFiles = append(configFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "MCP: find config files recursively")
	}
	return configFiles, nil
}

func findSingleConfigFile(configFile string) ([]string, error) {
	configPath := configFile
	if configPath == "" {
		var err error
		configPath, err = config.FindConfigFile(".")
		if err != nil {
			return nil, oops.Wrapf(err, "MCP: find configuration file")
		}
	}
	return []string{configPath}, nil
}

func isConfigFileName(basename string) bool {
	return basename == "ai_rules.yaml" || basename == "ai_rulez.yaml"
}

func processConfigFiles(ctx context.Context, configFiles []string, params *generateParams) []map[string]interface{} {
	var results []map[string]interface{}
	for _, configPath := range configFiles {
		result := processConfigFile(ctx, configPath, params)
		results = append(results, result)
	}
	return results
}

func processConfigFile(ctx context.Context, configPath string, params *generateParams) map[string]interface{} {
	cfg, err := config.LoadConfigWithIncludes(ctx, configPath)
	if err != nil {
		return createErrorResult(configPath, err)
	}

	gen := generator.NewWithConfigFile(configPath)

	if params.DryRun {
		return handleDryRunGeneration(configPath, gen, cfg)
	}
	return handleRealGeneration(configPath, gen, cfg, params)
}

func createErrorResult(configPath string, err error) map[string]interface{} {
	return map[string]interface{}{
		"file":    configPath,
		"success": false,
		"error":   err.Error(),
	}
}

func handleDryRunGeneration(configPath string, gen *generator.Generator, cfg *config.Config) map[string]interface{} {
	preview, err := gen.PreviewAll(cfg)
	if err != nil {
		return createErrorResult(configPath, err)
	}
	return map[string]interface{}{
		"file":    configPath,
		"success": true,
		"dry_run": true,
		"outputs": preview,
	}
}

func handleRealGeneration(configPath string, gen *generator.Generator, cfg *config.Config, params *generateParams) map[string]interface{} {
	err := gen.GenerateAll(cfg)
	if err != nil {
		return createErrorResult(configPath, err)
	}
	return map[string]interface{}{
		"file":    configPath,
		"success": true,
		"written": len(cfg.Outputs),
	}
}

func createGenerateResponse(results []map[string]interface{}) (*mcp.CallToolResult, error) {
	result := map[string]interface{}{
		"results": results,
		"count":   len(results),
	}

	content, _ := json.MarshalIndent(result, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(content),
			},
		},
	}, nil
}

type generateParams struct {
	ConfigFile      string `json:"config_file"`
	DryRun          bool   `json:"dry_run"`
	UpdateGitignore bool   `json:"update_gitignore"`
	Recursive       bool   `json:"recursive"`
}

// ValidateConfigHandler Validate handles the validate_config MCP tool
func ValidateConfigHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile string `json:"config_file"`
	}{}

	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			//nolint:errcheck // Ignore JSON unmarshal errors, use defaults
			_ = json.Unmarshal(data, &params)
		}
	}

	configPath := params.ConfigFile
	if configPath == "" {
		var err error
		configPath, err = config.FindConfigFile(".")
		if err != nil {
			return nil, oops.Wrapf(err, "MCP: find configuration file")
		}
	}

	cfg, err := config.LoadConfigWithIncludes(ctx, configPath)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"valid": false, "error": %q}`, err.Error()),
				},
			},
		}, nil
	}

	result := map[string]interface{}{
		"valid":    true,
		"file":     configPath,
		"rules":    len(cfg.Rules),
		"sections": len(cfg.Sections),
		"outputs":  len(cfg.Outputs),
		"agents":   len(cfg.Agents),
	}

	content, _ := json.MarshalIndent(result, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(content),
			},
		},
	}, nil
}

// InitProjectHandler InitProject handles the init_project MCP tool
func InitProjectHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ProjectName string `json:"project_name"`
		Preset      string `json:"preset"`
		Claude      bool   `json:"claude"`
		Cursor      bool   `json:"cursor"`
		Windsurf    bool   `json:"windsurf"`
		Copilot     bool   `json:"copilot"`
		Gemini      bool   `json:"gemini"`
		Amp         bool   `json:"amp"`
		Codex       bool   `json:"codex"`
		Cline       bool   `json:"cline"`
		ContinueDev bool   `json:"continue_dev"`
	}{}

	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			//nolint:errcheck // Ignore JSON unmarshal errors, use defaults
			_ = json.Unmarshal(data, &params)
		}
	}

	if _, err := config.FindConfigFile("."); err == nil {
		return nil, fmt.Errorf("configuration file already exists in the current directory")
	}

	providers := templates.ProviderConfig{
		Claude:      params.Claude || params.Preset == "claude",
		Cursor:      params.Cursor || params.Preset == "cursor",
		Windsurf:    params.Windsurf || params.Preset == "windsurf",
		Copilot:     params.Copilot || params.Preset == "copilot",
		Gemini:      params.Gemini || params.Preset == "gemini",
		Amp:         params.Amp || params.Preset == "amp",
		Codex:       params.Codex || params.Preset == "codex",
		Cline:       params.Cline || params.Preset == "cline",
		ContinueDev: params.ContinueDev || params.Preset == "continue-dev",
	}

	if !providers.HasAny() {
		providers.Claude = true
	}

	configContent := templates.GenerateConfigTemplate(params.ProjectName, providers)

	configPath := "ai_rulez.yaml"
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return nil, oops.Wrapf(err, "MCP: write configuration file")
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Project '%s' initialized successfully", params.ProjectName),
		"file":    configPath,
		"providers": map[string]bool{
			"claude":       providers.Claude,
			"cursor":       providers.Cursor,
			"windsurf":     providers.Windsurf,
			"copilot":      providers.Copilot,
			"gemini":       providers.Gemini,
			"amp":          providers.Amp,
			"codex":        providers.Codex,
			"cline":        providers.Cline,
			"continue_dev": providers.ContinueDev,
		},
	}

	content, _ := json.MarshalIndent(result, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(content),
			},
		},
	}, nil
}

// GetVersion returns a handler function for the get_version MCP tool

// HandlerFunc is the type for MCP handler functions
type HandlerFunc func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error)
