package evals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserKeyReadersNeverCreateTheKey(t *testing.T) {
	// Arrange
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	keyPath := filepath.Join(xdg, "ai-rulez", ResultsKeyFile)

	// Act / Assert: a reader finds nothing and creates nothing.
	assert.Nil(t, ExistingUserKey())
	_, err := os.Lstat(keyPath)
	assert.True(t, os.IsNotExist(err))

	// The writer creates it; the reader then returns the same key.
	created := UserKey()
	require.Len(t, created, 32)
	assert.Equal(t, created, ExistingUserKey())
}
