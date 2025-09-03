package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListOutputsHandler handles the list_outputs MCP tool
func ListOutputsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleList(ctx, "outputs")
}

// GetOutputHandler handles the get_output MCP tool
func GetOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.Params.GetString("path")
	return crud.HandleGet(ctx, "outputs", path)
}

// AddOutputHandler handles the add_output MCP tool
func AddOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.Params.GetString("path")
	outputType, _ := request.Params.GetString("type")
	namingScheme, _ := request.Params.GetString("naming_scheme")
	templateType, _ := request.Params.GetString("template_type")
	templateValue, _ := request.Params.GetString("template_value")

	newOutput := &config.Output{
		Path:         path,
		Type:         outputType,
		NamingScheme: namingScheme,
		Template:     crud.CreateTemplateConfig(templateType, templateValue),
	}

	return crud.HandleAdd(ctx, "outputs", newOutput)
}

// UpdateOutputHandler handles the update_output MCP tool
func UpdateOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.Params.GetString("path")

	updates := make(map[string]interface{})
	if newPath, ok := request.Params.GetString("new_path"); ok {
		updates["Path"] = newPath
	}
	if outputType, ok := request.Params.GetString("type"); ok {
		updates["Type"] = outputType
	}
	if namingScheme, ok := request.Params.GetString("naming_scheme"); ok {
		updates["NamingScheme"] = namingScheme
	}
	if templateType, typeOK := request.Params.GetString("template_type"); typeOK {
		if templateValue, valueOK := request.Params.GetString("template_value"); valueOK {
			updates["Template"] = crud.CreateTemplateConfig(templateType, templateValue)
		}
	}

	return crud.HandleUpdate(ctx, "outputs", path, updates)
}

// DeleteOutputHandler handles the delete_output MCP tool
func DeleteOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.Params.GetString("path")
	return crud.HandleDelete(ctx, "outputs", path)
}
