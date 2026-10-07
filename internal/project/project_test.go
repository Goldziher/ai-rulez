package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

func TestLoadWiresTheResolversAndTheRegistry(t *testing.T) {
	// Arrange: a project with a local include and a built-in preset.
	dir := writeProject(t, map[string]string{
		".ai-rulez/config.toml":        "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[[includes]]\nname = \"shared\"\nsource = \"shared\"\n",
		"shared/rules/from-include.md": "# From the include\n",
	})

	// Act
	cfg, err := Load(t.Context(), dir)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, cfg.Registry)
	gen, err := cfg.Registry.Generator("claude")
	require.NoError(t, err)
	assert.Equal(t, "claude", gen.GetName())
	var names []string
	for _, r := range cfg.Content.Rules {
		names = append(names, r.Name)
	}
	assert.Contains(t, names, "from-include")
}

func TestExplicitOptionsOverrideTheDefaults(t *testing.T) {
	// Arrange
	mine := config.NewRegistry()
	dir := writeProject(t, map[string]string{
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n",
	})

	// Act
	cfg, err := Load(t.Context(), dir, config.WithRegistry(mine))

	// Assert
	require.NoError(t, err)
	assert.Same(t, mine, cfg.Registry)
}
