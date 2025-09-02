package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetAgentsHandler handles the get_agents MCP tool
func GetAgentsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		"agents": cfg.Agents,
		"count":  len(cfg.Agents),
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

// AddAgentHandler handles the add_agent MCP tool
func AddAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := struct {
		ConfigFile   string   `json:"config_file"`
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		SystemPrompt string   `json:"system_prompt"`
		Tools        []string `json:"tools"`
		Priority     float64  `json:"priority"`
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

	for i := range cfg.Agents {
		if cfg.Agents[i].Name == params.Name {
			return nil, fmt.Errorf("agent with name '%s' already exists", params.Name)
		}
	}

	priority := 5
	if params.Priority > 0 {
		priority = int(params.Priority)
	}

	newAgent := config.Agent{
		Name:         params.Name,
		Description:  params.Description,
		SystemPrompt: params.SystemPrompt,
		Tools:        params.Tools,
		Priority:     priority,
	}

	cfg.Agents = append(cfg.Agents, newAgent)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Agent '%s' added successfully", params.Name),
		"agent":   newAgent,
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

// UpdateAgentHandler handles the update_agent MCP tool
func UpdateAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := parseUpdateAgentParams(request)
	if err != nil {
		return nil, err
	}

	cfg, configPath, err := loadConfigWithPath(ctx, params.ConfigFile)
	if err != nil {
		return nil, err
	}

	agentIndex := findAgentIndex(cfg, params.Name)
	if agentIndex == -1 {
		return nil, fmt.Errorf("agent with name '%s' not found", params.Name)
	}

	if err := validateAgentNameUpdate(cfg, params, agentIndex); err != nil {
		return nil, err
	}

	updateAgentFields(cfg, params, agentIndex)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	return createUpdateAgentResponse(params.Name, &cfg.Agents[agentIndex])
}

func parseUpdateAgentParams(request mcp.CallToolRequest) (*updateAgentParams, error) {
	params := &updateAgentParams{}
	if request.Params.Arguments != nil {
		if data, err := json.Marshal(request.Params.Arguments); err == nil {
			//nolint:errcheck // Ignore JSON unmarshal errors, use defaults
			_ = json.Unmarshal(data, params)
		}
	}
	return params, nil
}

func findAgentIndex(cfg *config.Config, name string) int {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			return i
		}
	}
	return -1
}

func validateAgentNameUpdate(cfg *config.Config, params *updateAgentParams, agentIndex int) error {
	if params.NewName != "" && params.NewName != params.Name {
		for i := range cfg.Agents {
			if cfg.Agents[i].Name == params.NewName {
				return fmt.Errorf("agent with name '%s' already exists", params.NewName)
			}
		}
	}
	return nil
}

func updateAgentFields(cfg *config.Config, params *updateAgentParams, agentIndex int) {
	agent := &cfg.Agents[agentIndex]

	if params.NewName != "" && params.NewName != params.Name {
		agent.Name = params.NewName
	}
	if params.Description != "" {
		agent.Description = params.Description
	}
	if params.SystemPrompt != "" {
		agent.SystemPrompt = params.SystemPrompt
	}
	if params.Tools != nil {
		agent.Tools = params.Tools
	}
	if params.Priority > 0 {
		agent.Priority = int(params.Priority)
	}
}

func createUpdateAgentResponse(originalName string, agent *config.Agent) (*mcp.CallToolResult, error) {
	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Agent '%s' updated successfully", originalName),
		"agent":   agent,
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

type updateAgentParams struct {
	ConfigFile   string   `json:"config_file"`
	Name         string   `json:"name"`
	NewName      string   `json:"new_name"`
	Description  string   `json:"description"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
	Priority     float64  `json:"priority"`
}

// DeleteAgentHandler handles the delete_agent MCP tool
func DeleteAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	agentIndex := -1
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == params.Name {
			agentIndex = i
			break
		}
	}

	if agentIndex == -1 {
		return nil, fmt.Errorf("agent with name '%s' not found", params.Name)
	}

	cfg.Agents = append(cfg.Agents[:agentIndex], cfg.Agents[agentIndex+1:]...)

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Agent '%s' deleted successfully", params.Name),
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
