package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListAgentsHandler handles the list_agents MCP tool
func ListAgentsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleList(ctx, "agents")
}

// GetAgentHandler handles the get_agent MCP tool
func GetAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleGet(ctx, "agents", name)
}

// AddAgentHandler handles the add_agent MCP tool
func AddAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	description, _ := request.Params.GetString("description")
	id, _ := request.Params.GetString("id")
	priorityStr, _ := request.Params.GetString("priority")
	tools, _ := request.Params.GetArray("tools")
	systemPrompt, _ := request.Params.GetString("system_prompt")
	templateType, _ := request.Params.GetString("template_type")
	templateValue, _ := request.Params.GetString("template_value")
	targets, _ := request.Params.GetArray("targets")

	priority, err := config.ParsePriority(priorityStr)
	if err != nil {
		return crud.ToolError(err)
	}

	newAgent := &config.Agent{
		ID:           id,
		Name:         name,
		Description:  description,
		Priority:     priority,
		Tools:        crud.InterfaceSliceToStringSlice(tools),
		SystemPrompt: systemPrompt,
		Template:     crud.CreateTemplateConfig(templateType, templateValue),
		Targets:      crud.InterfaceSliceToStringSlice(targets),
	}

	return crud.HandleAdd(ctx, "agents", newAgent)
}

// UpdateAgentHandler handles the update_agent MCP tool
func UpdateAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")

	updates := make(map[string]interface{})
	if newName, ok := request.Params.GetString("new_name"); ok {
		updates["Name"] = newName
	}
	if id, ok := request.Params.GetString("id"); ok {
		updates["ID"] = id
	}
	if description, ok := request.Params.GetString("description"); ok {
		updates["Description"] = description
	}
	if priorityStr, ok := request.Params.GetString("priority"); ok {
		priority, err := config.ParsePriority(priorityStr)
		if err != nil {
			return crud.ToolError(err)
		}
		updates["Priority"] = priority
	}
	if tools, ok := request.Params.GetArray("tools"); ok {
		updates["Tools"] = crud.InterfaceSliceToStringSlice(tools)
	}
	if systemPrompt, ok := request.Params.GetString("system_prompt"); ok {
		updates["SystemPrompt"] = systemPrompt
	}
	if templateType, typeOK := request.Params.GetString("template_type"); typeOK {
		if templateValue, valueOK := request.Params.GetString("template_value"); valueOK {
			updates["Template"] = crud.CreateTemplateConfig(templateType, templateValue)
		}
	}
	if targets, ok := request.Params.GetArray("targets"); ok {
		updates["Targets"] = crud.InterfaceSliceToStringSlice(targets)
	}

	return crud.HandleUpdate(ctx, "agents", name, updates)
}

// DeleteAgentHandler handles the delete_agent MCP tool
func DeleteAgentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleDelete(ctx, "agents", name)
}
