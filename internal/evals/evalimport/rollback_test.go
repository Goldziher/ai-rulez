package evalimport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockedImport imports the lift scenario with --force into a directory where the
// second file of the import cannot be written (a non-empty directory sits at its
// path), so the import fails after the first file.
func blockedImport(t *testing.T, out string) error {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(out, "write-the-changelog.task.md", "keep"), 0o750))
	_, err := Run(&Options{Source: Tessl{}, Paths: []string{filepath.Join("testdata", "scenarios", "lift")}, OutDir: out, Force: true})
	return err
}

func TestRun_FailedWriteRestoresWhatItOverwrote(t *testing.T) {
	// Arrange
	out := t.TempDir()
	first := filepath.Join(out, "write-the-changelog.eval.yaml")
	require.NoError(t, os.WriteFile(first, []byte("previous\n"), 0o600))

	// Act
	err := blockedImport(t, out)

	// Assert
	require.Error(t, err)
	data, readErr := os.ReadFile(first)
	require.NoError(t, readErr)
	assert.Equal(t, "previous\n", string(data), "the file the failed import overwrote is put back")
}

func TestRun_FailedWriteRemovesFilesItCreated(t *testing.T) {
	// Arrange
	out := t.TempDir()

	// Act
	err := blockedImport(t, out)

	// Assert
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(out, "write-the-changelog.eval.yaml"), "a file the failed import created is removed")
}
