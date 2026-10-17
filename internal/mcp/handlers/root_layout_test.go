package handlers

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const rootLayoutToml = "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n"

func writeRootLayout(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.RootConfigFilename), []byte(rootLayoutToml), 0o644))
}

func TestContentWriteHandlers_RefuseInARootLayout(t *testing.T) {
	type handler func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)
	tests := []struct {
		name    string
		handler handler
		args    map[string]any
	}{
		{"create_rule", CreateRuleHandler, map[string]any{"name": "r", "content": "x"}},
		{"update_rule", UpdateRuleHandler, map[string]any{"name": "r", "content": "x"}},
		{"delete_rule", DeleteRuleHandler, map[string]any{"name": "r"}},
		{"create_context", CreateContextHandler, map[string]any{"name": "c", "content": "x"}},
		{"update_context", UpdateContextHandler, map[string]any{"name": "c", "content": "x"}},
		{"delete_context", DeleteContextHandler, map[string]any{"name": "c"}},
		{"create_skill", CreateSkillHandler, map[string]any{"name": "s", "description": "d", "content": "x"}},
		{"update_skill", UpdateSkillHandler, map[string]any{"name": "s", "content": "x"}},
		{"delete_skill", DeleteSkillHandler, map[string]any{"name": "s"}},
		{"create_agent", CreateAgentHandler, map[string]any{"name": "a", "content": "x"}},
		{"update_agent", UpdateAgentHandler, map[string]any{"name": "a", "content": "x"}},
		{"delete_agent", DeleteAgentHandler, map[string]any{"name": "a"}},
		{"create_command", CreateCommandHandler, map[string]any{"name": "c", "content": "x"}},
		{"update_command", UpdateCommandHandler, map[string]any{"name": "c", "content": "x"}},
		{"delete_command", DeleteCommandHandler, map[string]any{"name": "c"}},
		{"create_check", CreateCheckHandler, map[string]any{"name": "c", "content": "x"}},
		{"update_check", UpdateCheckHandler, map[string]any{"name": "c", "content": "x"}},
		{"delete_check", DeleteCheckHandler, map[string]any{"name": "c"}},
		{"create_domain", CreateDomainHandler, map[string]any{"name": "d"}},
		{"delete_domain", DeleteDomainHandler, map[string]any{"name": "d"}},
		{"create_rule local", CreateRuleHandler, map[string]any{"name": "r", "content": "x", "local": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeRootLayout(t, dir)
			args := map[string]any{"working_directory": dir}
			for k, v := range tt.args {
				args[k] = v
			}

			// Act
			res, err := tt.handler(context.Background(), newRequestWithArgs(args))

			// Assert
			require.NoError(t, err)
			require.True(t, res.IsError, textOf(t, res))
			assert.Contains(t, textOf(t, res), "no .ai-rulez/ directory")
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			assert.Len(t, entries, 1, "only ai-rulez.toml is in the project")
		})
	}
}

func TestFindRecursiveConfigDirs_RootLayouts(t *testing.T) {
	// Arrange
	base := t.TempDir()
	writeRootLayout(t, filepath.Join(base, "svc-a"))
	writeRootLayout(t, filepath.Join(base, "deep", "svc-b"))
	// A directory layout above wins over the file beside it.
	writeRecursiveConfig(t, base, "both/.ai-rulez/config.toml")
	writeRootLayout(t, filepath.Join(base, "both"))
	// The .agents layout owns its own ai-rulez.toml; it is not a root layout.
	writeRecursiveConfig(t, base, "agents-proj/.agents/ai-rulez.toml")
	// Pruned trees are not projects.
	writeRootLayout(t, filepath.Join(base, "node_modules", "pkg"))
	writeRootLayout(t, filepath.Join(base, ".hidden"))

	// Act
	dirs, err := findRecursiveConfigDirs(base, "")

	// Assert
	require.NoError(t, err)
	sort.Strings(dirs)
	want := []string{
		filepath.Join(base, "agents-proj"),
		filepath.Join(base, "both"),
		filepath.Join(base, "deep", "svc-b"),
		filepath.Join(base, "svc-a"),
	}
	assert.Equal(t, want, dirs)
}

func TestFindRecursiveConfigDirs_RootLayoutBelowADirectoryRoot(t *testing.T) {
	// Arrange: a monorepo with a .ai-rulez root and a single-file package below it.
	base := t.TempDir()
	writeRecursiveConfig(t, base, ".ai-rulez/config.toml")
	writeRootLayout(t, filepath.Join(base, "packages", "tool"))

	// Act
	dirs, err := findRecursiveConfigDirs(base, "")

	// Assert
	require.NoError(t, err)
	sort.Strings(dirs)
	assert.Equal(t, []string{base, filepath.Join(base, "packages", "tool")}, dirs)
}

func TestFindRecursiveConfigDirs_ExplicitDirNameIgnoresRootLayouts(t *testing.T) {
	// Arrange
	base := t.TempDir()
	writeRootLayout(t, filepath.Join(base, "svc"))

	// Act
	dirs, err := findRecursiveConfigDirs(base, ".ai-rulez")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, dirs)
}
