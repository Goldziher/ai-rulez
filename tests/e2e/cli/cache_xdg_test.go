package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// TestGenerateFromALockedIncludeUnderAReadOnlyHome guards #591: a lock pins the
// include, the home directory cannot take the cache, and an absolute
// XDG_CACHE_HOME names a writable one. The cache has to move there rather than
// failing with a read-only filesystem error that reads as a lock violation.
func TestGenerateFromALockedIncludeUnderAReadOnlyHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only directory")
	}

	repo := tagtest.New(t)
	repo.Write(".ai-rulez/rules/shared.md", "# Shared\n\nshared from the remote\n")
	repo.Commit("shared")

	project := t.TempDir()
	writeTree(t, project, map[string]string{
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\ngitignore = false\n\n" +
			"[[includes]]\nname = \"shared\"\nsource = '" + repo.URL + "'\n",
		".ai-rulez/rules/local.md": "# Local\n\nlocal rule\n",
	})

	// Write the lock the way a checkout carries it, with a home directory that
	// can hold the include cache.
	lockHome := t.TempDir()
	locked := testutil.RunCLIWithEnv(t, project, map[string]string{
		"HOME": lockHome, "USERPROFILE": lockHome,
		"XDG_CACHE_HOME": "", "XDG_CONFIG_HOME": filepath.Join(lockHome, ".config"),
	}, "lock")
	require.Equal(t, 0, locked.ExitCode, locked.Stderr)

	readOnlyHome := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.MkdirAll(readOnlyHome, 0o555))
	t.Cleanup(func() { _ = os.Chmod(readOnlyHome, 0o755) })
	cacheHome := t.TempDir()

	gen := testutil.RunCLIWithEnv(t, project, map[string]string{
		"HOME": readOnlyHome, "USERPROFILE": readOnlyHome,
		"XDG_CACHE_HOME":  cacheHome,
		"XDG_CONFIG_HOME": filepath.Join(cacheHome, "config"),
		"XDG_STATE_HOME":  filepath.Join(cacheHome, "state"),
		"XDG_DATA_HOME":   filepath.Join(cacheHome, "data"),
	}, "generate", "--locked")

	require.Equal(t, 0, gen.ExitCode,
		"generate --locked must place the cache under XDG_CACHE_HOME\nstdout: %s\nstderr: %s",
		gen.Stdout, gen.Stderr)
	generated, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(generated), "shared from the remote")

	includesDir := filepath.Join(cacheHome, "ai-rulez", "includes")
	info, err := os.Stat(includesDir)
	require.NoError(t, err, "the include cache must be under XDG_CACHE_HOME")
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "the include cache stays private to the user")
}
