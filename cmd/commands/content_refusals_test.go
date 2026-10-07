package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// The library prints nothing about the symlinked content it refuses; the command
// line still shows it on stderr, where --quiet does not hide it.
func TestLoadersPrintRefusedProjectSymlinks(t *testing.T) {
	// Arrange
	project, outside := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(project, ".ai-rulez", "config.toml"), validRootConfig)
	writeFile(t, filepath.Join(outside, "secret.md"), "# secret\n")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez", "rules"), 0o755))
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "secret.md"), filepath.Join(project, ".ai-rulez", "rules", "leak.md"))

	// Act
	var err error
	stderr := captureStderr(t, func() {
		_, err = loadProject(cmdContext(), project)
	})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, stderr, "refusing symlinked content")
	assert.Contains(t, stderr, "leak.md")
}
