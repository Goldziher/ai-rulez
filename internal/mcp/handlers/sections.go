package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListSectionsHandler handles the list_sections MCP tool
func ListSectionsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleListMCP(ctx, "sections")
}

// GetSectionHandler handles the get_section MCP tool
func GetSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleGetMCP(ctx, "sections", name)
}

// AddSectionHandler handles the add_section MCP tool
func AddSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	content := request.GetString("content", "")
	id := request.GetString("id", "")
	priorityStr := request.GetString("priority", "")
	targets := request.GetStringSlice("targets", []string{})

	priority, err := config.ParsePriority(priorityStr)
	if err != nil {
		return crud.ToolError(err)
	}

	newSection := &config.Section{
		ID:       id,
		Name:     name,
		Content:  content,
		Priority: priority,
		Targets:  targets,
	}

	return crud.HandleAddMCP(ctx, "sections", newSection)
}

// UpdateSectionHandler handles the update_section MCP tool
func UpdateSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	return crud.HandleUpdateMCP(ctx, "sections", name, updates)
}

// DeleteSectionHandler handles the delete_section MCP tool
func DeleteSectionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleDeleteMCP(ctx, "sections", name)
}
