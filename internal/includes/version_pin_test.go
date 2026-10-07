package includes

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
)

// versionFixture is a remote with tagged releases and a project that includes it
// by version range and installs one skill from it.
type versionFixture struct {
	repo    *tagtest.Repo
	project string
}

func (f *versionFixture) release(t *testing.T, body string, tag string, annotated bool) string {
	t.Helper()
	f.repo.Write(".ai-rulez/rules/shared.md", "# Shared\n\n"+body+"\n")
	f.repo.Write("skills/foo/SKILL.md", "---\nname: foo\ndescription: Use when foo.\n---\n"+body+"\n")
	sha := f.repo.Commit(body)
	if annotated {
		f.repo.AnnotatedTag(tag)
	} else {
		f.repo.Tag(tag)
	}
	return sha
}

func newVersionFixture(t *testing.T, includeLine, skillLine string) *versionFixture {
	t.Helper()
	isolateHome(t)
	resetPolicy(t)
	f := &versionFixture{repo: tagtest.New(t), project: t.TempDir()}
	f.release(t, "release one", "v1.0.0", false)
	f.release(t, "release two", "v1.1.0", true)
	f.release(t, "release three", "v2.0.0", false)
	f.writeConfig(t, includeLine, skillLine)
	return f
}

func (f *versionFixture) writeConfig(t *testing.T, includeLine, skillLine string) {
	t.Helper()
	writeTestFile(t, filepath.Join(f.project, ".ai-rulez", "config.toml"), `version = "4.0"
name = "p"
presets = ["claude"]
gitignore = false

[[includes]]
name = "shared"
source = "`+filepath.ToSlash(f.repo.URL)+`"
`+includeLine+`

[[installed_skills]]
name = "foo"
source = "`+filepath.ToSlash(f.repo.URL)+`"
`+skillLine+`
`)
}

func resetPolicy(t *testing.T) {
	t.Helper()
	reset := func() {
		lockPolicy.Mode, lockPolicy.Refresh, lockPolicy.Offline = LockAuto, nil, false
		Advance, AllowDowngrade, AcceptMovedTag = nil, false, false
		ResetObserved()
	}
	reset()
	t.Cleanup(reset)
}

func (f *versionFixture) load(t *testing.T) (*config.Config, error) {
	t.Helper()
	return loadWithResolvers(context.Background(), f.project, config.WithoutLocal())
}

// refresh runs `ai-rulez lock` (lockPolicy.Mode refresh) and returns the lock and the problems.
func (f *versionFixture) refresh(t *testing.T, current *lockfile.File) (*lockfile.File, []string) {
	t.Helper()
	lockPolicy.Mode = LockRefresh
	lockPolicy.Refresh = nil
	ResetObserved()
	defer func() { lockPolicy.Mode = LockAuto }()
	cfg, err := f.load(t)
	require.NoError(t, err)
	return BuildLock(cfg, current)
}

func TestVersionPin_LockResolvesTheNewestAllowedTag(t *testing.T) {
	// Arrange
	f := newVersionFixture(t, `version = "^1"`, `version = "~1.0.0"`)

	// Act
	lock, problems := f.refresh(t, nil)

	// Assert
	require.Empty(t, problems)
	inc := lock.Find(lockfile.KindInclude, "shared")
	require.NotNil(t, inc)
	assert.Equal(t, "^1", inc.Ref, "the lock records the constraint as the requested ref")
	assert.Equal(t, "v1.1.0", inc.Tag)
	assert.Equal(t, f.repo.TagCommit("v1.1.0"), inc.Commit, "the pin is the peeled commit")
	assert.Equal(t, f.repo.TagObject("v1.1.0"), inc.TagObject, "an annotated tag keeps its object id")
	skill := lock.Find(lockfile.KindSkill, "foo")
	require.NotNil(t, skill)
	assert.Equal(t, "v1.0.0", skill.Tag)
	assert.Empty(t, skill.TagObject, "a lightweight tag has no tag object")
}

func TestVersionPin_GenerateUsesThePinNotTheTags(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `version = "~1.0.0"`)
	lock := f.refresh2(t)
	require.NotNil(t, lock)
	f.release(t, "release 1.9", "v1.9.0", false) // a newer allowed tag appears
	lockPolicy.Mode = LockRequire

	cfg, err := f.load(t)

	require.NoError(t, err)
	assert.Contains(t, ruleBody(cfg), "release two", "generate never moves a pin, even to a newer allowed tag")
}

// refresh2 refreshes and saves the lock.
func (f *versionFixture) refresh2(t *testing.T) *lockfile.File {
	t.Helper()
	lock, problems := f.refresh(t, nil)
	require.Empty(t, problems)
	require.NoError(t, lockfile.Save(filepath.Join(f.project, ".ai-rulez"), lock))
	return lock
}

