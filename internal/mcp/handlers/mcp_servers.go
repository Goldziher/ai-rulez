package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListMCPServersHandler handles the list_mcp_servers MCP tool
func ListMCPServersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleList(ctx, "mcp_servers")
}

// GetMCPServerHandler handles the get_mcp_server MCP tool
func GetMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleGet(ctx, "mcp_servers", name)
}

// AddMCPServerHandler handles the add_mcp_server MCP tool
func AddMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, _ := request.Params.GetString("id")
	name, _ := request.Params.GetString("name")
	description, _ := request.Params.GetString("description")
	command, _ := request.Params.GetString("command")
	args, _ := request.Params.GetArray("args")
	env, _ := request.Params.GetObject("env")
	transport, _ := request.Params.GetString("transport")
	url, _ := request.Params.GetString("url")
	enabled, _ := request.Params.GetBoolean("enabled")
	targets, _ := request.Params.GetArray("targets")

	newServer := &config.MCPServer{
		ID:          id,
		Name:        name,
		Description: description,
		Command:     command,
		Args:        crud.InterfaceSliceToStringSlice(args),
		Env:         crud.InterfaceMapToStringMap(env),
		Transport:   transport,
		URL:         url,
		Enabled:     enabled,
		Targets:     crud.InterfaceSliceToStringSlice(targets),
	}

	return crud.HandleAdd(ctx, "mcp_servers", newServer)
}

// UpdateMCPServerHandler handles the update_mcp_server MCP tool
func UpdateMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if command, ok := request.Params.GetString("command"); ok {
		updates["Command"] = command
	}
	if args, ok := request.Params.GetArray("args"); ok {
		updates["Args"] = crud.InterfaceSliceToStringSlice(args)
	}
	if env, ok := request.Params.GetObject("env"); ok {
		updates["Env"] = crud.InterfaceMapToStringMap(env)
	}
	if transport, ok := request.Params.GetString("transport"); ok {
		updates["Transport"] = transport
	}
	if url, ok := request.Params.GetString("url"); ok {
		updates["URL"] = url
	}
	if enabled, ok := request.Params.GetBoolean("enabled"); ok {
		updates["Enabled"] = enabled
	}
	if targets, ok := request.Params.GetArray("targets"); ok {
		updates["Targets"] = crud.InterfaceSliceToStringSlice(targets)
	}

	return crud.HandleUpdate(ctx, "mcp_servers", name, updates)
}

// DeleteMCPServerHandler handles the delete_mcp_server MCP tool
func DeleteMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, _ := request.Params.GetString("name")
	return crud.HandleDelete(ctx, "mcp_servers", name)
}
