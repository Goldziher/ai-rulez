package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// enforceProject is a locked project that then gains a remote include the lock
// does not cover.
func enforceProject(t *testing.T, lockTable string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	includes.Mode, includes.RefreshFilter, includes.SkipFetch, includes.RequireWhenEnforced = includes.LockAuto, nil, false, false
	includes.ResetObserved()
	t.Cleanup(func() {
		includes.Mode, includes.RefreshFilter, includes.SkipFetch, includes.RequireWhenEnforced = includes.LockAuto, nil, false, false
	})
	root := lockProject(t, lockTable)
	require.Equal(t, 0, writeLockAt("", "", nil))
	remote := crossRepo(t, ".ai-rulez/rules/shared.md", "# Shared\nBe kind.\n")
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(data, []byte("\n[[includes]]\nname = \"shared\"\nsource = \"file://"+filepath.ToSlash(remote)+"\"\nref = \"v1\"\n")...), 0o644))
}

func TestEnforceDefaultsOnWhenALockExists(t *testing.T) {
	root := lockProject(t, "")
	cfg, err := loadForLock("")
	require.NoError(t, err)
	assert.False(t, cfg.LockEnforced(), "no lock file, no [lock] table")

	require.Equal(t, 0, writeLockAt("", "", nil))
	cfg, err = loadForLock("")
	require.NoError(t, err)
	assert.True(t, cfg.LockEnforced(), "a lock file turns enforcement on")

	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), lockProjectConfig+"\n[lock]\nenforce = false\n")
	cfg, err = loadForLock("")
	require.NoError(t, err)
	assert.False(t, cfg.LockEnforced(), "enforce = false opts out")
}

func TestEnforce_GenerateRefusesAnUnlockedRemoteInclude(t *testing.T) {
	enforceProject(t, "")
	includes.RequireWhenEnforced = true // what generate sets
	_, err := loadForLock("")
	require.ErrorIs(t, err, config.ErrLockViolation)
	assert.Contains(t, err.Error(), "not covered by ai-rulez.lock")
	assert.Contains(t, err.Error(), "ai-rulez lock")
}

func TestEnforce_OptOutFetchesTheUnlockedIncludeAsBefore(t *testing.T) {
	enforceProject(t, "[lock]\nenforce = false\n")
	includes.RequireWhenEnforced = true
	cfg, err := loadForLock("")
	require.NoError(t, err)
	assert.NotEmpty(t, includes.Unpinned(cfg))
}

func TestEnforce_UnpinnedRemoteIsAnErrorFindingUnderEnforceAndAWarningWithout(t *testing.T) {
	enforceProject(t, "")
	includes.SkipFetch = false
	cfg, err := loadForLock("")
	require.NoError(t, err)
	severity := func() string {
		tree, terr := lint.LoadTree(cfg.BaseDir)
		require.NoError(t, terr)
		rep, lerr := lint.Run(cfg, tree)
		require.NoError(t, lerr)
		for _, f := range rep.Findings {
			if f.Code == lint.CodeUnpinnedRemote {
				return string(f.Severity)
			}
		}
		return ""
	}
	assert.Equal(t, "error", severity())

	off := false
	cfg.Lock = &config.LockConfig{Enforce: &off}
	assert.Equal(t, "warning", severity())
}
