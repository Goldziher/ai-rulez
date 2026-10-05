package skillsource

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func skillMD(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
}

// fixture is a work repo and a bare clone of it that tests use as a remote.
type fixture struct {
	t    *testing.T
	work string
	bare string
	url  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	work := t.TempDir()
	git(t, work, "init", "--quiet", "--initial-branch=main")
	write(t, work, "skills/pdf/SKILL.md", skillMD("pdf", "Work with PDF files"))
	write(t, work, "skills/pdf/references/forms.md", "forms v1\n")
	write(t, work, "skills/sql/SKILL.md", skillMD("sql", "Write SQL queries"))
	write(t, work, "skills/wip-draft/SKILL.md", skillMD("wip-draft", "Draft"))
	write(t, work, "README.md", "not a skill\n")
	git(t, work, "add", "-A")
	git(t, work, "commit", "--quiet", "-m", "v1")
	git(t, work, "tag", "-a", "v1.0.0", "-m", "v1.0.0")
	bare := filepath.Join(t.TempDir(), "remote.git")
	git(t, "", "clone", "--quiet", "--bare", work, bare)
	return &fixture{t: t, work: work, bare: bare, url: "file://" + bare}
}

// advance adds a commit, moves the v1.0.0 tag and the main branch to it, and pushes to the bare remote.
func (f *fixture) advance(rel, content string) string {
	write(f.t, f.work, rel, content)
	git(f.t, f.work, "add", "-A")
	git(f.t, f.work, "commit", "--quiet", "-m", "next")
	git(f.t, f.work, "tag", "-f", "-a", "v1.0.0", "-m", "moved")
	git(f.t, f.work, "push", "--quiet", "--force", f.bare, "main", "refs/tags/v1.0.0")
	return git(f.t, f.work, "rev-parse", "HEAD")
}

func (f *fixture) firstCommit() string {
	return git(f.t, f.work, "rev-list", "--max-parents=0", "HEAD")
}

func names(r *Resolved) []string {
	var out []string
	for _, s := range r.Skills {
		out = append(out, s.Name)
	}
	return out
}

func TestParseArg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		arg  string
		want Spec
	}{
		{"git+https://github.com/org/skills@v1.2.0#skills/", Spec{Name: "cli-skills", URL: "git+https://github.com/org/skills", Ref: "v1.2.0", Path: "skills"}},
		{"https://github.com/org/repo.git", Spec{Name: "cli-repo", URL: "https://github.com/org/repo.git"}},
		{"git@github.com:org/repo.git@v2", Spec{Name: "cli-repo", URL: "git@github.com:org/repo.git", Ref: "v2"}},
		{"git@github.com:org/repo.git", Spec{Name: "cli-repo", URL: "git@github.com:org/repo.git"}},
		{"https://user@host.example/org/My_Repo", Spec{Name: "cli-my_repo", URL: "https://user@host.example/org/My_Repo"}},
		{"./vendor/skills#team", Spec{Name: "cli-skills", URL: "./vendor/skills", Path: "team"}},
	}
	for _, tt := range tests {
		got, err := ParseArg(tt.arg)
		require.NoError(t, err, tt.arg)
		assert.Equal(t, tt.want, got, tt.arg)
	}
	_, err := ParseArg("git+https://h/r@v1#../x")
	require.Error(t, err)
	_, err = ParseArg(" ")
	require.Error(t, err)
}

func TestResolve_TagIsResolvedToACommitAndStaysPinnedByTheLock(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: "git+" + f.url, Ref: "v1.0.0", Path: "skills"}
	first := f.firstCommit()

	res, err := Resolve(context.Background(), spec, Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, first, res.Commit, "an annotated tag is peeled to the commit it points at")
	assert.Equal(t, []string{"pdf", "sql", "wip-draft"}, names(res))
	assert.False(t, res.Pinned, "without a lock a tag is not pinned (AR010)")
	assert.Equal(t, "tag", res.RefKind)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, res.Digest)

	// Record it the way `ai-rulez lock` does.
	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, res.Entry())
	assert.Equal(t, lockfile.Entry{Name: "team", Source: "file://" + f.bare, Path: "skills", Ref: "v1.0.0", Commit: first, Digest: res.Digest}, res.Entry())

	// The remote now moves the tag. Without the lock the new commit wins, with it the pin holds.
	moved := f.advance("skills/pdf/references/forms.md", "forms v2\n")
	unlocked, err := Resolve(context.Background(), spec, Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, moved, unlocked.Commit)
	assert.NotEqual(t, res.Digest, unlocked.Digest)

	locked, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock})
	require.NoError(t, err)
	assert.Equal(t, first, locked.Commit, "the lock keeps serving the pinned commit after the tag moved")
	assert.Equal(t, res.Digest, locked.Digest)
	assert.True(t, locked.Pinned)
	assert.True(t, locked.Locked)

	refreshed, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock, Refresh: true})
	require.NoError(t, err)
	assert.Equal(t, moved, refreshed.Commit, "`ai-rulez lock` re-resolves the ref")
}

