//go:build unix

package config

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestInstalledSkillBundle_SymlinkOutsideAndFIFONeverBundled(t *testing.T) {
	// Arrange
	dir := hardeningSkill(t, map[string]string{"keep.md": "# keep\n"})
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, "link.md"))
	testutil.SymlinkOrSkip(t, filepath.Dir(outside), filepath.Join(dir, "linkdir"))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644))

	// Act
	resources, err := LoadSkillResourcesContext(context.Background(), dir)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"keep.md"}, resourcePaths(resources))
}
