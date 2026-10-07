package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestLock_RefusesAnInvalidConfiguration(t *testing.T) {
	// Arrange: a config generate rejects
	root := lockProject(t, "")
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	text := strings.Replace(string(data), `version = "5.0"`, `version = "4"`, 1)
	require.NotEqual(t, string(data), text, "the fixture sets a version to replace")
	require.NoError(t, os.WriteFile(cfgPath, []byte(text), 0o644))

	// Act
	var code int
	_, _ = capture(t, func() { code = writeLockAt("", "", nil) })

	// Assert
	assert.Equal(t, 1, code)
	_, statErr := os.Stat(lockfile.Path(filepath.Join(root, ".ai-rulez")))
	assert.True(t, os.IsNotExist(statErr), "no lock is written for an invalid configuration")
}
