package handlers

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const agentsMCPConfig = "version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\n"

func writeAgentsMCPFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestCreateRuleWritesIntoTheAgentsTree(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		configBody string
	}{
		{"config in .agents", ".agents/ai-rulez.toml", agentsMCPConfig},
		{"source key in .ai-rulez", ".ai-rulez/config.toml", agentsMCPConfig + "source = \"agents\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeAgentsMCPFile(t, dir, tt.configPath, tt.configBody)

			// Act
			res, err := CreateRuleHandler(context.Background(),
				newRequestWithArgs(map[string]any{"working_directory": dir, "name": "style", "content": "# Style\n"}))

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			assert.FileExists(t, filepath.Join(dir, ".agents", "rules", "style.md"))
			assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez", "rules"))
		})
	}
}

func TestFindRecursiveConfigDirsFindsAgentsConfigs(t *testing.T) {
	base := t.TempDir()
	writeAgentsMCPFile(t, base, "svc-a/.agents/ai-rulez.toml", agentsMCPConfig)
	// Both in one project: .agents wins, the project is listed once.
	writeAgentsMCPFile(t, base, "svc-b/.agents/ai-rulez.toml", agentsMCPConfig)
	writeAgentsMCPFile(t, base, "svc-b/.ai-rulez/config.toml", agentsMCPConfig)
	// An .agents directory that is only some other tool's is not a root.
	writeAgentsMCPFile(t, base, "svc-c/.agents/skills/a/SKILL.md", "x")
	writeAgentsMCPFile(t, base, "svc-d/.ai-rulez/config.toml", agentsMCPConfig)

	dirs, err := findRecursiveConfigDirs(base, "")
	require.NoError(t, err)
	sort.Strings(dirs)

	assert.Equal(t, []string{filepath.Join(base, "svc-a"), filepath.Join(base, "svc-b"), filepath.Join(base, "svc-d")}, dirs)
}
