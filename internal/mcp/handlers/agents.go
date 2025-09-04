package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

func ListAgentsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleListMCP(ctx, "agents")
}

func GetAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleGetMCP(ctx, "agents", name)
}

func AddAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	description := request.GetString("description", "")
	id := request.GetString("id", "")
	priorityStr := request.GetString("priority", "")
	tools := request.GetStringSlice("tools", []string{})
	systemPrompt := request.GetString("system_prompt", "")
	templateType := request.GetString("template_type", "")
	templateValue := request.GetString("template_value", "")
	targets := request.GetStringSlice("targets", []string{})

	priority, err := config.ParsePriority(priorityStr)
	if err != nil {
		return crud.ToolError(err)
	}

	newAgent := &config.Agent{
		ID:           id,
		Name:         name,
		Description:  description,
		Priority:     priority,
		Tools:        tools,
		SystemPrompt: systemPrompt,
		Template:     crud.CreateTemplateConfig(templateType, templateValue),
		Targets:      targets,
	}

	return crud.HandleAddMCP(ctx, "agents", newAgent)
}

func UpdateAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")

	updates := make(map[string]interface{})
	if newName := request.GetString("new_name", ""); newName != "" {
		updates["Name"] = newName
	}
	if id := request.GetString("id", ""); id != "" {
		updates["ID"] = id
	}
	if description := request.GetString("description", ""); description != "" {
		updates["Description"] = description
	}
	if priorityStr := request.GetString("priority", ""); priorityStr != "" {
		priority, err := config.ParsePriority(priorityStr)
		if err != nil {
			return crud.ToolError(err)
		}
		updates["Priority"] = priority
	}
	if tools := request.GetStringSlice("tools", nil); tools != nil {
		updates["Tools"] = tools
	}
	if systemPrompt := request.GetString("system_prompt", ""); systemPrompt != "" {
		updates["SystemPrompt"] = systemPrompt
	}
	if templateType := request.GetString("template_type", ""); templateType != "" {
		if templateValue := request.GetString("template_value", ""); templateValue != "" {
			updates["Template"] = crud.CreateTemplateConfig(templateType, templateValue)
		}
	}
	if targets := request.GetStringSlice("targets", nil); targets != nil {
		updates["Targets"] = targets
	}

	return crud.HandleUpdateMCP(ctx, "agents", name, updates)
}

func DeleteAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleDeleteMCP(ctx, "agents", name)
}
