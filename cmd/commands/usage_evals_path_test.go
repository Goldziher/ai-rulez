package commands

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEvalEvents_NamedFileMustExist(t *testing.T) {
	// Arrange
	setupTelemetry(t, "", "")

	// Act
	events, err := loadEvalEvents(filepath.Join(t.TempDir(), "typo.json"))

	// Assert: a typo is an error, not a silent empty export.
	require.Error(t, err)
	assert.Empty(t, events)
}

func TestDefaultEvalStorePath_FollowsRootAndConfigDirNotTheWorkingDirectory(t *testing.T) {
	// Arrange
	env := setupTelemetry(t, "", "")
	t.Chdir(t.TempDir())

	// Act / Assert
	assert.Equal(t, evals.DefaultStorePath(filepath.Join(env.root, ".ai-rulez")), defaultEvalStorePath())

	configDir = "custom"
	t.Cleanup(func() { configDir = "" })
	assert.Equal(t, evals.DefaultStorePath(filepath.Join(env.root, "custom")), defaultEvalStorePath())
}
