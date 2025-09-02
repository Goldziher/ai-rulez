package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetSectionsHandler GetSections handles the get_sections MCP tool
func GetSectionsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		"sections": cfg.Sections,
		"count":    len(cfg.Sections),
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

// AddSectionHandler AddSection handles the add_section MCP tool
func AddSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile string  `json:"config_file"`
		Name       string  `json:"name"`
		Content    string  `json:"content"`
		Priority   float64 `json:"priority"`
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

	for _, section := range cfg.Sections {
		if section.Name == params.Name {
			return nil, fmt.Errorf("section with title '%s' already exists", params.Name)
		}
	}

	priority := 5
	if params.Priority > 0 {
		priority = int(params.Priority)
	}

	newSection := config.Section{
		Name:     params.Name,
		Content:  params.Content,
		Priority: priority,
	}

	cfg.Sections = append(cfg.Sections, newSection)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Section '%s' added successfully", params.Name),
		"section": newSection,
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

// UpdateSectionHandler UpdateSection handles the update_section MCP tool
func UpdateSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile string  `json:"config_file"`
		Name       string  `json:"name"`
		NewName    string  `json:"new_name"`
		Content    string  `json:"content"`
		Priority   float64 `json:"priority"`
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

	sectionIndex := -1
	for i, section := range cfg.Sections {
		if section.Name == params.Name {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		return nil, fmt.Errorf("section with title '%s' not found", params.Name)
	}

	if params.NewName != "" && params.NewName != params.Name {
		for _, section := range cfg.Sections {
			if section.Name == params.NewName {
				return nil, fmt.Errorf("section with title '%s' already exists", params.NewName)
			}
		}
		cfg.Sections[sectionIndex].Name = params.NewName
	}

	if params.Content != "" {
		cfg.Sections[sectionIndex].Content = params.Content
	}

	if params.Priority > 0 {
		cfg.Sections[sectionIndex].Priority = int(params.Priority)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Section '%s' updated successfully", params.Name),
		"section": cfg.Sections[sectionIndex],
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

// DeleteSectionHandler DeleteSection handles the delete_section MCP tool
func DeleteSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile string `json:"config_file"`
		Name       string `json:"name"`
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

	sectionIndex := -1
	for i, section := range cfg.Sections {
		if section.Name == params.Name {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		return nil, fmt.Errorf("section with title '%s' not found", params.Name)
	}

	cfg.Sections = append(cfg.Sections[:sectionIndex], cfg.Sections[sectionIndex+1:]...)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Section '%s' deleted successfully", params.Name),
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
