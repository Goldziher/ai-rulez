package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetMetadataHandler handles the get_metadata MCP tool
func GetMetadataHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleGetMCP(ctx, "metadata", "")
}

// SetMetadataHandler handles the set_metadata MCP tool
func SetMetadataHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	updates := make(map[string]interface{})
	if name := request.GetString("name", ""); name != "" {
		updates["Name"] = name
	}
	if version := request.GetString("version", ""); version != "" {
		updates["Version"] = version
	}
	if description := request.GetString("description", ""); description != "" {
		updates["Description"] = description
	}
	return crud.HandleUpdateMCP(ctx, "metadata", "", updates)
}

// GetExtendsHandler handles the get_extends MCP tool
func GetExtendsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleGetMCP(ctx, "extends", "")
}

// SetExtendsHandler handles the set_extends MCP tool
func SetExtendsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL := request.GetString("path_or_url", "")
	updates := map[string]interface{}{"Extends": pathOrURL}
	return crud.HandleUpdateMCP(ctx, "extends", "", updates)
}

// DeleteExtendsHandler handles the delete_extends MCP tool
func DeleteExtendsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleDeleteMCP(ctx, "extends", "")
}

// ListIncludesHandler handles the list_includes MCP tool
func ListIncludesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleListMCP(ctx, "includes")
}

// AddIncludeHandler handles the add_include MCP tool
func AddIncludeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL := request.GetString("path_or_url", "")
	return crud.HandleAddToListMCP(ctx, "includes", pathOrURL)
}

// DeleteIncludeHandler handles the delete_include MCP tool
func DeleteIncludeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL := request.GetString("path_or_url", "")
	return crud.HandleDeleteFromListMCP(ctx, "includes", pathOrURL)
}
