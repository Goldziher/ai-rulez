package cli

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldConfigureForTool(t *testing.T) {
	tests := []struct {
		name        string
		server      config.MCPServer
		toolName    string
		expected    bool
		description string
	}{
		{
			name: "no targets - should configure for all tools",
			server: config.MCPServer{
				Name:    "test-server",
				Command: "test",
				Targets: []string{},
			},
			toolName:    "claude",
			expected:    true,
			description: "Empty targets means configure for all available tools",
		},
		{
			name: "explicit claude targeting",
			server: config.MCPServer{
				Name:    "test-server",
				Command: "test",
				Targets: []string{"@claude-cli"},
			},
			toolName:    "claude",
			expected:    true,
			description: "Explicit @claude-cli target should configure for claude",
		},
		{
			name: "explicit gemini targeting",
			server: config.MCPServer{
				Name:    "test-server",
				Command: "test",
				Targets: []string{"@gemini-cli"},
			},
			toolName:    "gemini",
			expected:    true,
			description: "Explicit @gemini-cli target should configure for gemini",
		},
		{
			name: "wrong tool targeting",
			server: config.MCPServer{
				Name:    "test-server",
				Command: "test",
				Targets: []string{"@claude-cli"},
			},
			toolName:    "gemini",
			expected:    false,
			description: "Claude target should not configure for gemini",
		},
		{
			name: "mixed targeting with file and CLI",
			server: config.MCPServer{
				Name:    "test-server",
				Command: "test",
				Targets: []string{".cursor/mcp.json", "@claude-cli", "@gemini-cli"},
			},
			toolName:    "claude",
			expected:    true,
			description: "Mixed targets including @claude-cli should configure for claude",
		},
		{
			name: "only file targets",
			server: config.MCPServer{
				Name:    "test-server",
				Command: "test",
				Targets: []string{".cursor/mcp.json", ".mcp.json"},
			},
			toolName:    "claude",
			expected:    false,
			description: "Only file targets should not configure CLI tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldConfigureForTool(&tt.server, tt.toolName)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestBuildEnvFlags(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		flagPattern string
		expected    []string
		description string
	}{
		{
			name:        "empty env",
			env:         map[string]string{},
			flagPattern: "-e",
			expected:    nil,
			description: "Empty environment should return nil",
		},
		{
			name: "single env var",
			env: map[string]string{
				"API_KEY": "secret123",
			},
			flagPattern: "-e",
			expected:    []string{"-e", "API_KEY=secret123"},
			description: "Single environment variable should create flag pair",
		},
		{
			name: "multiple env vars",
			env: map[string]string{
				"API_KEY":    "secret123",
				"DEBUG_MODE": "true",
			},
			flagPattern: "-e",
			expected:    []string{"-e", "API_KEY=secret123", "-e", "DEBUG_MODE=true"},
			description: "Multiple environment variables should create multiple flag pairs",
		},
		{
			name: "different flag pattern",
			env: map[string]string{
				"TOKEN": "abc123",
			},
			flagPattern: "--env",
			expected:    []string{"--env", "TOKEN=abc123"},
			description: "Different flag pattern should be used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEnvFlags(tt.env, tt.flagPattern)

			assert.Equal(t, len(tt.expected), len(result), tt.description)

			if len(tt.expected) > 0 {
				assert.Equal(t, len(result)%2, 0, "Result should have even number of elements (flag-value pairs)")

				flagCount := 0
				for i := 0; i < len(result); i += 2 {
					assert.Equal(t, tt.flagPattern, result[i], "Flag should match pattern")
					assert.Contains(t, result[i+1], "=", "Value should contain = separator")
					flagCount++
				}
				assert.Equal(t, len(tt.env), flagCount, "Should have flag for each env var")
			}
		})
	}
}

func TestConfigureCLITools_EmptyServers(t *testing.T) {
	err := ConfigureCLITools([]config.MCPServer{})
	assert.NoError(t, err, "Empty server list should not cause error")
}

func TestConfigureCLITools_DisabledServer(t *testing.T) {
	disabled := false
	servers := []config.MCPServer{
		{
			Name:    "disabled-server",
			Command: "test",
			Enabled: &disabled,
		},
	}

	err := ConfigureCLITools(servers)
	assert.NoError(t, err, "Disabled servers should be skipped without error")
}

func TestValidateConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		server      config.MCPServer
		description string
	}{
		{
			name: "valid stdio server",
			server: config.MCPServer{
				Name:      "test-server",
				Command:   "test-command",
				Args:      []string{"--arg1", "value1"},
				Transport: "stdio",
				Env: map[string]string{
					"API_KEY": "secret",
				},
			},
			description: "Standard stdio server configuration",
		},
		{
			name: "valid http server",
			server: config.MCPServer{
				Name:      "http-server",
				URL:       "https://api.example.com/mcp",
				Transport: "http",
			},
			description: "HTTP server with URL",
		},
		{
			name: "valid sse server",
			server: config.MCPServer{
				Name:      "sse-server",
				URL:       "https://api.example.com/sse",
				Transport: "sse",
			},
			description: "SSE server with URL",
		},
		{
			name: "server with targets",
			server: config.MCPServer{
				Name:      "targeted-server",
				Command:   "test",
				Transport: "stdio",
				Targets:   []string{"@claude-cli", "@gemini-cli"},
			},
			description: "Server with specific CLI tool targets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEmpty(t, tt.server.Name, "Server must have a name")

			transport := tt.server.GetTransport()
			assert.Contains(t, []string{"stdio", "http", "sse"}, transport,
				"Transport should be one of the supported types")

			if transport == "stdio" {
				assert.NotEmpty(t, tt.server.Command, "stdio transport requires command")
			}

			if transport == "http" || transport == "sse" {
				assert.NotEmpty(t, tt.server.URL, "%s transport requires URL", transport)
			}

			assert.True(t, tt.server.IsEnabled(), "Server should be enabled by default")
		})
	}
}

func TestIsToolAvailable_MockBehavior(t *testing.T) {
	t.Run("tool availability check structure", func(t *testing.T) {
		result := isToolAvailable("nonexistent-tool-12345")
		assert.False(t, result, "Nonexistent tool should not be available")
	})
}
