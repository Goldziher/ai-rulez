package handlers

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

// GetVersionHandler returns a handler that returns the version
func GetVersionHandler(version string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := map[string]string{
			"version": version,
		}

		content, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(content),
				},
			},
		}, nil
	}
}
