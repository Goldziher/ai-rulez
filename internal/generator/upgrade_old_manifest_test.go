package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// legacyLeftovers are files an older ai-rulez wrote that v5 no longer does; the
// old manifest lists them, none carries a Content-Hash and no digest was recorded.
var legacyLeftovers = map[string]string{
	".codex/skills/old/SKILL.md": "---\nname: old\ndescription: Old skill\n---\nOld.\n",
	".codex/commands/iterate.md": "# /iterate\n\nIterate.\n",
	".agents/agents/helper.md":   "---\nname: helper\n---\nHelp.\n",
	".github/commands/review.md": "# /review\n\nReview.\n",
	".claude/settings.json":      "{\n  \"mcpServers\": {\n    \"api\": {\n      \"args\": [\"--port\", \"1\"],\n      \"command\": \"srv\"\n    }\n  }\n}\n",
	"notes/old-rules.md":         "not a path any preset writes\n",
}

// oldManifestProject is a project upgraded from an older ai-rulez: its outputs
// and a committed manifest without merged claims or digests are on disk.
func oldManifestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`version = "4.0"
name = "upgrade"
presets = ["claude", "codex"]
gitignore = false

[[mcp_servers]]
name = "api"
command = "srv"
args = ["--port", "1"]
`), 0o644))
	files := make([]string, 0, len(legacyLeftovers))
	for rel, body := range legacyLeftovers {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
		files = append(files, rel)
	}
	manifest, err := json.Marshal(map[string]any{"version": "1", "files": files})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, generatedManifestName), manifest, 0o644))
	return dir
}

func TestUpgrade_FromAnOldManifestWithRetiredOutputs(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, dir string, warned []string)
	}{
		{
			name: "unprovable leftovers are kept",
			check: func(t *testing.T, dir string, _ []string) {
				t.Helper()
				for rel := range legacyLeftovers {
					assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(rel)))
				}
			},
		},
		{
			name: "they are reported once per reason, not once per file",
			check: func(t *testing.T, _ string, warned []string) {
				t.Helper()
				assert.Equal(t, 1, countContaining(warned, "cannot verify that ai-rulez generated 4 file(s)"), warned)
				assert.Equal(t, 1, countContaining(warned, "1 file(s) the previous manifest lists are not at a path"), warned)
				assert.Zero(t, countContaining(warned, "Stale file not removed"), warned)
			},
		},
		{
			name: "Claude's server leaves .claude/settings.json for .mcp.json",
			check: func(t *testing.T, dir string, _ []string) {
				t.Helper()
				settings, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
				require.NoError(t, err)
				assert.NotContains(t, string(settings), "mcpServers")
				mcp, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
				require.NoError(t, err)
				assert.Contains(t, string(mcp), `"srv"`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietWarnings(t)
			// Arrange
			dir := oldManifestProject(t)
			rec, host := capturedMergeWarnings()
			cfg, err := config.LoadConfig(context.Background(), dir, host)
			require.NoError(t, err)

			// Act
			err = NewGenerator(cfg).Generate("default")

			// Assert
			require.NoError(t, err)
			tt.check(t, dir, rec.Level("WARN"))
		})
	}
}

func TestUpgrade_CheckIsCleanAfterTheUpgradeRun(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := oldManifestProject(t)
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))

	// Act
	drift, err := newProjectGenerator(t, dir).CheckDrift("default")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, drift)
}
