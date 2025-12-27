package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

func CreateContinueDevConfig() error {
	continueDir := ".continue"
	if err := os.MkdirAll(continueDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .continue directory: %w", err)
	}

	configContent := `{
  "models": [
    {
      "title": "Claude 3.5 Sonnet",
      "provider": "anthropic",
      "model": "claude-3-5-sonnet-20241022",
      "apiKey": "[API_KEY]"
    }
  ],
  "customCommands": [
    {
      "name": "test",
      "prompt": "{{{ input }}}\n\nWrite comprehensive unit tests for the selected code. Ensure the tests cover all edge cases and functions.",
      "description": "Write unit tests for the selected code"
    }
  ],
  "contextProviders": [
    {
      "name": "code",
      "params": {}
    },
    {
      "name": "docs",
      "params": {}
    }
  ],
  "slashCommands": [
    {
      "name": "edit",
      "description": "Edit the selected code"
    },
    {
      "name": "comment",
      "description": "Add comments to the selected code"
    }
  ]
}`

	configPath := filepath.Join(continueDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return fmt.Errorf("failed to write continue config: %w", err)
	}

	return nil
}
