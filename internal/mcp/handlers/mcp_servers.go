package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

var ListMCPServersHandler = CreateListHandler("mcp_servers", false)
var GetMCPServerHandler = CreateGetHandler("mcp_servers")
var DeleteMCPServerHandler = CreateDeleteHandler("mcp_servers")

func AddMCPServerHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	id := request.GetString("id", "")
	name := request.GetString("name", "")
	description := request.GetString("description", "")
	command := request.GetString("command", "")
	args := request.GetStringSlice("args", []string{})
	envInterface := request.GetArguments()["env"]
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

func UpdateMCPServerHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
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
