package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetMetadataHandler handles the get_metadata MCP tool
func GetMetadataHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleGet(ctx, "metadata", "")
}

// SetMetadataHandler handles the set_metadata MCP tool
func SetMetadataHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	updates := make(map[string]interface{})
	if name, ok := request.Params.GetString("name"); ok {
		updates["Name"] = name
	}
	if version, ok := request.Params.GetString("version"); ok {
		updates["Version"] = version
	}
	if description, ok := request.Params.GetString("description"); ok {
		updates["Description"] = description
	}
	return crud.HandleUpdate(ctx, "metadata", "", updates)
}

// GetExtendsHandler handles the get_extends MCP tool
func GetExtendsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleGet(ctx, "extends", "")
}

// SetExtendsHandler handles the set_extends MCP tool
func SetExtendsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL, _ := request.Params.GetString("path_or_url")
	updates := map[string]interface{}{"Extends": pathOrURL}
	return crud.HandleUpdate(ctx, "extends", "", updates)
}

// DeleteExtendsHandler handles the delete_extends MCP tool
func DeleteExtendsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleDelete(ctx, "extends", "")
}

// ListIncludesHandler handles the list_includes MCP tool
func ListIncludesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleList(ctx, "includes")
}

// AddIncludeHandler handles the add_include MCP tool
func AddIncludeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL, _ := request.Params.GetString("path_or_url")
	return crud.HandleAddToList(ctx, "includes", pathOrURL)
}

// DeleteIncludeHandler handles the delete_include MCP tool
func DeleteIncludeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL, _ := request.Params.GetString("path_or_url")
	return crud.HandleDeleteFromList(ctx, "includes", pathOrURL)
}
