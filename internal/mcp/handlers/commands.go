package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ListCommandsHandler = CreateListHandler("commands", false)
var GetCommandHandler = CreateGetHandler("commands")
var DeleteCommandHandler = CreateDeleteHandler("commands")

func AddCommandHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	id := request.GetString("id", "")
	name := request.GetString("name", "")
	description := request.GetString("description", "")
	aliases := request.GetStringSlice("aliases", []string{})
	usage := request.GetString("usage", "")
	systemPrompt := request.GetString("system_prompt", "")
	shortcut := request.GetString("shortcut", "")
	enabled := request.GetBool("enabled", false)
	enabledPtr := &enabled
	targets := request.GetStringSlice("targets", []string{})

	newCmd := &config.Command{
		ID:           id,
		Name:         name,
		Description:  description,
		Aliases:      aliases,
		Usage:        usage,
		SystemPrompt: systemPrompt,
		Shortcut:     shortcut,
		Enabled:      enabledPtr,
		Targets:      targets,
	}

	return crud.HandleAddMCP(ctx, "commands", newCmd)
}

func UpdateCommandHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
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
	if aliases := request.GetStringSlice("aliases", nil); aliases != nil {
		updates["Aliases"] = aliases
	}
	if usage := request.GetString("usage", ""); usage != "" {
		updates["Usage"] = usage
	}
	if systemPrompt := request.GetString("system_prompt", ""); systemPrompt != "" {
		updates["SystemPrompt"] = systemPrompt
	}
	if shortcut := request.GetString("shortcut", ""); shortcut != "" {
		updates["Shortcut"] = shortcut
	}
	if request.GetArguments()["enabled"] != nil {
		enabled := request.GetBool("enabled", false)
		enabledPtr := &enabled
		updates["Enabled"] = enabledPtr
	}
	if targets := request.GetStringSlice("targets", nil); targets != nil {
		updates["Targets"] = targets
	}

	return crud.HandleUpdateMCP(ctx, "commands", name, updates)
}
