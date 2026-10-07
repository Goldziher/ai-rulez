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
	cliLockPolicy = config.LockPolicy{}
	t.Cleanup(func() { cliLockPolicy = config.LockPolicy{} })
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
	applyLockFlags() // what generate sets before it loads anything
	_, err := loadForLock("")
	require.ErrorIs(t, err, config.ErrLockViolation)
	assert.Contains(t, err.Error(), "not covered by ai-rulez.lock")
	assert.Contains(t, err.Error(), "ai-rulez lock")
}

// TestApplyLockFlags pins the lock policy generate's flags give its loads: every
// generate run requires an enforced lock (RV-ENGINE-1), --locked and --frozen
// require a lock outright, and --frozen and --no-fetch stay offline.
func TestApplyLockFlags(t *testing.T) {
	tests := []struct {
		name                    string
		locked, frozen, noFetch bool
		want                    config.LockPolicy
	}{
		{name: "plain generate", want: config.LockPolicy{RequireWhenEnforced: true}},
		{name: "--no-fetch", noFetch: true, want: config.LockPolicy{RequireWhenEnforced: true, Offline: true}},
		{name: "--locked", locked: true, want: config.LockPolicy{Mode: config.LockRequire, RequireWhenEnforced: true}},
		{name: "--frozen", frozen: true, want: config.LockPolicy{Mode: config.LockFrozen, RequireWhenEnforced: true, Offline: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Cleanup(func() {
				generateLocked, generateFrozen, noFetch = false, false, false
				cliLockPolicy = config.LockPolicy{}
			})
			generateLocked, generateFrozen, noFetch = tt.locked, tt.frozen, tt.noFetch

			// Act
			applyLockFlags()

			// Assert
			assert.Equal(t, tt.want, cliLockPolicy)
		})
	}
}

func TestEnforce_OptOutFetchesTheUnlockedIncludeAsBefore(t *testing.T) {
	enforceProject(t, "[lock]\nenforce = false\n")
	cliLockPolicy.RequireWhenEnforced = true
	cfg, err := loadForLock("")
	require.NoError(t, err)
	assert.NotEmpty(t, includes.Unpinned(cfg))
}

func TestEnforce_UnpinnedRemoteIsAnErrorFindingUnderEnforceAndAWarningWithout(t *testing.T) {
	enforceProject(t, "")
	cliLockPolicy.Offline = false
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
