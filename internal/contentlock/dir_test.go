package contentlock

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
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
	testutil.SymlinkOrSkip(t, real, link)

	_, err := DigestDir(KindInclude, link)
	require.Error(t, err, "a symlink root would otherwise digest to the empty hash")
	_, err = DigestDir(KindInclude, real)
	require.NoError(t, err)
}

func TestDigestDir_SymlinkLeafVectors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	// The expected digests come from an independent Python implementation of the
	// scheme in docs/lockfile.md.
	dir := t.TempDir()
	writeTreeFile(t, dir, "rules/a.md", "# A\n", 0o644)
	testutil.SymlinkOrSkip(t, "rules/a.md", filepath.Join(dir, "README.md"))

	got, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.Equal(t, "sha256:1b842d9b161a3808fb191ac281ba28c2ed27604486a5292d3e2da5d7a70345eb", got)

	require.NoError(t, os.Remove(filepath.Join(dir, "README.md")))
	testutil.SymlinkOrSkip(t, "other.md", filepath.Join(dir, "README.md"))
	got, err = DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.Equal(t, "sha256:550880810dde3cc343d893888958939a4263f3d2cc9636193ddeffb337bc989b", got, "retargeting a link changes the digest")
}

func TestDigestDir_RecordsSymlinksWithoutFollowingThem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("TOP-SECRET"), 0o600))
	otherDir := t.TempDir()
	tests := []struct {
		name   string
		link   string
		target string
	}{
		{"file link", "rules/leak.md", secret},
		{"directory link", "rules/dir", otherDir},
		{"dangling link", "rules/gone.md", filepath.Join(otherDir, "missing")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeTreeFile(t, dir, "rules/a.md", "# A\n", 0o644)
			plain, err := DigestDir(KindInclude, dir)
			require.NoError(t, err)
			testutil.SymlinkOrSkip(t, tt.target, filepath.Join(dir, tt.link))

			// Act
			withLink, err := DigestDir(KindInclude, dir)
			require.NoError(t, err)
			require.NoError(t, os.Remove(filepath.Join(dir, tt.link)))
			testutil.SymlinkOrSkip(t, tt.target+"-moved", filepath.Join(dir, tt.link))
			moved, err := DigestDir(KindInclude, dir)
			require.NoError(t, err)

			// Assert
			assert.NotEqual(t, plain, withLink, "the link is a leaf of its own")
			assert.NotEqual(t, withLink, moved, "a changed link target changes the digest")
		})
	}
}

func TestDigestDir_TargetContentBehindALinkIsNotPinned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	target := filepath.Join(t.TempDir(), "t.md")
	require.NoError(t, os.WriteFile(target, []byte("one"), 0o644))
	dir := t.TempDir()
	testutil.SymlinkOrSkip(t, target, filepath.Join(dir, "link.md"))
	before, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, []byte("two"), 0o644))
	after, err := DigestDir(KindInclude, dir)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a link is never followed, only its target string is pinned")
}

func TestTreeModes_ReadsTheGitIndexOncePerTree(t *testing.T) {
	// Arrange
	calls := 0
	tracked := func(string) (map[string]uint32, bool, error) {
		calls++
		return map[string]uint32{"a.sh": 0o100755, "b.md": 0o100644}, true, nil
	}
	modeOf := treeModes("windows", "/tree", tracked)

	// Act
	a := modeOf(treeFile{rel: "a.sh"})
	b := modeOf(treeFile{rel: "b.md"})
	c := modeOf(treeFile{rel: "untracked.md"})

	// Assert
	assert.Equal(t, 1, calls)
	assert.Equal(t, ModeExecutable, a)
	assert.Equal(t, ModeRegular, b)
	assert.Equal(t, ModeRegular, c)
}

func TestDigestDir_CacheMetaIsExcludedOnlyAtTheRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the execute bit comes from the git index on Windows")
	}
	base := t.TempDir()
	writeTreeFile(t, base, "rules/a.md", "# A\n", 0o644)
	want, err := DigestDir(KindInclude, base)
	require.NoError(t, err)

	tests := []struct {
		name     string
		rel      string
		excluded bool
	}{
		{"root meta", ".cache_meta.json", true},
		{"root meta temp file", ".cache_meta.json.tmp", true},
		{"nested meta", "rules/.cache_meta.json", false},
		{"root prefixed name", ".cache_meta.json.evil.md", false},
		{"root suffixed name", ".cache_meta.jsonl", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTreeFile(t, dir, "rules/a.md", "# A\n", 0o644)
			writeTreeFile(t, dir, tt.rel, "x", 0o644)

			got, err := DigestDir(KindInclude, dir)

			require.NoError(t, err)
			if tt.excluded {
				assert.Equal(t, want, got)
			} else {
				assert.NotEqual(t, want, got, "authored content must stay pinned")
			}
		})
	}
}

// The streaming digest must equal TreeDigest over the same bytes, including a
// CRLF split across two read chunks and the large-file path.
func TestDigestDir_StreamingMatchesTreeDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the execute bit comes from the git index on Windows")
	}
	old := streamChunk
	streamChunk = 7
	t.Cleanup(func() { streamChunk = old })

	files := map[string]string{
		"a.md":          "one\r\ntwo\r\n\r\nthree\r",
		"b.md":          "\r\n\r\n",
		"c.md":          "x\r",
		"d.md":          "",
		"e.sh":          "echo\r\nbye\r\n",
		"f.md":          strings.Repeat("line\r\n", 1000),
		"sub/g.yaml":    "k: v\r\n",
		"sub/h.md":      "\r\r\n\r",
		"sub/deep/i.md": "no newline",
	}
	dir := t.TempDir()
	var leaves []Leaf
	for rel, body := range files {
		writeTreeFile(t, dir, rel, body, 0o644)
		leaves = append(leaves, Leaf{Path: rel, Mode: ModeRegular, Data: []byte(body)})
	}
	want, err := TreeDigest(KindInclude, leaves)
	require.NoError(t, err)

	got, err := DigestDir(KindInclude, dir)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}
