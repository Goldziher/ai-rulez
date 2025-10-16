package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func GetMetadataHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleGetMCP(ctx, "metadata", "")
}

func SetMetadataHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
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

func GetExtendsHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleGetMCP(ctx, "extends", "")
}

func SetExtendsHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL := request.GetString("path_or_url", "")
	updates := map[string]interface{}{"Extends": pathOrURL}
	return crud.HandleUpdateMCP(ctx, "extends", "", updates)
}

func DeleteExtendsHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleDeleteMCP(ctx, "extends", "")
}

func ListIncludesHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	return crud.HandleListMCP(ctx, "includes")
}

func AddIncludeHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL := request.GetString("path_or_url", "")
	return crud.HandleAddToListMCP(ctx, "includes", pathOrURL)
}

func DeleteIncludeHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	pathOrURL := request.GetString("path_or_url", "")
	return crud.HandleDeleteFromListMCP(ctx, "includes", pathOrURL)
}
