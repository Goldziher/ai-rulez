//go:build !windows

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
