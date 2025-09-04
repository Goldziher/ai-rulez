package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListOutputsHandler handles the list_outputs MCP tool
func ListOutputsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleListMCP(ctx, "outputs")
}

// GetOutputHandler handles the get_output MCP tool
func GetOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")
	return crud.HandleGetMCP(ctx, "outputs", path)
}

// AddOutputHandler handles the add_output MCP tool
func AddOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")
	outputType := request.GetString("type", "")
	namingScheme := request.GetString("naming_scheme", "")
	templateType := request.GetString("template_type", "")
	templateValue := request.GetString("template_value", "")

	newOutput := &config.Output{
		Path:         path,
		Type:         outputType,
		NamingScheme: namingScheme,
		Template:     crud.CreateTemplateConfig(templateType, templateValue),
	}

	return crud.HandleAddMCP(ctx, "outputs", newOutput)
}

// UpdateOutputHandler handles the update_output MCP tool
func UpdateOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")

	updates := make(map[string]interface{})
	if newPath := request.GetString("new_path", ""); newPath != "" {
		updates["Path"] = newPath
	}
	if outputType := request.GetString("type", ""); outputType != "" {
		updates["Type"] = outputType
	}
	if namingScheme := request.GetString("naming_scheme", ""); namingScheme != "" {
		updates["NamingScheme"] = namingScheme
	}
	if templateType := request.GetString("template_type", ""); templateType != "" {
		if templateValue := request.GetString("template_value", ""); templateValue != "" {
			updates["Template"] = crud.CreateTemplateConfig(templateType, templateValue)
		}
	}

	return crud.HandleUpdateMCP(ctx, "outputs", path, updates)
}

// DeleteOutputHandler handles the delete_output MCP tool
func DeleteOutputHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")
	return crud.HandleDeleteMCP(ctx, "outputs", path)
}
