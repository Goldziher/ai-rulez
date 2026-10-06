package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// A symlinked content file must never be read: its target can be any local file
// (for example a secret), and the content lock does not pin it.
func TestScanContentTree_RefusesSymlinkedContentFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	// Arrange
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("TOP-SECRET"), 0o600))
	rules := filepath.Join(root, "rules")
	require.NoError(t, os.MkdirAll(rules, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rules, "real.md"), []byte("# real\n"), 0o644))
	testutil.SymlinkOrSkip(t, secret, filepath.Join(rules, "leak.md"))
	skill := filepath.Join(root, "skills", "evil")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	testutil.SymlinkOrSkip(t, secret, filepath.Join(skill, "SKILL.md"))

	// Act
	tree, err := ScanContentTree(root)

	// Assert
	require.NoError(t, err)
	require.Len(t, tree.Rules, 1)
	assert.Equal(t, "real", tree.Rules[0].Name)
	assert.Empty(t, tree.Skills)
}

func TestLoadContentFile_SymlinkErrorNamesFileAndNeverReadsTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("TOP-SECRET"), 0o600))
	link := filepath.Join(dir, "leak.md")
	testutil.SymlinkOrSkip(t, secret, link)

	_, err := loadContentFile(link)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "leak.md")
	assert.Contains(t, err.Error(), "symlink")
	assert.NotContains(t, err.Error(), "TOP-SECRET")
}
