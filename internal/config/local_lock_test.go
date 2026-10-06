//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestLocalDoc_LockTimesOutInsteadOfHanging(t *testing.T) {
	// Arrange
	_, configDir := overlayProject(t, securityShared)
	held, err := OpenLocalDoc(configDir, "config.toml")
	require.NoError(t, err)
	t.Cleanup(held.Close)
	oldTimeout, oldPoll := localLockTimeout, localLockPoll
	localLockTimeout, localLockPoll = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { localLockTimeout, localLockPoll = oldTimeout, oldPoll })

	// Act
	start := time.Now()
	second, err := OpenLocalDoc(configDir, "config.toml")

	// Assert
	require.Error(t, err)
	assert.Nil(t, second)
	assert.Contains(t, err.Error(), "another ai-rulez process is editing")
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestLocalDoc_LockRefusesToFollowASymlink(t *testing.T) {
	// Arrange: the lock path is a symlink to a file that must stay untouched
	_, configDir := overlayProject(t, securityShared)
	victim := filepath.Join(t.TempDir(), "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("keep"), 0o644))
	testutil.SymlinkOrSkip(t, victim, filepath.Join(configDir, localLockName))

	// Act
	d, err := OpenLocalDoc(configDir, "config.toml")

	// Assert
	require.Error(t, err)
	assert.Nil(t, d)
	info, statErr := os.Stat(victim)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}