func TestResolve_BranchIsUnpinnedAndSHAIsPinned(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	branch, err := Resolve(context.Background(), Spec{Name: "b", URL: f.url, Ref: "main", Path: "skills"}, Options{CacheDir: cache})
	require.NoError(t, err)
	assert.False(t, branch.Pinned)
	assert.Equal(t, "branch", branch.RefKind)

	head, err := Resolve(context.Background(), Spec{Name: "h", URL: f.url, Path: "skills"}, Options{CacheDir: cache})
	require.NoError(t, err)
	assert.False(t, head.Pinned)
	assert.Equal(t, "head", head.RefKind)

	sha := f.firstCommit()
	bySHA, err := Resolve(context.Background(), Spec{Name: "s", URL: f.url, Ref: sha, Path: "skills"}, Options{CacheDir: cache})
	require.NoError(t, err)
	assert.True(t, bySHA.Pinned, "a full commit SHA is pinned by itself")
	assert.Equal(t, sha, bySHA.Commit)
	assert.Equal(t, "commit", bySHA.RefKind)
	assert.Equal(t, []string{"pdf", "sql", "wip-draft"}, names(bySHA))

	_, err = Resolve(context.Background(), Spec{Name: "x", URL: f.url, Ref: "nope"}, Options{CacheDir: cache})
	require.ErrorContains(t, err, "not found on the remote")
}

func TestResolve_FrozenNeverTouchesTheNetwork(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: f.url, Ref: "v1.0.0", Path: "skills"}

	_, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Frozen: true})
	require.ErrorIs(t, err, config.ErrLockViolation, "frozen without a lock refuses")

	online, err := Resolve(context.Background(), spec, Options{CacheDir: cache})
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, online.Entry())

	// Make any network use fail: the remote disappears.
	require.NoError(t, os.RemoveAll(f.bare))
	frozen, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock, Frozen: true})
	require.NoError(t, err, "frozen serves from the lock and the cache without the remote")
	assert.Equal(t, online.Commit, frozen.Commit)
	assert.Equal(t, online.Digest, frozen.Digest)

	// A stale lock (ref changed in config) is refused rather than re-resolved.
	stale := spec
	stale.Ref = "v2.0.0"
	_, err = Resolve(context.Background(), stale, Options{CacheDir: cache, Lock: lock, Frozen: true})
	require.ErrorIs(t, err, config.ErrLockViolation)

	// An empty cache cannot be filled while frozen.
	_, err = Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Lock: lock, Frozen: true})
	require.ErrorContains(t, err, "not cached")
}

func TestResolve_OfflineUsesTheRefRecordedOnline(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: f.url, Ref: "v1.0.0", Path: "skills"}
	_, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Offline: true})
	require.ErrorContains(t, err, "never resolved")

	online, err := Resolve(context.Background(), spec, Options{CacheDir: cache})
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(f.bare))
	offline, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Offline: true})
	require.NoError(t, err)
	assert.Equal(t, online.Commit, offline.Commit)
}

func TestResolve_TamperedCacheIsRepairedOnlineAndRefusedOffline(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: f.url, Ref: "v1.0.0", Path: "skills"}
	online, err := Resolve(context.Background(), spec, Options{CacheDir: cache})
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, online.Entry())

	require.NoError(t, os.WriteFile(filepath.Join(online.Dir, "pdf", "SKILL.md"), []byte("---\nname: pdf\ndescription: evil\n---\n"), 0o644))
	_, err = Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock, Offline: true})
	require.ErrorIs(t, err, config.ErrLockViolation, "a tampered cache fails closed offline")

	repaired, err := Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock})
	require.NoError(t, err, "online, the pinned commit is fetched again")
	assert.Equal(t, online.Digest, repaired.Digest)
}

