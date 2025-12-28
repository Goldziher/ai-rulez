package templates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func TestMCPTemplates_Render(t *testing.T) {
	disabled := false

	data := &templates.TemplateData{
		ProjectName: "Test Project",
		MCPServers: []config.MCPServer{
			{
				Name:      "ai-rulez",
				Command:   "npx",
				Args:      []string{"-y", "ai-rulez@latest", "mcp"},
				Transport: "stdio",
			},
			{
				Name:      "remote",
				Transport: "http",
				URL:       "https://example.com/mcp",
				Enabled:   &disabled,
			},
		},
	}

	tests := []struct {
		name     string
		template string
		contains []string
	}{
		{
			name:     "claude-code-mcp",
			template: "claude-code-mcp",
			contains: []string{
				`"mcpServers"`,
				`"ai-rulez"`,
				`"command": "npx"`,
				`"type": "stdio"`,
				`"remote"`,
				`"type": "http"`,
				`"url": "https://example.com/mcp"`,
			},
		},
		{
			name:     "cursor-mcp",
			template: "cursor-mcp",
			contains: []string{
				`"McpServers"`,
				`"ai-rulez"`,
				`"command": "npx"`,
				`"remote"`,
				`"url": "https://example.com/mcp"`,
			},
		},
		{
			name:     "windsurf-mcp",
			template: "windsurf-mcp",
			contains: []string{
				`"mcpServers"`,
				`"ai-rulez"`,
				`"command": "npx"`,
				`"disabled": false`,
				`"remote"`,
				`"url": "https://example.com/mcp"`,
				`"disabled": true`,
				`"transport": "http"`,
			},
		},
		{
			name:     "vscode-mcp",
			template: "vscode-mcp",
			contains: []string{
				`"servers"`,
				`"ai-rulez"`,
				`"command": "npx"`,
				`"type": "stdio"`,
				`"remote"`,
				`"type": "http"`,
				`"url": "https://example.com/mcp"`,
			},
		},
		{
			name:     "continuedev-mcp",
			template: "continuedev-mcp",
			contains: []string{
				"mcpServers:",
				"- name: ai-rulez",
				"command: npx",
				"type: streamable-http",
				"url: https://example.com/mcp",
			},
		},
		{
			name:     "cline-mcp",
			template: "cline-mcp",
			contains: []string{
				`"mcpServers"`,
				`"ai-rulez"`,
				`"command": "npx"`,
				`"disabled": false`,
				`"remote"`,
				`"url": "https://example.com/mcp"`,
				`"disabled": true`,
			},
		},
		{
			name:     "gemini-mcp",
			template: "gemini-mcp",
			contains: []string{
				`"mcpServers"`,
				`"ai-rulez"`,
				`"command": "npx"`,
				`"type": "stdio"`,
				`"remote"`,
				`"type": "http"`,
				`"url": "https://example.com/mcp"`,
			},
		},
	}

	renderer := templates.NewRenderer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := renderer.Render(tt.template, data)
			require.NoError(t, err)
			for _, snippet := range tt.contains {
				assert.Contains(t, output, snippet)
			}
		})
	}
}
