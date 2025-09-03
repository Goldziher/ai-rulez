package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListSectionsHandler handles the list_sections MCP tool
func ListSectionsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleList(ctx, "sections")
}

// GetSectionHandler handles the get_section MCP tool
func GetSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleGet(ctx, "sections", name)
}

// AddSectionHandler handles the add_section MCP tool
func AddSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	content, _ := request.Params.GetString("content")
	id, _ := request.Params.GetString("id")
	priorityStr, _ := request.Params.GetString("priority")
	targets, _ := request.Params.GetArray("targets")

	priority, err := config.ParsePriority(priorityStr)
	if err != nil {
		return crud.ToolError(err)
	}

	newSection := &config.Section{
		ID:       id,
		Name:     name,
		Content:  content,
		Priority: priority,
		Targets:  crud.InterfaceSliceToStringSlice(targets),
	}

	return crud.HandleAdd(ctx, "sections", newSection)
}

// UpdateSectionHandler handles the update_section MCP tool
func UpdateSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	return crud.HandleUpdate(ctx, "sections", name, updates)
}

// DeleteSectionHandler handles the delete_section MCP tool
func DeleteSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleDelete(ctx, "sections", name)
}
