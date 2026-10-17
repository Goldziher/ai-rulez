package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// A fresh clone has the committed outputs and none of the machine-local files: the record of which hook
// entries ai-rulez wrote is gone. generate there must leave the committed settings document as it is.
func TestGenerate_FreshCloneKeepsCommittedHooksAndSelfServer(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "user hooks", config: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\nsession_start = { generate = false }\n\n" +
			"[[hooks]]\nevent = \"Stop\"\n[[hooks.hooks]]\ncommand = \"echo bye\"\n"},
		{name: "the guard hook", config: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\nsession_start = { generate = false }\n\n[guard]\ngenerated = true\n"},
		{name: "the session-start hook", config: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeProject(t, tt.config, nil)
			generateProject(t, root)
			settings := filepath.Join(root, ".claude", "settings.json")
			mcp := filepath.Join(root, ".mcp.json")
			settingsBefore, err := os.ReadFile(settings)
			require.NoError(t, err)
			mcpBefore, err := os.ReadFile(mcp)
			require.NoError(t, err)
			require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", ".generated-manifest.local.json")))

			generateProject(t, root)

			settingsAfter, err := os.ReadFile(settings)
			require.NoError(t, err)
			mcpAfter, err := os.ReadFile(mcp)
			require.NoError(t, err)
			assert.Equal(t, string(settingsBefore), string(settingsAfter), "the committed hooks stay")
			assert.Equal(t, string(mcpBefore), string(mcpAfter), "the self server stays")
		})
	}
}

func TestLockOutputs_LeaveOutADocumentWithOwnedHookGroups(t *testing.T) {
	root := writeProject(t, "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n", nil)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)

	outputs, err := NewGenerator(cfg).LockOutputs("default")

	require.NoError(t, err)
	var paths []string
	for _, o := range outputs {
		paths = append(paths, filepath.ToSlash(o.Path))
	}
	assert.NotContains(t, paths, ".claude/settings.json", "a clone cannot tell the document is wholly ai-rulez's")
	assert.Contains(t, paths, "CLAUDE.md")
}
