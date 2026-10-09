package handlers

import (
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"
)

// ToolError is the error result of a tool: IsError is set and the text is the
// error, followed by the hint the CLI would print for it.
func ToolError(err error) (*sdkmcp.CallToolResult, error) {
	text := fmt.Sprintf("Error: %v", err)
	if hint := hintOf(err); hint != "" {
		text += "\nHint: " + hint
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
		IsError: true,
	}, nil
}

// hintOf returns the remediation hint an error carries, or "".
func hintOf(err error) string {
	if oopsErr, ok := oops.AsOops(err); ok {
		return oopsErr.Hint()
	}
	return ""
}

// toolErrorDocument is an error result whose text is a JSON document. The
// server also returns the document as structuredContent, so a client can read
// why the call failed without parsing prose.
func toolErrorDocument(doc map[string]interface{}) (*sdkmcp.CallToolResult, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return ToolError(err)
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
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