func TestResolve_LockedDigestMismatchFailsClosed(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: f.url, Ref: "v1.0.0", Path: "skills"}
	online, err := Resolve(context.Background(), spec, Options{CacheDir: cache})
	require.NoError(t, err)
	entry := online.Entry()
	entry.Digest = "sha256:" + strings.Repeat("0", 64)
	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, entry)
	_, err = Resolve(context.Background(), spec, Options{CacheDir: cache, Lock: lock})
	require.ErrorIs(t, err, config.ErrLockViolation, "the remote serves the pinned commit, but its bytes are not the ones locked")
}

func TestDiscover_IncludeExcludePrefixAndSymlinks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pdf/SKILL.md", skillMD("pdf", "PDF"))
	write(t, root, "pdf/references/a.md", "a")
	write(t, root, "sql/SKILL.md", skillMD("sql", "SQL"))
	write(t, root, "sql-wip/SKILL.md", skillMD("sql-wip", "WIP"))
	write(t, root, "notskill/readme.md", "x")
	outside := t.TempDir()
	write(t, outside, "secret.txt", "secret")
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "pdf", "leak.txt")))

	all, err := Discover(Spec{Name: "s"}, root)
	require.NoError(t, err)
	var got []string
	for _, s := range all {
		got = append(got, s.Name)
	}
	assert.Equal(t, []string{"pdf", "sql", "sql-wip"}, got)

	only, err := Discover(Spec{Name: "s", Include: []string{"sql*"}, Exclude: []string{"*-wip"}, NamePrefix: "team-"}, root)
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, "team-sql", only[0].Name)
	assert.Equal(t, "sql", only[0].Dir)
	assert.Contains(t, string(only[0].Files[0].Content), "name: team-sql")
	assert.NotContains(t, string(only[0].Files[0].Content), "name: sql\n")

	pdf, err := Discover(Spec{Name: "s", Include: []string{"pdf"}}, root)
	require.NoError(t, err)
	require.Len(t, pdf, 1)
	var paths []string
	for _, f := range pdf[0].Files {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{"SKILL.md", "references/a.md"}, paths, "the symlink is not followed")
}

func TestResolve_LocalDirectory(t *testing.T) {
	root := t.TempDir()
	write(t, root, "team/pdf/SKILL.md", skillMD("pdf", "PDF"))
	spec := Spec{Name: "local", URL: root, Path: "team"}
	res, err := Resolve(context.Background(), spec, Options{Frozen: true})
	require.NoError(t, err, "a local directory needs no network and no pin")
	assert.Equal(t, []string{"pdf"}, names(res))
	assert.Empty(t, res.Commit)
	assert.False(t, res.Pinned)

	lock := &lockfile.File{Version: lockfile.Version}
	lock.Set(lockfile.KindSource, res.Entry())
	write(t, root, "team/pdf/references/new.md", "changed")
	_, err = Resolve(context.Background(), spec, Options{Lock: lock})
	require.ErrorIs(t, err, config.ErrLockViolation, "a locked local directory that changed is refused")

	_, err = Resolve(context.Background(), Spec{Name: "x", URL: filepath.Join(root, "missing")}, Options{})
	require.Error(t, err)
}

func TestCheckLock(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	src := config.SkillSourceConfig{Name: "team", URL: f.url, Ref: "v1.0.0", Path: "skills"}
	res, err := Resolve(context.Background(), FromConfig(&src), Options{CacheDir: cache})
	require.NoError(t, err)

	assert.NotEmpty(t, CheckLock([]config.SkillSourceConfig{src}, nil, cache), "no lock")
	lock := &lockfile.File{Version: lockfile.Version}
	assert.Equal(t, []Problem{{"team", "not covered by the lock"}}, CheckLock([]config.SkillSourceConfig{src}, lock, cache))

	lock.Set(lockfile.KindSource, res.Entry())
	assert.Empty(t, CheckLock([]config.SkillSourceConfig{src}, lock, cache))

	changed := src
	changed.Ref = "v2"
	assert.Contains(t, CheckLock([]config.SkillSourceConfig{changed}, lock, cache)[0].Message, "stale")
	assert.Contains(t, CheckLock(nil, lock, cache)[0].Message, "no longer configured")

	require.NoError(t, os.WriteFile(filepath.Join(res.Dir, "sql", "SKILL.md"), []byte("tampered"), 0o644))
	assert.Contains(t, CheckLock([]config.SkillSourceConfig{src}, lock, cache)[0].Message, "cached content digest")
}
