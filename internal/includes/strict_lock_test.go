package includes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyCache(t *testing.T) {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(home, ".cache", "ai-rulez")))
}

func TestLock_StrictModesFailWhenAPinnedSourceCannotBeLoaded(t *testing.T) {
	tests := []struct {
		name string
		mode LockMode
		skip bool
	}{
		{"locked with the remote gone", LockRequire, false},
		{"frozen with a cold cache", LockFrozen, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLockFixture(t)
			f.writeLock(t)
			emptyCache(t)
			require.NoError(t, os.RemoveAll(f.remote))

			Mode, SkipFetch = tt.mode, tt.skip
			_, err := f.load(t)
			require.Error(t, err, "a pinned include that cannot be loaded must not be dropped silently")
			assert.ErrorIs(t, err, config.ErrLockViolation)
		})
	}
}

func TestLock_StrictModesRefuseLocalOverride(t *testing.T) {
	f := newLockFixture(t)
	f.writeLock(t)

	override := filepath.Join(f.project, "override")
	writeTestFile(t, filepath.Join(override, ".ai-rulez", "rules", "shared.md"), "# Shared\n\nunpinned override\n")
	cfgPath := filepath.Join(f.project, ".ai-rulez", "config.toml")
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	patched := string(raw) + "\n"
	patched = strings.Replace(patched, "name = \"shared\"\n", "name = \"shared\"\nlocal_override = \""+filepath.ToSlash(override)+"\"\n", 1)
	writeTestFile(t, cfgPath, patched)

	Mode = LockRequire
	_, err = f.load(t)
	require.Error(t, err, "an unpinned local override must not be used under a strict lock")

	Mode = LockAuto
	cfg, err := f.load(t)
	require.NoError(t, err)
	assert.Contains(t, ruleBody(cfg), "unpinned override", "outside strict modes the override still applies")
}
