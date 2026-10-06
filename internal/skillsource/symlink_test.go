package skillsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_LocalSymlinkedRootIsDigestedThroughTheLink(t *testing.T) {
	real := t.TempDir()
	write(t, real, "pdf/SKILL.md", skillMD("pdf", "Work with PDF files"))
	link := filepath.Join(t.TempDir(), "skills-link")
	require.NoError(t, os.Symlink(real, link))

	viaLink, err := Resolve(context.Background(), Spec{Name: "local", URL: link, AllowOutside: true}, Options{})
	require.NoError(t, err)
	direct, err := Resolve(context.Background(), Spec{Name: "local", URL: real, AllowOutside: true}, Options{})
	require.NoError(t, err)
	assert.Equal(t, direct.Digest, viaLink.Digest)
	assert.NotEqual(t, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", viaLink.Digest, "the digest of empty input means nothing was hashed")
	assert.Equal(t, []string{"pdf"}, names(viaLink))

	write(t, real, "pdf/references/x.md", "changed\n")
	changed, err := Resolve(context.Background(), Spec{Name: "local", URL: link, AllowOutside: true}, Options{})
	require.NoError(t, err)
	assert.NotEqual(t, viaLink.Digest, changed.Digest)
}

func TestResolve_GitPathThroughASymlinkIsRefused(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	write(t, outside, "evil/SKILL.md", skillMD("evil", "Outside the repository"))
	require.NoError(t, os.Symlink(outside, filepath.Join(f.work, "linked")))
	require.NoError(t, os.Symlink("skills", filepath.Join(f.work, "alias")))
	git(t, f.work, "add", "-A")
	git(t, f.work, "commit", "--quiet", "-m", "links")
	git(t, f.work, "push", "--quiet", f.bare, "main")

	for _, p := range []string{"linked", "alias"} {
		_, err := Resolve(context.Background(), Spec{Name: "team", URL: "git+" + f.url, Path: p}, Options{CacheDir: t.TempDir()})
		require.Error(t, err, p)
		assert.Contains(t, err.Error(), "symlink", p)
	}
}
