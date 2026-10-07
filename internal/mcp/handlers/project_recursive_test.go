package handlers

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeRecursiveConfig(t *testing.T, base, rel string) {
	t.Helper()
	path := filepath.Join(base, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("version = \"5.0\"\nname = \"x\"\n"), 0o644))
}

func TestFindRecursiveConfigDirs_ConfigConvention(t *testing.T) {
	base := t.TempDir()

	writeRecursiveConfig(t, base, ".ai-rulez/config.toml")
	writeRecursiveConfig(t, base, "service-a/.config/ai-rulez/config.toml")
	// Both layouts in one directory: .ai-rulez wins.
	writeRecursiveConfig(t, base, "service-b/.ai-rulez/config.toml")
	writeRecursiveConfig(t, base, "service-b/.config/ai-rulez/config.toml")
	// A different tool's .config subtree must not be discovered or descended.
	writeRecursiveConfig(t, base, ".config/other-tool/config.toml")
	writeRecursiveConfig(t, base, ".config/other-tool/.config/ai-rulez/config.toml")
	// Pruned dirs.
	writeRecursiveConfig(t, base, "node_modules/pkg/.config/ai-rulez/config.toml")

	dirs, err := findRecursiveConfigDirs(base, "")
	require.NoError(t, err)
	sort.Strings(dirs)

	want := []string{base, filepath.Join(base, "service-a"), filepath.Join(base, "service-b")}
	sort.Strings(want)
	assert.Equal(t, want, dirs)
}

func TestFindRecursiveConfigDirs_ExplicitNestedDir(t *testing.T) {
	base := t.TempDir()

	writeRecursiveConfig(t, base, "svc/.config/ai-rulez/config.toml")
	// The convention fallback must not fire when an explicit dir is requested.
	writeRecursiveConfig(t, base, "other/.config/ai-rulez/config.toml")

	dirs, err := findRecursiveConfigDirs(base, ".config/ai-rulez")
	require.NoError(t, err)
	sort.Strings(dirs)

	// Both directories literally contain .config/ai-rulez when it is the
	// explicit target, so both are returned.
	want := []string{filepath.Join(base, "other"), filepath.Join(base, "svc")}
	sort.Strings(want)
	assert.Equal(t, want, dirs)
}
