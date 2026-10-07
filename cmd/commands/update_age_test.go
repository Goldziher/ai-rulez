package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
)

const longAgo = "2020-01-01T00:00:00Z"

func resetAgeFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		updateMajor, updateWriteConfig, updateAcceptFindings = false, false, false
		includes.ReleaseGate = nil
	}
	reset()
	t.Cleanup(reset)
}

// ageFixture is a remote with two releases from 2020 and one made now, and a
// project that includes it by version range.
type ageFixture struct {
	t    *testing.T
	repo *tagtest.Repo
	root string
}

func (f *ageFixture) release(body, tag string, annotated bool) string {
	f.repo.Write(".ai-rulez/rules/shared.md", "# Shared\n\n"+body+"\n")
	sha := f.repo.Commit(body)
	if annotated {
		f.repo.AnnotatedTag(tag)
	} else {
		f.repo.Tag(tag)
	}
	return sha
}

// newAgeFixture builds the remote and writes a config made of lockTable (the
// [lock] and [lint] tables) and one include with includeLines; it does not lock.
func newAgeFixture(t *testing.T, lockTable, includeLines string) *ageFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	resetUpdateFlags(t)
	resetLockViewFlags(t)
	resetAgeFlags(t)
	repo := tagtest.New(t)
	f := &ageFixture{t: t, repo: repo}
	repo.Date = longAgo
	f.release("one", "v1.0.0", false)
	f.release("two", "v1.1.0", true)
	repo.Date = ""
	f.release("three, just released", "v1.2.0", false)
	f.root = lockProject(t, "\n"+lockTable+"\n[[includes]]\nname = \"shared\"\nsource = \""+repo.URL+"\"\n"+includeLines+"\n")
	return f
}

func (f *ageFixture) lock() *lockfile.File {
	f.t.Helper()
	lock, err := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
	require.NoError(f.t, err)
	return lock
}

func (f *ageFixture) config() string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, ".ai-rulez", "config.toml"))
	require.NoError(f.t, err)
	return string(data)
}

func (f *ageFixture) setConfig(s string) {
	f.t.Helper()
	require.NoError(f.t, os.WriteFile(filepath.Join(f.root, ".ai-rulez", "config.toml"), []byte(s), 0o644))
}

const commitAge30 = "[lock]\nmin_release_age = \"30d\"\nmin_release_age_source = \"commit\"\n"

func TestMinReleaseAge_LockPinsTheNewestTagThatIsOldEnough(t *testing.T) {
	// Arrange: v1.2.0 was released a moment ago, v1.1.0 in 2020.
	f := newAgeFixture(t, commitAge30, `version = "^1"`)

	// Act
	require.Equal(t, 0, writeLockAt("", "", nil))

	// Assert
	e := f.lock().Find(lockfile.KindInclude, "shared")
	require.NotNil(t, e)
	assert.Equal(t, "v1.1.0", e.Tag, "v1.2.0 is held back (AR733)")
	assert.Equal(t, "commit", e.ReleasedFrom)
	released, err := time.Parse(time.RFC3339, e.Released)
	require.NoError(t, err)
	assert.Equal(t, 2020, released.Year())
}

