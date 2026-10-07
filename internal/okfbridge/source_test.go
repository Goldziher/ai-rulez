package okfbridge_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSource(t *testing.T) {
	cases := map[string]okfbridge.Source{
		"./bundle":                                {Dir: "./bundle"},
		"/abs/bundle":                             {Dir: "/abs/bundle"},
		"https://github.com/o/r":                  {URL: "https://github.com/o/r"},
		"https://github.com/o/r.git@v1.2":         {URL: "https://github.com/o/r.git", Ref: "v1.2"},
		"https://github.com/o/r@release/1.0#docs": {URL: "https://github.com/o/r", Ref: "release/1.0", Subdir: "docs"},
		"https://user@github.com/o/r":             {URL: "https://user@github.com/o/r"},
		"git@github.com:o/r.git":                  {URL: "git@github.com:o/r.git"},
		"git@github.com:o/r.git@main":             {URL: "git@github.com:o/r.git", Ref: "main"},
		"file:///tmp/repo@abc123#bundles/x":       {URL: "file:///tmp/repo", Ref: "abc123", Subdir: "bundles/x"},
	}
	for in, want := range cases {
		got, err := okfbridge.ParseSource(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "http://x.y/r", "https://x.y/r#../../etc", "https://x.y/r@-upload-pack=evil", "https://x.y/r#/abs", `https://x.y/r#\abs`, "https://x.y/r#C:/abs"} {
		_, err := okfbridge.ParseSource(bad)
		assert.Error(t, err, bad)
	}
}

// fileURL builds a file:// URL for a local path; Windows paths need a leading
// slash and forward slashes (file:///C:/dir).
func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

func TestFetchLocalGitRepoAtRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "--quiet", "-b", "main")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "kb"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "kb", "a.md"), []byte("---\ntype: Decision\n---\nv1\n"), 0o644))
	run("add", ".")
	run("commit", "--quiet", "-m", "one")
	run("tag", "v1")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "kb", "a.md"), []byte("---\ntype: Decision\n---\nv2\n"), 0o644))
	run("commit", "--quiet", "-am", "two")

	src, err := okfbridge.ParseSource(fileURL(repo) + "@v1#kb")
	require.NoError(t, err)
	dir, cleanup, err := src.Fetch(context.Background())
	require.NoError(t, err)
	defer cleanup()
	data, err := os.ReadFile(filepath.Join(dir, "a.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "v1")

	src, err = okfbridge.ParseSource(fileURL(repo) + "#kb")
	require.NoError(t, err)
	dir2, cleanup2, err := src.Fetch(context.Background())
	require.NoError(t, err)
	defer cleanup2()
	data, err = os.ReadFile(filepath.Join(dir2, "a.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "v2")
}

func TestFetchRejectsSymlinkedSubdir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "s.md"), []byte("---\ntype: Decision\n---\nhost file\n"), 0o644))
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "--quiet", "-b", "main")
	testutil.SymlinkOrSkip(t, secret, filepath.Join(repo, "sub"))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "real"), 0o755))
	testutil.SymlinkOrSkip(t, secret, filepath.Join(repo, "real", "inner"))
	run("add", ".")
	run("commit", "--quiet", "-m", "one")

	for _, sub := range []string{"sub", "real/inner"} {
		src, err := okfbridge.ParseSource("file://" + repo + "#" + sub)
		require.NoError(t, err)
		dir, cleanup, err := src.Fetch(context.Background())
		if err == nil {
			cleanup()
		}
		assert.ErrorContains(t, err, "symlink", "%s resolved to %s", sub, dir)
	}
}
