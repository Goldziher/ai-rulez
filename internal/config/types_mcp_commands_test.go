package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestMCPServer_IsEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   config.MCPServer
		expected bool
	}{
		{
			name:     "enabled nil defaults to true",
			server:   config.MCPServer{Name: "test"},
			expected: true,
		},
		{
			name: "explicitly enabled",
			server: config.MCPServer{
				Name:    "test",
				Enabled: &[]bool{true}[0],
			},
			expected: true,
		},
		{
			name: "explicitly disabled",
			server: config.MCPServer{
				Name:    "test",
				Enabled: &[]bool{false}[0],
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.server.IsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMCPServer_GetTransport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   config.MCPServer
		expected string
	}{
		{
			name:     "empty transport defaults to stdio",
			server:   config.MCPServer{Name: "test"},
			expected: "stdio",
		},
		{
			name: "explicit stdio transport",
			server: config.MCPServer{
				Name:      "test",
				Transport: "stdio",
			},
			expected: "stdio",
		},
		{
			name: "http transport",
			server: config.MCPServer{
				Name:      "test",
				Transport: "http",
			},
			expected: "http",
		},
		{
			name: "sse transport",
			server: config.MCPServer{
				Name:      "test",
				Transport: "sse",
			},
			expected: "sse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.server.GetTransport()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommand_IsEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		command  config.Command
		expected bool
	}{
		{
			name:     "enabled nil defaults to true",
			command:  config.Command{Name: "test", Description: "test"},
			expected: true,
		},
		{
			name: "explicitly enabled",
			command: config.Command{
				Name:        "test",
				Description: "test",
				Enabled:     &[]bool{true}[0],
			},
			expected: true,
		},
		{
			name: "explicitly disabled",
			command: config.Command{
				Name:        "test",
				Description: "test",
				Enabled:     &[]bool{false}[0],
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.command.IsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMCPServer_CompleteConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("stdio server configuration", func(t *testing.T) {
		server := config.MCPServer{
			ID:          "test-id",
			Name:        "github",
			Description: "GitHub integration",
			Command:     "npx",
			Args:        []string{"-y", "@modelcontextprotocol/server-github"},
			Env: map[string]string{
				"GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}",
			},
			Transport: "stdio",
			Enabled:   &[]bool{true}[0],
			Targets:   []string{"*.json"},
		}

		assert.Equal(t, "test-id", server.ID)
		assert.Equal(t, "github", server.Name)
		assert.Equal(t, "GitHub integration", server.Description)
		assert.Equal(t, "npx", server.Command)
		assert.Equal(t, []string{"-y", "@modelcontextprotocol/server-github"}, server.Args)
		assert.Equal(t, "stdio", server.GetTransport())
		assert.True(t, server.IsEnabled())
		assert.Equal(t, []string{"*.json"}, server.Targets)
		assert.Equal(t, "${GITHUB_TOKEN}", server.Env["GITHUB_PERSONAL_ACCESS_TOKEN"])
	})

	t.Run("http server configuration", func(t *testing.T) {
		server := config.MCPServer{
			Name:      "remote-api",
			URL:       "https://api.example.com/mcp",
			Transport: "http",
			Enabled:   &[]bool{true}[0],
		}

		assert.Equal(t, "remote-api", server.Name)
		assert.Equal(t, "https://api.example.com/mcp", server.URL)
		assert.Equal(t, "http", server.GetTransport())
		assert.True(t, server.IsEnabled())
	})

	t.Run("disabled server", func(t *testing.T) {
		server := config.MCPServer{
			Name:    "disabled-server",
			Command: "python",
			Args:    []string{"server.py"},
			Enabled: &[]bool{false}[0],
		}

		assert.Equal(t, "disabled-server", server.Name)
		assert.Equal(t, "python", server.Command)
		assert.False(t, server.IsEnabled())
		assert.Equal(t, "stdio", server.GetTransport())
	})
}

func TestCommand_CompleteConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("full command configuration", func(t *testing.T) {
		command := config.Command{
			ID:           "test-id",
			Name:         "newtask",
			Aliases:      []string{"nt", "new"},
			Description:  "Start a new task",
			Usage:        "/newtask <description>",
			SystemPrompt: "You are starting a new task",
			Shortcut:     "Ctrl+N",
			Enabled:      &[]bool{true}[0],
			Targets:      []string{"*.md"},
		}

		assert.Equal(t, "test-id", command.ID)
		assert.Equal(t, "newtask", command.Name)
		assert.Equal(t, []string{"nt", "new"}, command.Aliases)
		assert.Equal(t, "Start a new task", command.Description)
		assert.Equal(t, "/newtask <description>", command.Usage)
		assert.Equal(t, "You are starting a new task", command.SystemPrompt)
		assert.Equal(t, "Ctrl+N", command.Shortcut)
		assert.True(t, command.IsEnabled())
		assert.Equal(t, []string{"*.md"}, command.Targets)
	})

	t.Run("minimal command configuration", func(t *testing.T) {
		command := config.Command{
			Name:        "help",
			Description: "Show help",
		}

		assert.Equal(t, "help", command.Name)
		assert.Equal(t, "Show help", command.Description)
		assert.Empty(t, command.Aliases)
		assert.Empty(t, command.Usage)
		assert.Empty(t, command.SystemPrompt)
		assert.Empty(t, command.Shortcut)
		assert.True(t, command.IsEnabled())
		assert.Empty(t, command.Targets)
	})

	t.Run("command with aliases", func(t *testing.T) {
		command := config.Command{
			Name:        "smol",
			Aliases:     []string{"compact", "summarize"},
			Description: "Condense chat history",
		}

		assert.Equal(t, "smol", command.Name)
		assert.Equal(t, []string{"compact", "summarize"}, command.Aliases)
		assert.Equal(t, "Condense chat history", command.Description)
	})
}
