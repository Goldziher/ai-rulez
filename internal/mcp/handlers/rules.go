package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetRulesHandler GetRules handles the get_rules MCP tool
func GetRulesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile  string  `json:"config_file"`
		MinPriority float64 `json:"min_priority"`
		NameFilter  string  `json:"name_filter"`
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

	var filteredRules []config.Rule
	for _, rule := range cfg.Rules {
		if params.MinPriority > 0 && rule.Priority < int(params.MinPriority) {
			continue
		}
		if params.NameFilter != "" && !strings.Contains(strings.ToLower(rule.Name), strings.ToLower(params.NameFilter)) {
			continue
		}
		filteredRules = append(filteredRules, rule)
	}

	result := map[string]interface{}{
		"rules": filteredRules,
		"count": len(filteredRules),
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

// AddRuleHandler AddRule handles the add_rule MCP tool
func AddRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	for _, rule := range cfg.Rules {
		if rule.Name == params.Name {
			return nil, fmt.Errorf("rule with name '%s' already exists", params.Name)
		}
	}

	priority := 5
	if params.Priority > 0 {
		priority = int(params.Priority)
	}

	newRule := config.Rule{
		Name:     params.Name,
		Content:  params.Content,
		Priority: priority,
	}

	cfg.Rules = append(cfg.Rules, newRule)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Rule '%s' added successfully", params.Name),
		"rule":    newRule,
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

// UpdateRuleHandler UpdateRule handles the update_rule MCP tool
func UpdateRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	ruleIndex := -1
	for i, rule := range cfg.Rules {
		if rule.Name == params.Name {
			ruleIndex = i
			break
		}
	}

	if ruleIndex == -1 {
		return nil, fmt.Errorf("rule with name '%s' not found", params.Name)
	}

	if params.NewName != "" && params.NewName != params.Name {
		for _, rule := range cfg.Rules {
			if rule.Name == params.NewName {
				return nil, fmt.Errorf("rule with name '%s' already exists", params.NewName)
			}
		}
		cfg.Rules[ruleIndex].Name = params.NewName
	}

	if params.Content != "" {
		cfg.Rules[ruleIndex].Content = params.Content
	}

	if params.Priority > 0 {
		cfg.Rules[ruleIndex].Priority = int(params.Priority)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Rule '%s' updated successfully", params.Name),
		"rule":    cfg.Rules[ruleIndex],
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

// DeleteRuleHandler DeleteRule handles the delete_rule MCP tool
func DeleteRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	ruleIndex := -1
	for i, rule := range cfg.Rules {
		if rule.Name == params.Name {
			ruleIndex = i
			break
		}
	}

	if ruleIndex == -1 {
		return nil, fmt.Errorf("rule with name '%s' not found", params.Name)
	}

	cfg.Rules = append(cfg.Rules[:ruleIndex], cfg.Rules[ruleIndex+1:]...)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Rule '%s' deleted successfully", params.Name),
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
