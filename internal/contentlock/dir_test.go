package contentlock

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTreeFile(t *testing.T, dir, rel, body string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), mode))
	require.NoError(t, os.Chmod(p, mode))
}

// The expected digests come from the independent Python implementation in
// docs/lockfile.md.
func TestDigestDir_Vectors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the execute bit comes from the git index on Windows")
	}
	dir := t.TempDir()
	writeTreeFile(t, dir, "rules/a.md", "# A\n", 0o644)
	writeTreeFile(t, dir, "hooks/x.sh", "#!/bin/sh\n", 0o755)
	// ignored: VCS metadata and cache bookkeeping
	writeTreeFile(t, dir, ".git/HEAD", "ref", 0o644)
	writeTreeFile(t, dir, ".cache_meta.json", "{}", 0o644)

	got, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.Equal(t, "sha256:94a2c6de5e10aa7eef64adb55330c4a84c02d49566d56adcedbd7f5db2ddf01c", got)
	got, err = DigestDir(KindOKFInclude, dir)
	require.NoError(t, err)
	assert.Equal(t, "sha256:990b39514f6589c5e61d584b7f59173cb8b1a3bd260ee93af92ebedc819da5e7", got, "every tree kind is its own domain")
}

func TestDigestDir_SensitiveToContentModeAndScriptLineEndings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the execute bit comes from the git index on Windows")
	}
	dir := t.TempDir()
	writeTreeFile(t, dir, "x/one.md", "1", 0o644)
	writeTreeFile(t, dir, "run.sh", "echo\n", 0o644)
	base, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)

	writeTreeFile(t, dir, "x/one.md", "changed", 0o644)
	changed, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.NotEqual(t, base, changed, "content changes the digest")
	writeTreeFile(t, dir, "x/one.md", "1", 0o644)

	writeTreeFile(t, dir, "run.sh", "echo\n", 0o755)
	exec, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.NotEqual(t, base, exec, "the executable bit changes the digest")
	writeTreeFile(t, dir, "run.sh", "echo\n", 0o700)
	ownerOnly, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.Equal(t, exec, ownerOnly, "any execute bit is the same mode")

	writeTreeFile(t, dir, "run.sh", "echo\r\n", 0o644)
	crlf, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.NotEqual(t, base, crlf, "a script is hashed byte for byte")
	writeTreeFile(t, dir, "x/one.md", "1", 0o644)
	writeTreeFile(t, dir, "run.sh", "echo\n", 0o644)

	writeTreeFile(t, dir, "x/one.md", "a\r\nb", 0o644)
	a, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	writeTreeFile(t, dir, "x/one.md", "a\nb", 0o644)
	b, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.Equal(t, a, b, "a document is line-ending normalized")
}

func TestDigestDir_RefusesASymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(real, "a.md"), []byte("a"), 0o644))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))

	_, err := DigestDir(KindInclude, link)
	require.Error(t, err, "a symlink root would otherwise digest to the empty hash")
	_, err = DigestDir(KindInclude, real)
	require.NoError(t, err)
}
