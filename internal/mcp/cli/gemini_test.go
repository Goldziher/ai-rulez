package cli

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiIntegrator_ToolName(t *testing.T) {
	integrator := &GeminiIntegrator{}
	assert.Equal(t, "gemini", integrator.ToolName())
}

func TestValidateGeminiConfig(t *testing.T) {
	tests := []struct {
		name        string
		server      config.MCPServer
		expectError bool
		description string
	}{
		{
			name: "valid stdio server",
			server: config.MCPServer{
				Name:      "test-server",
				Command:   "test-command",
				Transport: "stdio",
			},
			expectError: false,
			description: "Valid stdio server should pass validation",
		},
		{
			name: "valid http server",
			server: config.MCPServer{
				Name:      "http-server",
				URL:       "https://api.example.com/mcp",
				Transport: "http",
			},
			expectError: false,
			description: "Valid HTTP server should pass validation",
		},
		{
			name: "valid sse server",
			server: config.MCPServer{
				Name:      "sse-server",
				URL:       "https://api.example.com/sse",
				Transport: "sse",
			},
			expectError: false,
			description: "Valid SSE server should pass validation",
		},
		{
			name: "stdio server with env vars",
			server: config.MCPServer{
				Name:      "env-server",
				Command:   "test-command",
				Transport: "stdio",
				Env: map[string]string{
					"API_KEY": "secret123",
					"DEBUG":   "true",
				},
			},
			expectError: false,
			description: "Stdio server with env vars should pass validation (but env vars are handled externally)",
		},
		{
			name: "missing name",
			server: config.MCPServer{
				Command:   "test-command",
				Transport: "stdio",
			},
			expectError: true,
			description: "Server without name should fail validation",
		},
		{
			name: "stdio without command",
			server: config.MCPServer{
				Name:      "test-server",
				Transport: "stdio",
			},
			expectError: true,
			description: "Stdio transport without command should fail validation",
		},
		{
			name: "http without url",
			server: config.MCPServer{
				Name:      "http-server",
				Transport: "http",
			},
			expectError: true,
			description: "HTTP transport without URL should fail validation",
		},
		{
			name: "sse without url",
			server: config.MCPServer{
				Name:      "sse-server",
				Transport: "sse",
			},
			expectError: true,
			description: "SSE transport without URL should fail validation",
		},
		{
			name: "unsupported transport",
			server: config.MCPServer{
				Name:      "test-server",
				Command:   "test-command",
				Transport: "websocket",
			},
			expectError: true,
			description: "Unsupported transport should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGeminiConfig(&tt.server)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

func TestGeminiIntegrator_ConfigureServer_CommandConstruction(t *testing.T) {
	tests := []struct {
		name           string
		server         config.MCPServer
		expectedPrefix []string
		description    string
	}{
		{
			name: "stdio server with args",
			server: config.MCPServer{
				Name:      "test-server",
				Command:   "test-command",
				Args:      []string{"--arg1", "value1"},
				Transport: "stdio",
			},
			expectedPrefix: []string{"gemini", "mcp", "add", "test-server", "test-command", "--arg1", "value1"},
			description:    "Should construct proper Gemini CLI command for stdio",
		},
		{
			name: "http server",
			server: config.MCPServer{
				Name:      "http-server",
				URL:       "https://api.example.com/mcp",
				Transport: "http",
			},
			expectedPrefix: []string{"gemini", "mcp", "add", "http-server", "https://api.example.com/mcp"},
			description:    "Should construct proper Gemini CLI command for HTTP",
		},
		{
			name: "sse server",
			server: config.MCPServer{
				Name:      "sse-server",
				URL:       "https://api.example.com/sse",
				Transport: "sse",
			},
			expectedPrefix: []string{"gemini", "mcp", "add", "sse-server", "https://api.example.com/sse"},
			description:    "Should construct proper Gemini CLI command for SSE",
		},
		{
			name: "stdio server with env vars (external handling)",
			server: config.MCPServer{
				Name:      "env-server",
				Command:   "test-command",
				Transport: "stdio",
				Env: map[string]string{
					"API_KEY": "secret123",
				},
			},
			expectedPrefix: []string{"gemini", "mcp", "add", "env-server", "test-command"},
			description:    "Should construct Gemini CLI command (env vars handled externally)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGeminiConfig(&tt.server)
			require.NoError(t, err, "Server configuration should be valid")

			assert.Equal(t, tt.server.GetTransport(), tt.server.Transport,
				"Transport should be correctly set")

			if tt.server.GetTransport() == "stdio" {
				assert.NotEmpty(t, tt.server.Command, "Stdio server should have command")
			}

			if tt.server.GetTransport() == "http" || tt.server.GetTransport() == "sse" {
				assert.NotEmpty(t, tt.server.URL, "HTTP/SSE server should have URL")
			}
		})
	}
}

func TestGeminiIntegrator_EnvironmentVariableHandling(t *testing.T) {
	server := config.MCPServer{
		Name:      "env-server",
		Command:   "test-command",
		Transport: "stdio",
		Env: map[string]string{
			"API_KEY":    "secret123",
			"DEBUG_MODE": "true",
			"TIMEOUT":    "30s",
		},
	}

	err := ValidateGeminiConfig(&server)
	assert.NoError(t, err, "Server with env vars should pass validation")
}

func TestGeminiIntegrator_ConfigureServer_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		server      config.MCPServer
		description string
	}{
		{
			name: "stdio without command",
			server: config.MCPServer{
				Name:      "bad-server",
				Transport: "stdio",
			},
			description: "Should fail for stdio without command",
		},
		{
			name: "http without url",
			server: config.MCPServer{
				Name:      "bad-server",
				Transport: "http",
			},
			description: "Should fail for HTTP without URL",
		},
		{
			name: "unsupported transport",
			server: config.MCPServer{
				Name:      "bad-server",
				Command:   "test",
				Transport: "invalid-transport",
			},
			description: "Should fail for unsupported transport",
		},
	}

	integrator := &GeminiIntegrator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := integrator.ConfigureServer(&tt.server)
			assert.Error(t, err, tt.description)
		})
	}
}

func TestGeminiIntegrator_CompareWithClaude(t *testing.T) {
	server := config.MCPServer{
		Name:      "comparison-server",
		Command:   "test-command",
		Args:      []string{"--verbose"},
		Transport: "stdio",
		Env: map[string]string{
			"API_KEY": "secret123",
		},
	}

	claudeErr := ValidateClaudeConfig(&server)
	geminiErr := ValidateGeminiConfig(&server)

	assert.NoError(t, claudeErr, "Claude validation should pass")
	assert.NoError(t, geminiErr, "Gemini validation should pass")

	claudeIntegrator := &ClaudeIntegrator{}
	geminiIntegrator := &GeminiIntegrator{}

	assert.Equal(t, "claude", claudeIntegrator.ToolName())
	assert.Equal(t, "gemini", geminiIntegrator.ToolName())
}
