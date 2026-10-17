package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProjectSettings(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		sub   string
		check func(t *testing.T, got ProjectSettings)
	}{
		{
			name:  "no configuration has no settings",
			files: map[string]string{"README.md": "x"},
			check: func(t *testing.T, got ProjectSettings) { assert.Equal(t, ProjectSettings{}, got) },
		},
		{
			name: "the [mcp] table of the config directory",
			files: map[string]string{
				".ai-rulez/config.toml": baseConfig + "\n[mcp]\nread_only = true\ntools = [\"find_skill\"]\nallow = [\"a-*\"]\n",
			},
			check: func(t *testing.T, got ProjectSettings) {
				require.NotNil(t, got.MCP.ReadOnly)
				assert.True(t, *got.MCP.ReadOnly)
				assert.Equal(t, []string{"find_skill"}, got.MCP.Tools)
				assert.Equal(t, []string{"a-*"}, got.MCP.Allow)
				assert.False(t, got.PolicyReadOnly)
			},
		},
		{
			name:  "the [mcp] table of a root ai-rulez.toml",
			files: map[string]string{"ai-rulez.toml": baseConfig + "\n[mcp]\ndaemon = true\n"},
			check: func(t *testing.T, got ProjectSettings) {
				require.NotNil(t, got.MCP.Daemon)
				assert.True(t, *got.MCP.Daemon)
			},
		},
		{
			name: "a subdirectory reads the settings of its project",
			files: map[string]string{
				".ai-rulez/config.toml": baseConfig + "\n[mcp]\nno_watch = true\n",
				"pkg/x/main.go":         "package x\n",
			},
			sub: "pkg/x",
			check: func(t *testing.T, got ProjectSettings) {
				require.NotNil(t, got.MCP.NoWatch)
				assert.True(t, *got.MCP.NoWatch)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tt.files {
				writeFile(t, root, rel, content)
			}

			got, err := LoadProjectSettings(context.Background(), filepath.Join(root, filepath.FromSlash(tt.sub)))

			require.NoError(t, err)
			tt.check(t, got)
		})
	}
}

func TestLoadProjectSettings_AnInvalidConfigurationIsAnError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".ai-rulez/config.toml", baseConfig+"\n[mcp\nallow = 1\n")

	_, err := LoadProjectSettings(context.Background(), root)

	require.Error(t, err)
}
