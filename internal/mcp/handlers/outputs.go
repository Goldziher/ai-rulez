package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetOutputsHandler handles the get_outputs MCP tool
func GetOutputsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile string `json:"config_file"`
	}{}

	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			//nolint:errcheck // Ignore JSON unmarshal errors, use defaults
			_ = json.Unmarshal(data, &params)
		}
	}

	cfg, err := loadConfig(ctx, params.ConfigFile)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"outputs": cfg.Outputs,
		"count":   len(cfg.Outputs),
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

// AddOutputHandler handles the add_output MCP tool
func AddOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile   string `json:"config_file"`
		Path         string `json:"path"`
		Type         string `json:"type"`
		NamingScheme string `json:"naming_scheme"`
	}{}

	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			//nolint:errcheck // Ignore JSON unmarshal errors, use defaults
			_ = json.Unmarshal(data, &params)
		}
	}

	if err := validateOutputType(params.Type); err != nil {
		return nil, err
	}

	cfg, configPath, err := loadConfigWithPath(ctx, params.ConfigFile)
	if err != nil {
		return nil, err
	}

	for _, output := range cfg.Outputs {
		if output.Path == params.Path {
			return nil, fmt.Errorf("output with path '%s' already exists", params.Path)
		}
	}

	newOutput := config.Output{
		Path:         params.Path,
		Type:         params.Type,
		NamingScheme: params.NamingScheme,
	}

	cfg.Outputs = append(cfg.Outputs, newOutput)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Output '%s' added successfully", params.Path),
		"output":  newOutput,
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

// UpdateOutputHandler UpdateOutput handles the update_output MCP tool
func UpdateOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile   string `json:"config_file"`
		Path         string `json:"path"`
		NewPath      string `json:"new_path"`
		Type         string `json:"type"`
		NamingScheme string `json:"naming_scheme"`
	}{}

	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			//nolint:errcheck // Ignore JSON unmarshal errors, use defaults
			_ = json.Unmarshal(data, &params)
		}
	}

	if params.Type != "" {
		if err := validateOutputType(params.Type); err != nil {
			return nil, err
		}
	}

	cfg, configPath, err := loadConfigWithPath(ctx, params.ConfigFile)
	if err != nil {
		return nil, err
	}

	outputIndex := findOutputIndex(cfg.Outputs, params.Path)
	if outputIndex == -1 {
		return nil, fmt.Errorf("output with path '%s' not found", params.Path)
	}

	if params.NewPath != "" && params.NewPath != params.Path {
		for _, output := range cfg.Outputs {
			if output.Path == params.NewPath {
				return nil, fmt.Errorf("output with path '%s' already exists", params.NewPath)
			}
		}
		cfg.Outputs[outputIndex].Path = params.NewPath
	}

	if params.Type != "" {
		cfg.Outputs[outputIndex].Type = params.Type
	}

	if params.NamingScheme != "" {
		cfg.Outputs[outputIndex].NamingScheme = params.NamingScheme
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Output '%s' updated successfully", params.Path),
		"output":  cfg.Outputs[outputIndex],
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

// DeleteOutputHandler DeleteOutput handles the delete_output MCP tool
func DeleteOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile string `json:"config_file"`
		Path       string `json:"path"`
	}{}

	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			//nolint:errcheck // Ignore JSON unmarshal errors, use defaults
			_ = json.Unmarshal(data, &params)
		}
	}

	cfg, configPath, err := loadConfigWithPath(ctx, params.ConfigFile)
	if err != nil {
		return nil, err
	}

	outputIndex := findOutputIndex(cfg.Outputs, params.Path)
	if outputIndex == -1 {
		return nil, fmt.Errorf("output with path '%s' not found", params.Path)
	}

	cfg.Outputs = append(cfg.Outputs[:outputIndex], cfg.Outputs[outputIndex+1:]...)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Output '%s' deleted successfully", params.Path),
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
