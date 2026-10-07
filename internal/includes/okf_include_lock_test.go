package includes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// okfLockFixture is a git repository holding an OKF bundle (tag v1 on its first
// commit) and a project that includes it with format = "okf".
type okfLockFixture struct {
	remote, project string
}

func newOKFLockFixture(t *testing.T, ref string) *okfLockFixture {
	t.Helper()
	isolateHome(t)
	lockPolicy.Mode, lockPolicy.Refresh, lockPolicy.Offline = LockAuto, nil, false
	OKFScan = nil
	ResetObserved()
	t.Cleanup(func() { lockPolicy.Mode, lockPolicy.Refresh, lockPolicy.Offline, OKFScan = LockAuto, nil, false, nil })

	remote := t.TempDir()
	git(t, remote, "init", "-q", "-b", "main")
	writeTestFile(t, filepath.Join(remote, "kb", "decisions", "shared.md"), "---\ntype: Decision\ndescription: Shared\n---\nversion one\n")
	git(t, remote, "add", "-A")
	git(t, remote, "commit", "-qm", "one")
	git(t, remote, "tag", "v1")

	project := t.TempDir()
	refLine := ""
	if ref != "" {
		refLine = "ref = \"" + ref + "\"\n"
	}
	writeTestFile(t, filepath.Join(project, ".ai-rulez", "config.toml"), `version = "5.0"
name = "p"
presets = ["claude"]
gitignore = false

[[includes]]
name = "kb"
source = "file://`+filepath.ToSlash(remote)+`"
path = "kb"
format = "okf"
`+refLine)
	return &okfLockFixture{remote: remote, project: project}
}

func (f *okfLockFixture) load(t *testing.T) (*config.Config, error) {
	t.Helper()
	return loadWithResolvers(context.Background(), f.project, config.WithoutLocal())
}

func (f *okfLockFixture) body(cfg *config.Config) string {
	for _, r := range cfg.Content.Rules {
		if strings.HasSuffix(r.Name, "shared") {
			return r.Content
		}
	}
	return ""
}

// advance commits a second version of the bundle and moves tag v1 onto it.
func (f *okfLockFixture) advance(t *testing.T) string {
	t.Helper()
	writeTestFile(t, filepath.Join(f.remote, "kb", "decisions", "shared.md"), "---\ntype: Decision\ndescription: Shared\n---\nversion two\n")
	git(t, f.remote, "commit", "-qam", "two")
	git(t, f.remote, "tag", "-f", "v1")
	return git(t, f.remote, "rev-parse", "HEAD")
}

func (f *okfLockFixture) writeLock(t *testing.T) *lockfile.File {
	t.Helper()
	lockPolicy.Mode = LockRefresh
	ResetObserved()
	cfg, err := f.load(t)
	require.NoError(t, err)
	lockPolicy.Mode = LockAuto
	next, problems := BuildLock(cfg, nil)
	require.Empty(t, problems)
	require.NoError(t, lockfile.Save(cfg.ConfigDir, next))
	return next
}

func TestOKFInclude_TagIsPinnedToACommitAndDigest(t *testing.T) {
	f := newOKFLockFixture(t, "v1")
	first := git(t, f.remote, "rev-parse", "v1^{commit}")
	lock := f.writeLock(t)
	require.Len(t, lock.Include, 1)
	entry := lock.Include[0]
	assert.Equal(t, "kb", entry.Name)
	assert.Equal(t, "v1", entry.Ref)
	assert.Equal(t, "kb", entry.Path)
	assert.Equal(t, first, entry.Commit, "the tag resolves to the commit it points at")
	assert.True(t, strings.HasPrefix(entry.Digest, "sha256:"))
}

func TestOKFInclude_MovedTagKeepsThePinUntilRelocked(t *testing.T) {
	f := newOKFLockFixture(t, "v1")
	before := f.writeLock(t)
	second := f.advance(t)

	lockPolicy.Mode = LockRequire
	cfg, err := f.load(t)
	require.NoError(t, err)
	assert.Contains(t, f.body(cfg), "version one", "the locked commit is used, not the moved tag")

	problems, _ := CheckLock(cfg, before)
	assert.Empty(t, problems, "the lock still matches the pinned commit")

	// Relocking follows the moved tag and records the new commit and digest.
	after := f.writeLock(t)
	assert.Equal(t, second, after.Include[0].Commit)
	assert.NotEqual(t, before.Include[0].Commit, after.Include[0].Commit)
	assert.NotEqual(t, before.Include[0].Digest, after.Include[0].Digest)
}

func TestOKFInclude_LockCheckReportsStaleAndTamperedEntries(t *testing.T) {
	f := newOKFLockFixture(t, "v1")
	cfg, err := loadWithResolvers(context.Background(), f.project, config.WithoutLocal(), config.WithoutRemote())
	require.NoError(t, err)
	problems, _ := CheckLock(cfg, nil)
	require.Len(t, problems, 1, "an OKF include from git is lockable like any other")
	assert.Equal(t, "kb", problems[0].Name)

	f.writeLock(t)
	lock, err := lockfile.Load(cfg.ConfigDir)
	require.NoError(t, err)
	problems, cached := CheckLock(cfg, lock)
	assert.Empty(t, problems)
	assert.Equal(t, 1, cached, "the cached bundle is digest-verified")

	cachedFile := okfCachedFile(t, f)
	require.FileExists(t, cachedFile)
	require.NoError(t, os.WriteFile(cachedFile, []byte("---\ntype: Decision\n---\ntampered\n"), 0o644))
	problems, _ = CheckLock(cfg, lock)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "digest")
}

