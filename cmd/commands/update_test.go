package commands

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

type updateFixture struct {
	t    *testing.T
	repo *tagtest.Repo
	root string
}

func (f *updateFixture) release(body, tag string, annotated bool) string {
	f.repo.Write(".ai-rulez/rules/shared.md", "# Shared\n\n"+body+"\n")
	f.repo.Write(".ai-rulez/rules/extra-"+tag+".md", "# Extra "+tag+"\n")
	sha := f.repo.Commit(body)
	if annotated {
		f.repo.AnnotatedTag(tag)
	} else {
		f.repo.Tag(tag)
	}
	return sha
}

func resetUpdateFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		lockOutdated, lockFailOnOutdated, lockOffline, lockFormat, lockKind = false, false, false, "", ""
		updateDryRun, updateAllowDowngrade, updateAcceptMoved, updateKind, updateFormat, updateOffline = false, false, false, "", "", false
		updateMajor, updateWriteConfig, updateAcceptFindings = false, false, false
		// A fixture represents a fresh CLI invocation. In particular, generate's
		// RequireWhenEnforced writer policy must not leak into update's read phase.
		cliLockPolicy = config.LockPolicy{}
	}
	reset()
	t.Cleanup(reset)
}

func newUpdateFixture(t *testing.T, include string) *updateFixture {
	t.Helper()
	testutil.IsolateHome(t)
	resetUpdateFlags(t)
	resetLockViewFlags(t)
	repo := tagtest.New(t)
	f := &updateFixture{t: t, repo: repo}
	f.release("one", "v1.0.0", false)
	f.release("two", "v1.1.0", true)
	f.release("three", "v2.0.0", false)
	f.root = lockProject(t, "\n[[includes]]\nname = \"shared\"\nsource = \""+tomlEscape(repo.URL)+"\"\n"+include+"\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	return f
}

func (f *updateFixture) lock() *lockfile.File {
	f.t.Helper()
	lock, err := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
	require.NoError(f.t, err)
	return lock
}

func TestUpdate_LockResolvesAndKeepsThenUpdateMoves(t *testing.T) {
	f := newUpdateFixture(t, `version = "^1"`)
	first := f.lock().Find(lockfile.KindInclude, "shared")
	require.NotNil(t, first)
	assert.Equal(t, "v1.1.0", first.Tag)
	assert.Equal(t, "^1", first.Ref)
	f.release("one point two", "v1.2.0", false)

	// `lock` keeps a pin that still satisfies the constraint.
	require.Equal(t, 0, writeLockAt("", "", nil))
	assert.Equal(t, "v1.1.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)

	// --dry-run reports the move and writes nothing.
	updateDryRun, updateFormat = true, formatJSON
	var code int
	stdout := captureStdout(t, func() { code = reported(runUpdate(nil)) })
	require.Equal(t, 0, code)
	validateAgainst(t, "../../schema/update.schema.json", []byte(stdout))
	var dry updateReport
	require.NoError(t, json.Unmarshal([]byte(stdout), &dry), stdout)
	require.Len(t, dry.Updates, 1)
	assert.True(t, dry.DryRun)
	assert.Equal(t, "v1.1.0", dry.Updates[0].From.Tag)
	assert.Equal(t, "v1.2.0", dry.Updates[0].To.Tag)
	assert.Equal(t, []tagresolve.FileChange{{Path: "rules/extra-v1.2.0.md", Change: "A"}, {Path: "rules/extra-v2.0.0.md", Change: "A"}, {Path: "rules/shared.md", Change: "M"}}, dry.Updates[0].Files)
	assert.Equal(t, "v1.1.0", f.lock().Find(lockfile.KindInclude, "shared").Tag, "dry run writes nothing")

	// The real update writes the lock, deterministically.
	updateDryRun, updateFormat = false, ""
	_, _ = capture(t, func() { code = reported(runUpdate(nil)) })
	require.Equal(t, 0, code)
	moved := f.lock().Find(lockfile.KindInclude, "shared")
	assert.Equal(t, "v1.2.0", moved.Tag)
	assert.Equal(t, f.repo.TagCommit("v1.2.0"), moved.Commit)
	assert.NotEqual(t, first.Digest, moved.Digest)

	// Nothing left to do.
	stdout, _ = capture(t, func() { code = reported(runUpdate(nil)) })
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "up to date")
}

func TestUpdate_OutdatedReportsAndGates(t *testing.T) {
	f := newUpdateFixture(t, `version = "^1"`)
	f.release("one point two", "v1.2.0", false)

	lockOutdated, lockFormat = true, formatJSON
	var code int
	stdout := captureStdout(t, func() { code = reported(outdatedAt("", "", nil)) })

	assert.Equal(t, 0, code, "an allowed update does not fail by default")
	validateAgainst(t, "../../schema/lock-outdated.schema.json", []byte(stdout))
	var rep tagresolve.Report
	require.NoError(t, json.Unmarshal([]byte(stdout), &rep), stdout)
	require.Len(t, rep.Sources, 1)
	row := rep.Sources[0]
	assert.Equal(t, tagresolve.StatusUpdatable, row.Status)
	assert.Equal(t, "v1.1.0", row.Locked.Tag)
	assert.Equal(t, "v1.2.0", row.Allowed.Tag)
	assert.Equal(t, "v2.0.0", row.Latest.Tag)
	assert.True(t, row.MajorAvailable)

	lockFailOnOutdated = true
	_ = captureStdout(t, func() { code = reported(outdatedAt("", "", nil)) })
	assert.Equal(t, exitDrift, code, "--fail-on-outdated exits 2 on an allowed update")
}

func TestUpdate_AMovedTagIsRefusedUntilAccepted(t *testing.T) {
	f := newUpdateFixture(t, `version = "^1"`)
	was := f.lock().Find(lockfile.KindInclude, "shared").Commit
	evil := f.release("evil", "v1.1.0", true)
	require.NotEqual(t, was, evil)

	// outdated flags it and exits 2 even without --fail-on-outdated.
	lockOutdated, lockFormat = true, formatJSON
	var code int
	stdout := captureStdout(t, func() { code = reported(outdatedAt("", "", nil)) })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stdout, `"code": "AR732"`)
	lockOutdated, lockFormat = false, ""

	// update refuses and writes nothing.
	_, stderr := capture(t, func() { code = reported(runUpdate(nil)) })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "AR732")
	assert.Equal(t, was, f.lock().Find(lockfile.KindInclude, "shared").Commit)

	// `lock` refuses too.
	_, stderr = capture(t, func() { code = runLockFor("", nil) })
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "AR732")

	// --accept-moved-tag re-pins the tag at its new commit.
	updateAcceptMoved = true
	_, _ = capture(t, func() { code = reported(runUpdate(nil)) })
	require.Equal(t, 0, code)
	assert.Equal(t, evil, f.lock().Find(lockfile.KindInclude, "shared").Commit)
}

