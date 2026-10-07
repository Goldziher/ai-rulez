package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A caller without a context (an importer run from a cobra command that has
// none) passes nil; the git questions the load asks must still run.
func TestLoadConfigToleratesANilContext(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	skill := filepath.Join(dir, ".ai-rulez", "skills", "s")
	require.NoError(t, os.MkdirAll(filepath.Join(skill, "references"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte("version = \"4.0\"\nname = \"n\"\npresets = [\"claude\"]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: s\ndescription: Use when testing.\n---\nBody\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "references", "a.md"), []byte("# A\n"), 0o600))
	var noCtx context.Context

	// Act
	cfg, err := LoadConfig(noCtx, dir, WithoutRemote()) //nolint:staticcheck // the nil context is the point

	// Assert
	require.NoError(t, err)
	require.Len(t, cfg.Content.Skills, 1)
}