func TestMinReleaseAge_OutdatedAndUpdateReportTheHeldTag(t *testing.T) {
	f := newAgeFixture(t, commitAge30, `version = "^1"`)
	require.Equal(t, 0, writeLockAt("", "", nil))

	// lock --outdated lists the held tag, and the report validates.
	lockOutdated, lockFormat = true, formatJSON
	var code int
	stdout := captureStdout(t, func() { code = outdatedAt("", "", nil) })
	require.Equal(t, 0, code)
	validateAgainst(t, "../../schema/lock-outdated.schema.json", []byte(stdout))
	var rep tagresolve.Report
	require.NoError(t, json.Unmarshal([]byte(stdout), &rep), stdout)
	row := rep.Sources[0]
	assert.Equal(t, tagresolve.StatusUpToDate, row.Status, "the only newer tag is held back")
	assert.Equal(t, "v1.1.0", row.Allowed.Tag)
	assert.Equal(t, "v1.2.0", row.Latest.Tag)
	require.Len(t, row.Held, 1)
	assert.Equal(t, "v1.2.0", row.Held[0].Tag)
	assert.Equal(t, 1, rep.Summary.HeldBack)
	lockFormat = ""
	text := captureStdout(t, func() { _ = outdatedAt("", "", nil) })
	assert.Contains(t, text, "AR733 v1.2.0 held back")

	// update says so and moves nothing.
	updateFormat = formatJSON
	stdout = captureStdout(t, func() { code = runUpdate(nil) })
	require.Equal(t, 0, code)
	validateAgainst(t, "../../schema/update.schema.json", []byte(stdout))
	assert.Equal(t, "v1.1.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)
	updateFormat = ""
	text, _ = capture(t, func() { code = runUpdate(nil) })
	assert.Equal(t, 0, code)
	assert.Contains(t, text, "AR733 v1.2.0 held back")
}

func TestMinReleaseAge_PerSourceValueOverridesTheLockDefault(t *testing.T) {
	f := newAgeFixture(t, commitAge30, `version = "^1"`)
	require.Equal(t, 0, writeLockAt("", "", nil))
	require.Equal(t, "v1.1.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)

	// "0" on the source switches the gate off for it.
	f.setConfig(strings.Replace(f.config(), `version = "^1"`, `version = "^1"`+"\nmin_release_age = \"0\"", 1))
	var code int
	_, _ = capture(t, func() { code = runUpdate(nil) })

	require.Equal(t, 0, code)
	e := f.lock().Find(lockfile.KindInclude, "shared")
	assert.Equal(t, "v1.2.0", e.Tag)
	assert.Empty(t, e.Released, "no gate, no release time recorded")
}

func TestMinReleaseAge_NothingOldEnoughIsAR730(t *testing.T) {
	f := newAgeFixture(t, "[lock]\nmin_release_age = \"3650d\"\nmin_release_age_source = \"commit\"\n", `version = "^1"`)

	var code int
	_, stderr := capture(t, func() { code = writeLockAt("", "", nil) })

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "AR730")
	assert.Contains(t, stderr, "min_release_age")
	assert.Nil(t, f.lock(), "nothing is written")
}

func TestMinReleaseAge_ForgeIsTheFirstSourceForGitHubRepositories(t *testing.T) {
	// Arrange: a GitHub source gets its release time from the (fake) forge.
	published := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	fake := &forge.Fake{
		ReleasesBy: map[string][]forge.Release{"github.com/o/r": {{Tag: "v1.2.3", Published: published, Commit: strings.Repeat("a", 40)}}},
		Tags:       map[string]forge.TagInfo{"github.com/o/r@v1.2.3": {Name: "v1.2.3", Commit: strings.Repeat("a", 40)}},
	}
	prev := newForgeClient
	newForgeClient = func(*config.Config, bool) forge.Client { return fake }
	t.Cleanup(func() { newForgeClient = prev })
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{Lock: &config.LockConfig{MinReleaseAge: "7d"}}
	gates := newAgeGates(cfg)
	w := lockfile.Want{Kind: lockfile.KindInclude, Name: "shared", Source: "https://github.com/o/r", Constraint: "^1"}

	// Act
	gate := gates.gateFor(w)
	require.NotNil(t, gate)
	rt, err := gate.Timer.ReleaseTime(context.Background(), tagresolve.RawTag{Name: "v1.2.3", Commit: strings.Repeat("a", 40)})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, tagresolve.SourceForge, rt.From)
	assert.True(t, rt.At.Equal(published))
	assert.Equal(t, 7*24*time.Hour, gate.Min)
	assert.NotNil(t, gates.gateFor(w), "one timer serves a repository")
	assert.Nil(t, gates.gateFor(lockfile.Want{Source: "https://github.com/o/r"}), "a source without a constraint has no gate")
	w.MinReleaseAge = "0"
	assert.Nil(t, gates.gateFor(w), `min_release_age = "0" switches the gate off`)
}

func TestOutdatedSeverityOptIn(t *testing.T) {
	tests := []struct {
		name     string
		lint     string
		wantCode int
		wantSev  string
	}{
		{"off by default", "", 0, ""},
		{"error fails the run", "[lint.severity]\nAR734 = \"error\"\n", exitDrift, "error"},
		{"warning reports without failing", "[lint.severity]\nsource-outdated = \"warning\"\n", 0, "warning"},
		{"off is off", "[lint.severity]\nAR734 = \"off\"\n", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: v1.0.0 pinned, v1.2.0 allowed (no min age here).
			f := newAgeFixture(t, tt.lint, `version = "~1.0.0"`)
			require.Equal(t, 0, writeLockAt("", "", nil))
			f.setConfig(strings.Replace(f.config(), `version = "~1.0.0"`, `version = "^1"`, 1))
			lockOutdated, lockFormat = true, formatJSON

			// Act
			var code int
			stdout := captureStdout(t, func() { code = outdatedAt("", "", nil) })

			// Assert
			assert.Equal(t, tt.wantCode, code)
			validateAgainst(t, "../../schema/lock-outdated.schema.json", []byte(stdout))
			var rep tagresolve.Report
			require.NoError(t, json.Unmarshal([]byte(stdout), &rep), stdout)
			assert.Equal(t, tt.wantSev, rep.Sources[0].Severity)
			if tt.wantSev != "" {
				assert.Equal(t, "AR734", rep.Sources[0].Code)
			}
		})
	}
}

// update.schema.json carries its own copy of the outdated row (a $ref to the
// published lock-outdated schema would validate against an older release), so
// the two must not drift apart.
func TestUpdateSchemaRowMatchesTheOutdatedSchema(t *testing.T) {
	read := func(name string) map[string]any {
		data, err := os.ReadFile(filepath.Join("..", "..", "schema", name))
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, json.Unmarshal(data, &doc))
		return doc
	}
	outdated, update := read("lock-outdated.schema.json"), read("update.schema.json")

	row := outdated["properties"].(map[string]any)["sources"].(map[string]any)["items"]
	assert.Equal(t, row, update["definitions"].(map[string]any)["row"])
	assert.Equal(t, outdated["definitions"].(map[string]any)["tagRef"], update["definitions"].(map[string]any)["tagRef"])
	assert.Equal(t, outdated["definitions"].(map[string]any)["held"], update["definitions"].(map[string]any)["held"])
}
