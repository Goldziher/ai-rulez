package improve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRWriteDirs_NarrowTheCacheAndDataToAiRulez(t *testing.T) {
	// Arrange: a home whose cache and data roots hold other tools' directories too
	home := t.TempDir()
	data := filepath.Join(home, "share")
	proj := filepath.Join(home, "proj")
	for _, d := range []string{filepath.Join(home, ".cache", "other"), filepath.Join(data, "other"), proj} {
		require.NoError(t, os.MkdirAll(d, 0o750))
	}
	env := []string{"HOME=" + home, "XDG_DATA_HOME=" + data}

	// Act
	dirs := prWriteDirs(proj, env)

	// Assert
	assert.Contains(t, dirs, proj)
	assert.Contains(t, dirs, filepath.Join(home, ".cache", "ai-rulez"))
	assert.Contains(t, dirs, filepath.Join(data, "ai-rulez"))
	assert.NotContains(t, dirs, filepath.Join(home, ".cache"), "the whole user cache is not writable")
	assert.NotContains(t, dirs, data, "the whole data root is not writable")
}
