package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListRulesHandler handles the list_rules MCP tool
func ListRulesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	nameFilter := request.GetString("name_filter", "")
	return crud.HandleListMCPWithFilter(ctx, "rules", nameFilter)
}

// GetRuleHandler handles the get_rule MCP tool
func GetRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleGetMCP(ctx, "rules", name)
}

// AddRuleHandler handles the add_rule MCP tool
func AddRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	content := request.GetString("content", "")
	id := request.GetString("id", "")
	priorityStr := request.GetString("priority", "")
	targets := request.GetStringSlice("targets", []string{})

	priority, err := config.ParsePriority(priorityStr)
	if err != nil {
		return crud.ToolError(err)
	}

	newRule := &config.Rule{
		ID:       id,
		Name:     name,
		Content:  content,
		Priority: priority,
		Targets:  targets,
	}

	return crud.HandleAddMCP(ctx, "rules", newRule)
}

// UpdateRuleHandler handles the update_rule MCP tool
func UpdateRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")

	updates := make(map[string]interface{})
	if newName := request.GetString("new_name", ""); newName != "" {
		updates["Name"] = newName
	}
	if id := request.GetString("id", ""); id != "" {
		updates["ID"] = id
	}
	if content := request.GetString("content", ""); content != "" {
		updates["Content"] = content
	}
	if priorityStr := request.GetString("priority", ""); priorityStr != "" {
		priority, err := config.ParsePriority(priorityStr)
		if err != nil {
			return crud.ToolError(err)
		}
		updates["Priority"] = priority
	}
	if targets := request.GetStringSlice("targets", nil); targets != nil {
		updates["Targets"] = targets
	}

	return crud.HandleUpdateMCP(ctx, "rules", name, updates)
}

// DeleteRuleHandler handles the delete_rule MCP tool
func DeleteRuleHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleDeleteMCP(ctx, "rules", name)
}