func TestUpdate_DowngradeNeedsAFlag(t *testing.T) {
	f := newUpdateFixture(t, `version = "^1"`)
	f.repo.DeleteTag("v1.1.0") // a truncated tag list: v1.0.0 is now the newest allowed tag

	var code int
	stdout, _ := capture(t, func() { code = reported(runUpdate(nil)) })
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "--allow-downgrade")
	assert.Equal(t, "v1.1.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)
}

func TestUpdate_UnsatisfiableConstraintIsRefused(t *testing.T) {
	f := newUpdateFixture(t, `version = "^1"`)
	f.writeVersion(t, `version = "^7"`)

	var code int
	_, stderr := capture(t, func() { code = reported(runUpdate(nil)) })

	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "AR730")
}

func (f *updateFixture) writeVersion(t *testing.T, include string) {
	t.Helper()
	writeFile(t, filepath.Join(f.root, ".ai-rulez", "config.toml"), lockProjectConfig+"\n[[includes]]\nname = \"shared\"\nsource = \""+tomlEscape(f.repo.URL)+"\"\n"+include+"\n")
}

func TestUpdate_UnknownNamesKindsAndOffline(t *testing.T) {
	newUpdateFixture(t, `version = "^1"`)
	tests := []struct {
		name  string
		setup func()
		args  []string
		want  string
	}{
		{"a name with no constraint", func() {}, []string{"nope"}, "not a remote include"},
		{"a bad kind", func() { updateKind = "bogus" }, nil, "unknown --kind"},
		{"offline", func() { updateOffline = true }, nil, "needs the network"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetUpdateFlags(t)
			tt.setup()
			var code int

			_, stderr := capture(t, func() { code = reported(runUpdate(tt.args)) })

			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, tt.want)
		})
	}
}

