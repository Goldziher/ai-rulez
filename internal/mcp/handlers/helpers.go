package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// loadConfig loads a configuration file from the specified path or auto-discovers it
func loadConfig(ctx context.Context, configFile string) (*config.Config, error) {
	cfg, _, err := loadConfigWithPath(ctx, configFile)
	return cfg, err
}

// loadConfigWithPath loads a configuration file and returns both the config and its path
func loadConfigWithPath(ctx context.Context, configFile string) (*config.Config, string, error) {
	var configPath string
	var err error

	if configFile != "" {
		configPath = configFile
	} else {
		configPath, err = config.FindConfigFile(".")
		if err != nil {
			return nil, "", oops.Wrapf(err, "MCP: find configuration file")
		}
	}

	cfg, err := config.LoadConfigWithIncludes(ctx, configPath)
	if err != nil {
		return nil, "", oops.Wrapf(err, "MCP: load configuration")
	}

	return cfg, configPath, nil
}

// saveConfig saves the configuration to the specified file
func saveConfig(cfg *config.Config, configPath string) error {
	// Marshal the config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return oops.Wrapf(err, "MCP: marshal configuration")
	}

	// Ensure the directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.Wrapf(err, "MCP: create config directory")
	}

	// Write the file
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return oops.Wrapf(err, "MCP: write configuration file")
	}

	return nil
}

// findOutputIndex finds the index of an output by path
func findOutputIndex(outputs []config.Output, path string) int {
	for i, output := range outputs {
		if output.Path == path {
			return i
		}
	}
	return -1
}

// validateOutputType validates that the output type is valid
func validateOutputType(outputType string) error {
	validTypes := map[string]bool{
		"rule":    true,
		"section": true,
		"both":    true,
		"":        true, // empty is valid, defaults to both
	}

	if !validTypes[outputType] {
		return fmt.Errorf("invalid output type '%s': must be 'rule', 'section', or 'both'", outputType)
	}

	return nil
}

// GetVersionHandler returns a handler that returns the version
func GetVersionHandler(version string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := map[string]string{
			"version": version,
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
}
