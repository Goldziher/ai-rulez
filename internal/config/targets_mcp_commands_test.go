package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestFilterMCPServers(t *testing.T) {
	t.Parallel()

	servers := []config.MCPServer{
		{
			Name:        "github",
			Description: "GitHub integration",
			Targets:     []string{"*.json", "CLAUDE.md"},
		},
		{
			Name:        "postgres",
			Description: "Database server",
			Targets:     []string{"backend/*"},
		},
		{
			Name:        "universal",
			Description: "Universal server",
			Targets:     []string{},
		},
		{
			Name:        "cursor-only",
			Description: "Cursor specific server",
			Targets:     []string{".cursor/*"},
		},
	}

	tests := []struct {
		name         string
		outputPath   string
		namedTargets map[string][]string
		expected     []string
	}{
		{
			name:       "json file matches github and universal",
			outputPath: "config.json",
			expected:   []string{"github", "universal"},
		},
		{
			name:       "claude file matches github and universal",
			outputPath: "CLAUDE.md",
			expected:   []string{"github", "universal"},
		},
		{
			name:       "backend path matches postgres and universal",
			outputPath: filepath.Join("backend", "api.md"),
			expected:   []string{"postgres", "universal"},
		},
		{
			name:       "cursor path matches cursor-only and universal",
			outputPath: filepath.Join(".cursor", "mcp.json"),
			expected:   []string{"universal", "cursor-only"},
		},
		{
			name:       "random file only matches universal",
			outputPath: "random.txt",
			expected:   []string{"universal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.FilterMCPServers(servers, tt.outputPath, tt.namedTargets)
			assert.NoError(t, err)

			serverNames := make([]string, len(result))
			for i, server := range result {
				serverNames[i] = server.Name
			}

			assert.ElementsMatch(t, tt.expected, serverNames)
		})
	}
}

func TestFilterMCPServers_WithNamedTargets(t *testing.T) {
	t.Parallel()

	servers := []config.MCPServer{
		{
			Name:    "docs-server",
			Targets: []string{"@doc-files"},
		},
		{
			Name:    "api-server",
			Targets: []string{"@api-files"},
		},
	}

	namedTargets := map[string][]string{
		"doc-files": {"*.md", "docs/*"},
		"api-files": {"api/*", "*.json"},
	}

	tests := []struct {
		name       string
		outputPath string
		expected   []string
	}{
		{
			name:       "markdown file matches doc-files target",
			outputPath: "README.md",
			expected:   []string{"docs-server"},
		},
		{
			name:       "docs path matches doc-files target",
			outputPath: "docs/guide.txt",
			expected:   []string{"docs-server"},
		},
		{
			name:       "api path matches api-files target",
			outputPath: "api/endpoints.md",
			expected:   []string{"api-server"},
		},
		{
			name:       "json file matches api-files target",
			outputPath: "config.json",
			expected:   []string{"api-server"},
		},
		{
			name:       "unmatched file returns empty",
			outputPath: "random.txt",
			expected:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.FilterMCPServers(servers, tt.outputPath, namedTargets)
			assert.NoError(t, err)

			serverNames := make([]string, len(result))
			for i, server := range result {
				serverNames[i] = server.Name
			}

			assert.ElementsMatch(t, tt.expected, serverNames)
		})
	}
}

func TestFilterMCPServers_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty servers list", func(t *testing.T) {
		result, err := config.FilterMCPServers([]config.MCPServer{}, "test.md", nil)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("nil servers list", func(t *testing.T) {
		result, err := config.FilterMCPServers(nil, "test.md", nil)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("server with nil targets", func(t *testing.T) {
		servers := []config.MCPServer{
			{
				Name:    "test-server",
				Targets: nil,
			},
		}

		result, err := config.FilterMCPServers(servers, "anything.txt", nil)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "test-server", result[0].Name)
	})
}

