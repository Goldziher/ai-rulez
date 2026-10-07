package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateCommand(t *testing.T) {
	assert.NotNil(t, MigrateCmd)
	assert.Equal(t, "migrate v5", MigrateCmd.Use)
	assert.NotNil(t, MigrateCmd.Run)
	assert.NotNil(t, MigrateCmd.Args)
}

func TestMigrateCommand_Flags(t *testing.T) {
	for _, name := range []string{"dry-run", "check", "adopt-defaults", "write", "recursive", "format"} {
		assert.NotNil(t, MigrateCmd.Flags().Lookup(name), name)
	}
}

func TestMigrateCommand_ExitCodes(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"), 0o644))
	t.Chdir(project)

	require.NoError(t, MigrateCmd.Flags().Set("check", "true"))
	t.Cleanup(func() { _ = MigrateCmd.Flags().Set("check", "false") })
	assert.Equal(t, 2, runMigrate(os.Stderr, "v5"), "pending migration under --check is drift")
	assert.Equal(t, 1, runMigrate(os.Stderr, "v4"), "only v5 is a target")

	require.NoError(t, MigrateCmd.Flags().Set("check", "false"))
	assert.Equal(t, 0, runMigrate(os.Stderr, "v5"))
	assert.Equal(t, 0, runMigrate(os.Stderr, "v5"), "second run has nothing to do")
}
