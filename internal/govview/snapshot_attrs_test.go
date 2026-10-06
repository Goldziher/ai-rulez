package govview

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRevision_KeepsTheCommittedFilesAsTheyAre(t *testing.T) {
	// Arrange: a committed .gitattributes tries to hide one file (export-ignore)
	// and rewrite another (export-subst); one script is committed executable.
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	snapGit(t, dir, "init", "-q", "-b", "main")
	cfg := filepath.Join(dir, ".ai-rulez")
	snapWrite(t, filepath.Join(cfg, "rules", "a.md"), "one\n")
	snapWrite(t, filepath.Join(cfg, "rules", "hidden.md"), "must stay\n")
	snapWrite(t, filepath.Join(cfg, "rules", "subst.md"), "id $Format:%H$\n")
	snapWrite(t, filepath.Join(cfg, "verifiers", "check.sh"), "#!/bin/sh\n")
	require.NoError(t, os.Chmod(filepath.Join(cfg, "verifiers", "check.sh"), 0o755))
	snapWrite(t, filepath.Join(dir, ".gitattributes"), ".ai-rulez/rules/hidden.md export-ignore\n.ai-rulez/rules/subst.md export-subst\n")
	snapGit(t, dir, "add", "-A")
	snapGit(t, dir, "commit", "-q", "-m", "one")
	dest := t.TempDir()

	// Act
	snap, err := ExtractRevision(context.Background(), dir, "HEAD", ".ai-rulez", dest)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 4, snap.Files)
	hidden, err := os.ReadFile(filepath.Join(dest, ".ai-rulez", "rules", "hidden.md"))
	require.NoError(t, err, "export-ignore does not remove a file from the revision")
	assert.Equal(t, "must stay\n", string(hidden))
	subst, err := os.ReadFile(filepath.Join(dest, ".ai-rulez", "rules", "subst.md"))
	require.NoError(t, err)
	assert.Equal(t, "id $Format:%H$\n", string(subst), "export-subst does not rewrite a file")
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(filepath.Join(dest, ".ai-rulez", "verifiers", "check.sh"))
		require.NoError(t, statErr)
		assert.NotZero(t, info.Mode().Perm()&0o100, "a file committed executable stays executable")
		plain, statErr := os.Stat(filepath.Join(dest, ".ai-rulez", "rules", "a.md"))
		require.NoError(t, statErr)
		assert.Zero(t, plain.Mode().Perm()&0o111)
	}
}
