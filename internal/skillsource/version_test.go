package skillsource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
)

func versionRepo(t *testing.T) *tagtest.Repo {
	t.Helper()
	r := tagtest.New(t)
	r.Write("skills/pdf/SKILL.md", skillMD("pdf", "Work with PDF files"))
	r.Write("skills/pdf/references/forms.md", "forms v1\n")
	r.Commit("one")
	r.Tag("v1.0.0")
	r.Write("skills/pdf/references/forms.md", "forms v1.1\n")
	r.Commit("two")
	r.AnnotatedTag("v1.1.0")
	r.Write("skills/pdf/references/forms.md", "forms v2\n")
	r.Commit("three")
	r.Tag("v2.0.0")
	return r
}

func TestResolve_VersionConstraintPicksAndRecordsTheTag(t *testing.T) {
	r := versionRepo(t)
	spec := Spec{Name: "team", URL: "git+" + r.URL, Version: "^1", Path: "skills"}

	res, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Refresh: true})

	require.NoError(t, err)
	assert.Equal(t, r.TagCommit("v1.1.0"), res.Commit)
	assert.Equal(t, "v1.1.0", res.Tag)
	assert.Equal(t, r.TagObject("v1.1.0"), res.TagObject)
	entry := res.Entry()
	assert.Equal(t, "^1", entry.Ref, "the lock records the constraint as the requested ref")
	assert.Equal(t, "v1.1.0", entry.Tag)
	assert.True(t, entry.Covers(spec.Want()))
}

func TestResolve_ACoveredVersionPinIsUsedWithoutResolving(t *testing.T) {
	r := versionRepo(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: "git+" + r.URL, Version: "^1", Path: "skills"}
	first, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Refresh: true})
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, first.Entry())
	r.Write("skills/pdf/references/forms.md", "forms v1.5\n")
	r.Commit("four")
	r.Tag("v1.5.0")

	locked, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock})
	require.NoError(t, err)
	kept, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock, Refresh: true})
	require.NoError(t, err)
	advance := config.VersionPolicy{Advance: func(kind, name string) bool { return kind == lockfile.KindSource }}
	advanced, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock, Refresh: true, Version: advance})
	require.NoError(t, err)

	assert.Equal(t, first.Commit, locked.Commit)
	assert.True(t, locked.Locked)
	assert.Equal(t, "v1.1.0", kept.Tag, "lock keeps a pin that still satisfies the constraint")
	assert.Equal(t, "v1.5.0", advanced.Tag, "update advances it")
}

func TestResolve_AMovedVersionTagFailsWithAR732(t *testing.T) {
	r := versionRepo(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: "git+" + r.URL, Version: "^1", Path: "skills"}
	first, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Refresh: true})
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, first.Entry())
	r.Write("skills/pdf/references/forms.md", "evil\n")
	now := r.Commit("evil")
	r.AnnotatedTag("v1.1.0")

	_, err = Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock, Refresh: true})
	accepted, err2 := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock, Refresh: true, Version: config.VersionPolicy{AcceptMovedTag: true}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR732")
	require.NoError(t, err2)
	assert.Equal(t, now, accepted.Commit)
}

func TestResolve_UnsatisfiableVersionIsAR730(t *testing.T) {
	r := versionRepo(t)
	spec := Spec{Name: "team", URL: "git+" + r.URL, Version: "^9", Path: "skills"}

	_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Refresh: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR730")
}

func TestResolve_FrozenRefusesAnUncoveredVersionSource(t *testing.T) {
	r := versionRepo(t)
	spec := Spec{Name: "team", URL: "git+" + r.URL, Version: "^1", Path: "skills"}

	_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Frozen: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not covered")
}

func TestResolve_PlainRefsKeepTheirOldBehaviour(t *testing.T) {
	r := versionRepo(t)
	spec := Spec{Name: "team", URL: "git+" + r.URL, Ref: "v1.0.0", Path: "skills"}

	res, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir()})

	require.NoError(t, err)
	assert.Equal(t, r.TagCommit("v1.0.0"), res.Commit)
	assert.Empty(t, res.Tag)
	assert.Equal(t, "v1.0.0", res.Entry().Ref)
}
