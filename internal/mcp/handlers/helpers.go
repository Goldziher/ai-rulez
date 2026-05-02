package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/builtins"
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

// ShowBuiltinHandler returns the full content of a builtin domain
func ShowBuiltinHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if !builtins.IsValid(name) {
		return ToolError(fmt.Errorf("unknown builtin: %s", name))
	}

	entries, err := builtins.LoadDomainContent(name)
	if err != nil {
		return ToolError(fmt.Errorf("failed to load builtin domain content: %w", err))
	}

	grouped := map[string][]map[string]string{}
	for _, e := range entries {
		entry := map[string]string{
			keyName:    e.Name,
			keyContent: e.Content,
		}
		if e.Priority != "" {
			entry["priority"] = e.Priority
		}
		grouped[e.Type] = append(grouped[e.Type], entry)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "show_builtin",
		keyName:      name,
		keyContent:   grouped,
	})
}
