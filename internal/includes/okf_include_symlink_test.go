package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestConvertOKFBundleSkipsSymlinks(t *testing.T) {
	// Arrange
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("---\ntype: Decision\n---\nsecret\n"), 0o644))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"), []byte("---\ntype: Decision\n---\nreal\n"), 0o644))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, "linked"))
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "secret.md"), filepath.Join(dir, "link.md"))
	// Act
	tree, err := convertOKFBundle(dir, "kb", nil)
	// Assert
	require.NoError(t, err)
	require.Len(t, tree.Rules, 1)
	assert.Equal(t, "real", tree.Rules[0].Name)
}
