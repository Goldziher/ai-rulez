package handlers

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetVersionHandler returns a handler that returns the version
func GetVersionHandler(version string) func(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
		result := map[string]string{
			"version": version,
		}

		content, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(content)},
			},
		}, nil
	}
}