func TestOKFInclude_FrozenNeverFetches(t *testing.T) {
	f := newOKFLockFixture(t, "v1")
	f.writeLock(t)
	require.NoError(t, os.RemoveAll(f.remote), "the remote is gone: any fetch would fail")

	lockPolicy.Mode, lockPolicy.Offline = LockFrozen, true
	cfg, err := f.load(t)
	require.NoError(t, err)
	assert.Contains(t, f.body(cfg), "version one")

	// A cold cache cannot be filled while frozen.
	isolateHome(t)
	_, err = f.load(t)
	require.Error(t, err, "a failing include is an error while frozen")
	assert.Contains(t, err.Error(), "not in the local cache", "nothing was fetched to fill the cold cache")
}

func TestOKFInclude_FrozenRejectsATamperedCache(t *testing.T) {
	f := newOKFLockFixture(t, "v1")
	f.writeLock(t)
	cachedFile := okfCachedFile(t, f)
	require.NoError(t, os.WriteFile(cachedFile, []byte("---\ntype: Decision\n---\ntampered\n"), 0o644))

	lockPolicy.Mode, lockPolicy.Offline = LockFrozen, true
	_, err := f.load(t)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrLockViolation)
}

func TestOKFInclude_RequireFailsWhenNotLocked(t *testing.T) {
	f := newOKFLockFixture(t, "v1")
	lockPolicy.Mode = LockRequire
	_, err := f.load(t)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrLockViolation)
}

func TestOKFInclude_MovingRefIsUnpinnedUntilLocked(t *testing.T) {
	for name, ref := range map[string]string{"branch": "main", "tag": "v1", "none": ""} {
		t.Run(name, func(t *testing.T) {
			f := newOKFLockFixture(t, ref)
			cfg, err := loadWithResolvers(context.Background(), f.project, config.WithoutLocal(), config.WithoutRemote())
			require.NoError(t, err)
			unpinned := Unpinned(cfg)
			require.Len(t, unpinned, 1)
			assert.Equal(t, "kb", unpinned[0].Name)

			f.writeLock(t)
			assert.Empty(t, Unpinned(cfg), "a lock entry pins it")
		})
	}
}

func TestOKFInclude_FullCommitIsPinnedWithoutALock(t *testing.T) {
	f := newOKFLockFixture(t, "")
	sha := git(t, f.remote, "rev-parse", "HEAD")
	f = &okfLockFixture{remote: f.remote, project: f.project}
	cfgPath := filepath.Join(f.project, ".ai-rulez", "config.toml")
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(raw, []byte("ref = \""+sha+"\"\n")...), 0o644))
	cfg, err := loadWithResolvers(context.Background(), f.project, config.WithoutLocal(), config.WithoutRemote())
	require.NoError(t, err)
	assert.Empty(t, Unpinned(cfg))
}

func TestOKFInclude_SecurityFindingRefusesTheBundle(t *testing.T) {
	f := newOKFLockFixture(t, "v1")
	OKFScan = func(map[string]string) []okfbridge.SecurityFinding {
		return []okfbridge.SecurityFinding{{Code: "AR001", Severity: okfbridge.SeverityError, File: "decisions/shared.md", Message: "secret"}}
	}
	_, err := f.load(t)
	require.Error(t, err, "a refused include stops the load")
	assert.Contains(t, err.Error(), "kb")
}

func TestOKFInclude_GitRunsHardenedAndScrubbed(t *testing.T) {
	fake := &runner.Fake{}
	ctx := runner.WithContext(context.Background(), fake)
	gitRun(withHardenedGit(ctx), "/tmp/x", nil, "fetch")
	gitRun(ctx, "", nil, "fetch")
	calls := fake.Calls()
	require.Len(t, calls, 2)
	joined := strings.Join(calls[0].Argv, " ")
	assert.Contains(t, joined, "core.hooksPath=/dev/null")
	assert.Contains(t, joined, "protocol.allow=never")
	assert.Contains(t, joined, "-C /tmp/x")
	assert.NotContains(t, strings.Join(calls[1].Argv, " "), "core.hooksPath")

	t.Setenv("GIT_DIR", "/elsewhere")
	for _, kv := range gitEnvFor(withHardenedGit(context.Background())) {
		assert.False(t, strings.HasPrefix(kv, "GIT_DIR="), "a hook's GIT_DIR must not reach the fetch")
	}
}

// okfCachedFile is the cached copy of the fixture's shared document.
func okfCachedFile(t *testing.T, f *okfLockFixture) string {
	t.Helper()
	dir, err := getIncludeCacheDir(ambient.Host{}, "kb", "file://"+filepath.ToSlash(f.remote))
	require.NoError(t, err)
	return filepath.Join(dir, "kb", "decisions", "shared.md")
}
