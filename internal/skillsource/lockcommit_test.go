package skillsource

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedThenMoved pins ref at the first commit, moves the remote on, and returns
// the lock and the pinned commit for use with an empty cache.
func lockedThenMoved(t *testing.T, f *fixture, ref string) (Spec, *lockfile.File, string, string) {
	t.Helper()
	spec := Spec{Name: "team", URL: "git+" + f.url, Ref: ref, Path: "skills"}
	res, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir()})
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, res.Entry())
	moved := f.advance("skills/pdf/references/forms.md", "forms v2\n")
	return spec, lock, res.Commit, moved
}

func TestResolve_LockCommitMustBeAFullHexSHA(t *testing.T) {
	f := newFixture(t)
	spec, lock, _, _ := lockedThenMoved(t, f, "v1.0.0")
	for _, bad := range []string{"../../../etc", "../x", "abc", "A123456789abcdef0123456789abcdef01234567"} {
		lock.Find(lockfile.KindSource, "team").Commit = bad
		_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Lock: lock})
		require.Error(t, err, bad)
		assert.ErrorIs(t, err, errLockCommit, bad)
	}
}

func TestCheckLock_InvalidLockCommitIsAProblem(t *testing.T) {
	f := newFixture(t)
	spec, lock, _, _ := lockedThenMoved(t, f, "v1.0.0")
	lock.Find(lockfile.KindSource, "team").Commit = "../../x"
	problems := CheckLock([]config.SkillSourceConfig{{Name: spec.Name, URL: spec.URL, Ref: spec.Ref, Path: spec.Path}}, lock, t.TempDir())
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "commit")
}
