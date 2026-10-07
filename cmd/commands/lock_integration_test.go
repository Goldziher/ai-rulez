package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// dynamicLockProject is a project with authored content, a served skill and a
// local skill source, so one lock file has to carry every kind of entry.
func dynamicLockProject(t *testing.T) (root, vendor string) {
	t.Helper()
	// A local source of a project config must live inside the project.
	root = lockProject(t, "\n[lock]\nenforce = true\n\n[[skill_sources]]\nname = \"vendor\"\nurl = \"vendor-skills\"\nname_prefix = \"v-\"\n")
	vendor = filepath.Join(root, "vendor-skills")
	writeFile(t, filepath.Join(vendor, "pdf", "SKILL.md"), "---\nname: pdf\ndescription: Work with PDFs\n---\n\n# pdf\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "heavy", "SKILL.md"),
		"---\nname: heavy\ndescription: Heavy served skill. Use when it is heavy.\ndelivery: served\n---\nHEAVY\n")
	return root, vendor
}

func TestLock_OneFileHoldsContentPinsSourcesAndServedSkills(t *testing.T) {
	root, _ := dynamicLockProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))

	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	assert.Equal(t, lockfile.Version, lock.Version)
	assert.True(t, lock.HasContentPins())
	require.Len(t, lock.Source, 1)
	assert.Equal(t, "vendor", lock.Source[0].Name)
	var served []string
	for _, e := range lock.Served {
		served = append(served, e.Name)
		assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, e.Digest, "served digests use the content-pin scheme")
	}
	assert.Equal(t, []string{"heavy", "v-pdf"}, served)
	assert.Equal(t, contentlock.TreeOf(lock), lock.Tree, "the tree digest covers sources and served skills too")

	var code int
	_, stderr := capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, 0, code, stderr)

	cfg, err := loadForLock("")
	require.NoError(t, err)
	drift := lockDriftFor(t.Context(), cfg)
	assert.Empty(t, drift, "[lock] enforce: nothing drifted")
}

func TestLock_CheckAndDiffReportServedAndSourceDrift(t *testing.T) {
	root, vendor := dynamicLockProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))

	// A served skill is edited: --check fails and names it, --diff lists it with scope "served".
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "heavy", "SKILL.md"),
		"---\nname: heavy\ndescription: Heavy served skill, edited. Use when it is heavy.\ndelivery: served\n---\nHEAVY2\n")
	var code int
	_, stderr := capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "skill heavy", "the authored source changed")
	assert.Contains(t, stderr, "served heavy", "and the served digest no longer matches")

	lockDiffFlag, lockFormat = true, formatJSON
	stdout, _ := capture(t, func() { code = diffLockAt("") })
	require.Equal(t, 0, code)
	validateAgainst(t, "../../schema/lock-diff.schema.json", []byte(stdout))
	assert.Contains(t, stdout, `"scope": "served"`)
	lockDiffFlag, lockFormat = false, ""

	// Re-locking brings everything back in sync, served digests included.
	require.Equal(t, 0, writeLockAt("", "", nil))
	_, stderr = capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, 0, code, stderr)

	// A source's content changes: the source pin and the served skill both drift.
	writeFile(t, filepath.Join(vendor, "pdf", "SKILL.md"), "---\nname: pdf\ndescription: Work with PDFs, changed\n---\n\n# pdf\n")
	_, stderr = capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "vendor")
}

func TestLock_HandEditedSourcePinBreaksTheTreeDigest(t *testing.T) {
	root, _ := dynamicLockProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))
	dir := filepath.Join(root, ".ai-rulez")
	lock, err := lockfile.Load(dir)
	require.NoError(t, err)
	tree := lock.Tree
	lock.Source[0].Digest = "sha256:" + string(bytes.Repeat([]byte("0"), 64))
	require.NoError(t, lockfile.Save(dir, lock))
	reloaded, err := lockfile.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, tree, reloaded.Tree)
	assert.NotEqual(t, tree, contentlock.TreeOf(reloaded))

	var code int
	_, stderr := capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "tree digest")
}

func TestLock_Version1FileStillLoadsAndKeepsItsSourceAndServedEntries(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, lockfile.FileName), []byte(`version = 1

[[include]]
name = "shared"
source = "https://example.com/r"
commit = "abc"
digest = "sha256:1"

[[source]]
name = "vendor"
source = "/x"
commit = ""
digest = "sha256:2"

[[served]]
name = "heavy"
source = ".ai-rulez/skills/heavy/SKILL.md"
commit = ""
digest = "sha256:3"
`), 0o644))
	lock, err := lockfile.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, lock.Version)
	assert.False(t, lock.HasContentPins())
	assert.NotNil(t, lock.Find(lockfile.KindInclude, "shared"))
	assert.NotNil(t, lock.Find(lockfile.KindSource, "vendor"))
	assert.NotNil(t, lock.Find(lockfile.KindServed, "heavy"))
}

func TestLock_ContentOnlyRefreshesLocalServedDigestsOffline(t *testing.T) {
	root, _ := dynamicLockProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil))
	dir := filepath.Join(root, ".ai-rulez")
	before, err := lockfile.Load(dir)
	require.NoError(t, err)

	writeFile(t, filepath.Join(dir, "skills", "heavy", "SKILL.md"),
		"---\nname: heavy\ndescription: Heavy served skill, edited. Use when it is heavy.\ndelivery: served\n---\nHEAVY2\n")
	lockContentOnly = true
	require.Equal(t, 0, writeLockAt("", "", nil))
	after, err := lockfile.Load(dir)
	require.NoError(t, err)
	assert.NotEqual(t, before.Find(lockfile.KindServed, "heavy").Digest, after.Find(lockfile.KindServed, "heavy").Digest)
	assert.Equal(t, before.Source, after.Source, "sources are not re-resolved by --content-only")

	lockContentOnly = false
	var code int
	_, stderr := capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, 0, code, stderr)
}
