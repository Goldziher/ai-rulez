package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListCommandsHandler handles the list_commands MCP tool
func ListCommandsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleList(ctx, "commands")
}

// GetCommandHandler handles the get_command MCP tool
func GetCommandHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleGet(ctx, "commands", name)
}

// AddCommandHandler handles the add_command MCP tool
func AddCommandHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, _ := request.Params.GetString("id")
	name, _ := request.Params.GetString("name")
	description, _ := request.Params.GetString("description")
	aliases, _ := request.Params.GetArray("aliases")
	usage, _ := request.Params.GetString("usage")
	systemPrompt, _ := request.Params.GetString("system_prompt")
	shortcut, _ := request.Params.GetString("shortcut")
	enabled, _ := request.Params.GetBoolean("enabled")
	targets, _ := request.Params.GetArray("targets")

	newCmd := &config.Command{
		ID:           id,
		Name:         name,
		Description:  description,
		Aliases:      crud.InterfaceSliceToStringSlice(aliases),
		Usage:        usage,
		SystemPrompt: systemPrompt,
		Shortcut:     shortcut,
		Enabled:      enabled,
		Targets:      crud.InterfaceSliceToStringSlice(targets),
	}

	return crud.HandleAdd(ctx, "commands", newCmd)
}

// UpdateCommandHandler handles the update_command MCP tool
func UpdateCommandHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")

	updates := make(map[string]interface{})
	if id, ok := request.Params.GetString("id"); ok {
		updates["ID"] = id
	}
	if newName, ok := request.Params.GetString("new_name"); ok {
		updates["Name"] = newName
	}
	if description, ok := request.Params.GetString("description"); ok {
		updates["Description"] = description
	}
	if aliases, ok := request.Params.GetArray("aliases"); ok {
		updates["Aliases"] = crud.InterfaceSliceToStringSlice(aliases)
	}
	if usage, ok := request.Params.GetString("usage"); ok {
		updates["Usage"] = usage
	}
	if systemPrompt, ok := request.Params.GetString("system_prompt"); ok {
		updates["SystemPrompt"] = systemPrompt
	}
	if shortcut, ok := request.Params.GetString("shortcut"); ok {
		updates["Shortcut"] = shortcut
	}
	if enabled, ok := request.Params.GetBoolean("enabled"); ok {
		updates["Enabled"] = enabled
	}
	if targets, ok := request.Params.GetArray("targets"); ok {
		updates["Targets"] = crud.InterfaceSliceToStringSlice(targets)
	}

	return crud.HandleUpdate(ctx, "commands", name, updates)
}

// DeleteCommandHandler handles the delete_command MCP tool
func DeleteCommandHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleDelete(ctx, "commands", name)
}
