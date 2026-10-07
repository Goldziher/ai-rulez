package migrate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/migrate"
)

func writeProject(t *testing.T, config string, files map[string]string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(config), 0o644))
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return root, dir
}

func TestWriteRewritesFrontmatterAfterTheConfigWasAlreadyMigrated(t *testing.T) {
	root, dir := writeProject(t, "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n",
		map[string]string{"rules/fm.md": "---\ndescription: d\npermission_mode: plan\n---\nBody\n"})

	first, err := migrate.Run(migrate.Options{Root: root})
	require.NoError(t, err)
	require.False(t, first.Failed(), "%+v", first.Projects)
	second, err := migrate.Run(migrate.Options{Root: root, Write: true})
	require.NoError(t, err)

	require.False(t, second.Failed(), "%+v", second.Projects)
	assert.Equal(t, migrate.StatusMigrated, second.Projects[0].Status)
	got, err := os.ReadFile(filepath.Join(dir, "rules", "fm.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), "permission_mode")
}

func TestToleratePlusBudgetGivesAnActionableError(t *testing.T) {
	root, dir := writeProject(t, "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n\n[lint.tolerate]\nAR101 = 1\n\n[lint.budget]\nAR101 = 5\nAR102 = 2\n", nil)
	before, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)

	report, err := migrate.Run(migrate.Options{Root: root})

	require.NoError(t, err)
	require.True(t, report.Failed())
	assert.Contains(t, report.Projects[0].Error, "[lint.tolerate]")
	assert.Contains(t, report.Projects[0].Error, "[lint.budget]")
	assert.NotContains(t, report.Projects[0].Error, "already exists")
	after, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}