func TestFilterCommands(t *testing.T) {
	t.Parallel()

	commands := []config.Command{
		{
			Name:        "newtask",
			Description: "Start new task",
			Targets:     []string{"*.md"},
		},
		{
			Name:        "review",
			Description: "Code review",
			Targets:     []string{"*.go", "*.js"},
		},
		{
			Name:        "universal",
			Description: "Universal command",
			Targets:     []string{},
		},
		{
			Name:        "claude-only",
			Description: "Claude specific",
			Targets:     []string{"CLAUDE.md"},
		},
	}

	tests := []struct {
		name         string
		outputPath   string
		namedTargets map[string][]string
		expected     []string
	}{
		{
			name:       "markdown file matches newtask, claude-only and universal",
			outputPath: "CLAUDE.md",
			expected:   []string{"newtask", "universal", "claude-only"},
		},
		{
			name:       "go file matches review and universal",
			outputPath: "main.go",
			expected:   []string{"review", "universal"},
		},
		{
			name:       "js file matches review and universal",
			outputPath: "app.js",
			expected:   []string{"review", "universal"},
		},
		{
			name:       "txt file only matches universal",
			outputPath: "notes.txt",
			expected:   []string{"universal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.FilterCommands(commands, tt.outputPath, tt.namedTargets)
			assert.NoError(t, err)

			commandNames := make([]string, len(result))
			for i, cmd := range result {
				commandNames[i] = cmd.Name
			}

			assert.ElementsMatch(t, tt.expected, commandNames)
		})
	}
}

func TestFilterCommands_WithNamedTargets(t *testing.T) {
	t.Parallel()

	commands := []config.Command{
		{
			Name:    "doc-cmd",
			Targets: []string{"@documentation"},
		},
		{
			Name:    "code-cmd",
			Targets: []string{"@source-code"},
		},
	}

	namedTargets := map[string][]string{
		"documentation": {"*.md", "docs/*"},
		"source-code":   {"*.go", "*.js", "src/*"},
	}

	tests := []struct {
		name       string
		outputPath string
		expected   []string
	}{
		{
			name:       "markdown file matches documentation",
			outputPath: "README.md",
			expected:   []string{"doc-cmd"},
		},
		{
			name:       "docs path matches documentation",
			outputPath: filepath.Join("docs", "api.txt"),
			expected:   []string{"doc-cmd"},
		},
		{
			name:       "go file matches source-code",
			outputPath: "main.go",
			expected:   []string{"code-cmd"},
		},
		{
			name:       "src path matches source-code",
			outputPath: filepath.Join("src", "utils.txt"),
			expected:   []string{"code-cmd"},
		},
		{
			name:       "unmatched file returns empty",
			outputPath: "config.json",
			expected:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.FilterCommands(commands, tt.outputPath, namedTargets)
			assert.NoError(t, err)

			commandNames := make([]string, len(result))
			for i, cmd := range result {
				commandNames[i] = cmd.Name
			}

			assert.ElementsMatch(t, tt.expected, commandNames)
		})
	}
}

func TestFilterCommands_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty commands list", func(t *testing.T) {
		result, err := config.FilterCommands([]config.Command{}, "test.md", nil)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("nil commands list", func(t *testing.T) {
		result, err := config.FilterCommands(nil, "test.md", nil)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("command with empty targets", func(t *testing.T) {
		commands := []config.Command{
			{
				Name:        "universal-cmd",
				Description: "Universal command",
				Targets:     []string{},
			},
		}

		result, err := config.FilterCommands(commands, "anything.txt", nil)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "universal-cmd", result[0].Name)
	})

	t.Run("command with whitespace targets", func(t *testing.T) {
		commands := []config.Command{
			{
				Name:    "test-cmd",
				Targets: []string{"*.md", "", "  ", "*.txt"},
			},
		}

		result, err := config.FilterCommands(commands, "test.md", nil)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "test-cmd", result[0].Name)
	})
}
