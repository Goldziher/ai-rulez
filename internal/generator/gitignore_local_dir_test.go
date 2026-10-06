package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitignore_ManagedBlockAlwaysCoversTheLocalDir(t *testing.T) {
	// Arrange: gitignore = true and no .ai-rulez/local/ yet. usage record and
	// telemetry record create files there after this run.
	base, _ := generateWithGitignore(t, "claude", false)
	require.NoDirExists(t, filepath.Join(base, ".ai-rulez", "local"))

	// Act
	data, err := os.ReadFile(filepath.Join(base, ".gitignore"))

	// Assert
	require.NoError(t, err)
	assert.Contains(t, strings.Split(string(data), "\n"), ".ai-rulez/local/")
}