func TestVersionPin_LockKeepsAPinThatStillSatisfiesAndUpdateAdvancesIt(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `version = "~1.0.0"`)
	first := f.refresh2(t)
	f.release(t, "release 1.2", "v1.2.0", true)

	kept, problems := f.refresh(t, first)
	require.Empty(t, problems)
	Advance = func(kind, name string) bool { return kind == lockfile.KindInclude }
	moved, problems2 := f.refresh(t, first)

	require.Empty(t, problems2)
	assert.Equal(t, "v1.1.0", kept.Find(lockfile.KindInclude, "shared").Tag, "lock never upgrades a satisfied pin")
	assert.Equal(t, "v1.2.0", moved.Find(lockfile.KindInclude, "shared").Tag, "update moves it")
	assert.Equal(t, "v1.0.0", moved.Find(lockfile.KindSkill, "foo").Tag, "only the advanced source moves")
}

func TestVersionPin_AMovedTagIsAnErrorUnlessAccepted(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `version = "~1.0.0"`)
	first := f.refresh2(t)
	was := first.Find(lockfile.KindInclude, "shared").Commit
	now := f.release(t, "evil", "v1.1.0", true) // force-pushed to a new commit
	require.NotEqual(t, was, now)

	_, problems := f.refresh(t, first)
	AcceptMovedTag = true
	accepted, problems2 := f.refresh(t, first)

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "AR732")
	require.Empty(t, problems2)
	assert.Equal(t, now, accepted.Find(lockfile.KindInclude, "shared").Commit)
	assert.Equal(t, "v1.1.0", accepted.Find(lockfile.KindInclude, "shared").Tag)
}

func TestVersionPin_ADeletedTagKeepsThePinnedCommit(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `version = "~1.0.0"`)
	first := f.refresh2(t)
	f.repo.DeleteTag("v1.1.0")

	kept, problems := f.refresh(t, first)

	require.Empty(t, problems)
	assert.Equal(t, first.Find(lockfile.KindInclude, "shared").Commit, kept.Find(lockfile.KindInclude, "shared").Commit)
	assert.Equal(t, "v1.1.0", kept.Find(lockfile.KindInclude, "shared").Tag)
}

func TestVersionPin_UnsatisfiableAndInvalidConstraintsNameTheirCode(t *testing.T) {
	tests := []struct {
		name, line, code string
	}{
		{"no tag satisfies", `version = "^3"`, "AR730"},
		{"prerelease only", `version = "^1.2.0-rc.1"`, "AR730"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newVersionFixture(t, tt.line, `ref = "main"`)

			_, problems := f.refresh(t, nil)

			require.NotEmpty(t, problems)
			assert.Contains(t, problems[0], tt.code)
		})
	}
}

func TestVersionPin_DowngradeIsRefusedUnlessAllowed(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	first := f.refresh2(t)
	f.repo.DeleteTag("v1.1.0") // a truncated tag list: the highest allowed tag is now v1.0.0
	Advance = func(string, string) bool { return true }

	_, problems := f.refresh(t, first)
	AllowDowngrade = true
	down, problems2 := f.refresh(t, first)

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "refusing to move")
	require.Empty(t, problems2)
	assert.Equal(t, "v1.0.0", down.Find(lockfile.KindInclude, "shared").Tag)
}

func TestVersionPin_EditingTheConstraintInvalidatesThePin(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	f.refresh2(t)
	f.writeConfig(t, `version = "^2"`, `ref = "main"`)
	lockPolicy.Mode = LockRequire

	_, err := f.load(t)

	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrLockViolation)
	assert.Contains(t, err.Error(), "stale")
}

func TestVersionPin_PrefixedTagsAndPrereleases(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"
tag_prefix = "pkg/v"`, `ref = "main"`)
	f.release(t, "pkg one", "pkg/v1.4.0", false)
	f.release(t, "pkg rc", "pkg/v1.5.0-rc.1", false)

	lock, problems := f.refresh(t, nil)

	require.Empty(t, problems)
	assert.Equal(t, "pkg/v1.4.0", lock.Find(lockfile.KindInclude, "shared").Tag, "prerelease skipped, plain tags ignored")

	f.writeConfig(t, `version = "^1"
tag_prefix = "pkg/v"
include_prerelease = true`, `ref = "main"`)
	lock, problems = f.refresh(t, nil)
	require.Empty(t, problems)
	assert.Equal(t, "pkg/v1.5.0-rc.1", lock.Find(lockfile.KindInclude, "shared").Tag)
}

func TestVersionPin_PlainRefsTakeTheOldPath(t *testing.T) {
	f := newVersionFixture(t, `ref = "main"`, `ref = "`+"v1.0.0"+`"`)

	lock, problems := f.refresh(t, nil)

	require.Empty(t, problems)
	inc := lock.Find(lockfile.KindInclude, "shared")
	assert.Equal(t, "main", inc.Ref)
	assert.Empty(t, inc.Tag, "a plain ref records no tag")
	assert.Equal(t, f.repo.Head(), inc.Commit)
}

func TestVersionPin_NeedsTheNetworkToResolve(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	lockPolicy.Offline = true
	lockPolicy.Mode = LockRefresh
	ResetObserved()

	cfg, err := f.load(t)
	require.NoError(t, err)
	_, problems := BuildLock(cfg, nil)

	require.NotEmpty(t, problems)
	assert.Contains(t, problems[0], "offline")
}
