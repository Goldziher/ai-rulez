package gitutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFileAtomic(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "exclude")
	require.NoError(t, os.WriteFile(path, []byte("old\n"), 0o600))

	// Act
	err := WriteFileAtomic(path, []byte("new\n"), 0o644)

	// Assert
	require.NoError(t, err)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "new\n", string(data))
	entries, _ := os.ReadDir(filepath.Dir(path))
	assert.Len(t, entries, 1, "no temp file is left behind")
}
