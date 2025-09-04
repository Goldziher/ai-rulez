package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// convertInterfaceMapToStringMap converts map[string]interface{} to map[string]string
func convertInterfaceMapToStringMap(input interface{}) map[string]string {
	if input == nil {
		return nil
	}

	if inputMap, ok := input.(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range inputMap {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result
	}

	return nil
}

// ListMCPServersHandler handles the list_mcp_servers MCP tool
func ListMCPServersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleListMCP(ctx, "mcp_servers")
}

// GetMCPServerHandler handles the get_mcp_server MCP tool
func GetMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleGetMCP(ctx, "mcp_servers", name)
}

// AddMCPServerHandler handles the add_mcp_server MCP tool
func AddMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := request.GetString("id", "")
	name := request.GetString("name", "")
	description := request.GetString("description", "")
	command := request.GetString("command", "")
	args := request.GetStringSlice("args", []string{})
	envInterface := request.GetArguments()["env"] // Get env as interface{} since it's a map
	env := convertInterfaceMapToStringMap(envInterface)
	transport := request.GetString("transport", "")
	url := request.GetString("url", "")
	enabled := request.GetBool("enabled", false)
	enabledPtr := &enabled
	targets := request.GetStringSlice("targets", []string{})

	newServer := &config.MCPServer{
		ID:          id,
		Name:        name,
		Description: description,
		Command:     command,
		Args:        args,
		Env:         env,
		Transport:   transport,
		URL:         url,
		Enabled:     enabledPtr,
		Targets:     targets,
	}

	return crud.HandleAddMCP(ctx, "mcp_servers", newServer)
}

// UpdateMCPServerHandler handles the update_mcp_server MCP tool
func UpdateMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")

	updates := make(map[string]interface{})
	if id := request.GetString("id", ""); id != "" {
		updates["ID"] = id
	}
	if newName := request.GetString("new_name", ""); newName != "" {
		updates["Name"] = newName
	}
	if description := request.GetString("description", ""); description != "" {
		updates["Description"] = description
	}
	if command := request.GetString("command", ""); command != "" {
		updates["Command"] = command
	}
	if args := request.GetStringSlice("args", nil); args != nil {
		updates["Args"] = args
	}
	if envInterface := request.GetArguments()["env"]; envInterface != nil {
		updates["Env"] = convertInterfaceMapToStringMap(envInterface)
	}
	if transport := request.GetString("transport", ""); transport != "" {
		updates["Transport"] = transport
	}
	if url := request.GetString("url", ""); url != "" {
		updates["URL"] = url
	}
	if request.GetArguments()["enabled"] != nil {
		enabled := request.GetBool("enabled", false)
		enabledPtr := &enabled
		updates["Enabled"] = enabledPtr
	}
	if targets := request.GetStringSlice("targets", nil); targets != nil {
		updates["Targets"] = targets
	}

	return crud.HandleUpdateMCP(ctx, "mcp_servers", name, updates)
}

// DeleteMCPServerHandler handles the delete_mcp_server MCP tool
func DeleteMCPServerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	return crud.HandleDeleteMCP(ctx, "mcp_servers", name)
}