func TestLockOutdated_OfflineRefuses(t *testing.T) {
	newUpdateFixture(t, `version = "^1"`)
	lockOutdated, lockOffline = true, true

	var code int
	_, stderr := capture(t, func() { code = reported(outdatedAt("", "", nil)) })

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "lock --check")
}

func TestUpdate_AllowDowngradeMovesToTheLowerTag(t *testing.T) {
	f := newUpdateFixture(t, `version = "^1"`)
	f.repo.DeleteTag("v1.1.0")
	updateAllowDowngrade = true

	var code int
	_, _ = capture(t, func() { code = reported(runUpdate(nil)) })

	require.Equal(t, 0, code)
	assert.Equal(t, "v1.0.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)
}

func TestLockOutdated_UnknownNameIsAnError(t *testing.T) {
	newUpdateFixture(t, `version = "^1"`)
	lockOutdated, lockFormat = true, ""

	var code int
	_, stderr := capture(t, func() { code = reported(outdatedAt("", "", []string{"nope"})) })

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "not a remote include")
}

func TestUpdate_OutdatedReportsMarketplaceAndPluginKinds(t *testing.T) {
	// Arrange: a marketplace and a plugin, each a tagged repository pinned by a version constraint
	testutil.IsolateHome(t)
	resetUpdateFlags(t)
	resetLockViewFlags(t)
	plug := plugintest.NewRepo(t, plugintest.ClaudePlugin("tools"))
	plug.Tag("v1.0.0")
	market := plugintest.NewRepo(t, plugintest.ClaudeMarketplace("acme"))
	market.Tag("v1.0.0")
	lockProject(t, "\n[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+tomlEscape(market.URL())+"\"\nversion = \"^1\"\n"+
		"\n[[installed_plugins]]\nname = \"tools\"\nsource = \""+tomlEscape(plug.URL())+"\"\nversion = \"^1\"\ninclude = [\"skills\"]\n")
	require.Equal(t, 0, writeLockAt("", "", nil), "lock")
	plug.Commit(map[string]string{"README.md": "# one point one\n"}, "release")
	plug.Tag("v1.1.0")
	market.Commit(map[string]string{"README.md": "# one point one\n"}, "release")
	market.Tag("v1.1.0")
	lockOutdated, lockFormat = true, formatJSON

	// Act
	var code int
	stdout := captureStdout(t, func() { code = reported(outdatedAt("", "", nil)) })

	// Assert
	assert.Equal(t, 0, code)
	validateAgainst(t, "../../schema/lock-outdated.schema.json", []byte(stdout))
	var rep tagresolve.Report
	require.NoError(t, json.Unmarshal([]byte(stdout), &rep), stdout)
	kinds := map[string]string{}
	for _, row := range rep.Sources {
		kinds[row.Kind] = row.Name
	}
	assert.Equal(t, map[string]string{lockfile.KindMarketplace: "acme", lockfile.KindPlugin: "tools"}, kinds, stdout)
}
