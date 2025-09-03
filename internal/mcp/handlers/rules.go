package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListRulesHandler handles the list_rules MCP tool
func ListRulesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleList(ctx, "rules")
}

// GetRuleHandler handles the get_rule MCP tool
func GetRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleGet(ctx, "rules", name)
}

// AddRuleHandler handles the add_rule MCP tool
func AddRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	content, _ := request.Params.GetString("content")
	id, _ := request.Params.GetString("id")
	priorityStr, _ := request.Params.GetString("priority")
	targets, _ := request.Params.GetArray("targets")

	priority, err := config.ParsePriority(priorityStr)
	if err != nil {
		return crud.ToolError(err)
	}

	newRule := &config.Rule{
		ID:       id,
		Name:     name,
		Content:  content,
		Priority: priority,
		Targets:  crud.InterfaceSliceToStringSlice(targets),
	}

	return crud.HandleAdd(ctx, "rules", newRule)
}

// UpdateRuleHandler handles the update_rule MCP tool
func UpdateRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")

	updates := make(map[string]interface{})
	if newName, ok := request.Params.GetString("new_name"); ok {
		updates["Name"] = newName
	}
	if id, ok := request.Params.GetString("id"); ok {
		updates["ID"] = id
	}
	if content, ok := request.Params.GetString("content"); ok {
		updates["Content"] = content
	}
	if priorityStr, ok := request.Params.GetString("priority"); ok {
		priority, err := config.ParsePriority(priorityStr)
		if err != nil {
			return crud.ToolError(err)
		}
		updates["Priority"] = priority
	}
	if targets, ok := request.Params.GetArray("targets"); ok {
		updates["Targets"] = crud.InterfaceSliceToStringSlice(targets)
	}

	return crud.HandleUpdate(ctx, "rules", name, updates)
}

// DeleteRuleHandler handles the delete_rule MCP tool
func DeleteRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleDelete(ctx, "rules", name)
}
