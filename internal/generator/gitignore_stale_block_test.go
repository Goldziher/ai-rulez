package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
)

func TestGenerate_RemovesTheManagedGitignoreBlockWhenGitignoreIsOff(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules", "style.md"), []byte("---\ndescription: d\n---\nUse tabs.\n"), 0o644))
	stale := "node_modules/\n\n" + gitignore.BeginMarker + "\nAGENTS.md\nCLAUDE.md\n" + gitignore.EndMarker + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(base, ".gitignore"), []byte(stale), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n"), 0o644))

	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	require.False(t, cfg.ShouldUpdateGitignore())
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	got, err := os.ReadFile(filepath.Join(base, ".gitignore"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), gitignore.BeginMarker)
	assert.NotContains(t, string(got), "AGENTS.md")
	assert.Contains(t, string(got), "node_modules/", "the user's own lines stay")
}
