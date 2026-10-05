package handlers

import (
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func ToolError(err error) (*sdkmcp.CallToolResult, error) {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: fmt.Sprintf("Error: %v", err)},
		},
		IsError: true,
	}, nil
}

func ToolSuccess(result interface{}) (*sdkmcp.CallToolResult, error) {
	text := fmt.Sprintf("%v", result)
	if data, err := json.Marshal(result); err == nil {
		text = string(data)
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: text},
		},
	}, nil
}
