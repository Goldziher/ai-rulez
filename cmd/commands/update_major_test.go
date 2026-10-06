package commands

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// majorFixture locks `version = "^1"` (a comment on the line) with a v2.0.0 on the remote.
func newMajorFixture(t *testing.T) *ageFixture {
	t.Helper()
	f := newAgeFixture(t, "", "version = \"^1\"  # stay on 1.x")
	f.release("four", "v2.0.0", false)
	require.Equal(t, 0, writeLockAt("", "", nil))
	require.Equal(t, "v1.2.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)
	return f
}

func runUpdateJSON(t *testing.T) (updateReport, int) {
	t.Helper()
	updateFormat = formatJSON
	var code int
	stdout := captureStdout(t, func() { code = runUpdate(nil) })
	validateAgainst(t, "../../schema/update.schema.json", []byte(stdout))
	var rep updateReport
	require.NoError(t, json.Unmarshal([]byte(stdout), &rep), stdout)
	return rep, code
}

func TestUpdateMajor_SuggestsAndWritesNothingWithoutWriteConfig(t *testing.T) {
	// Arrange
	f := newMajorFixture(t)
	before, lockBefore := f.config(), f.lock()
	updateMajor = true

	// Act
	rep, code := runUpdateJSON(t)

	// Assert
	require.Equal(t, 0, code)
	require.Len(t, rep.Major, 1)
	assert.Equal(t, majorItem{Kind: "include", Name: "shared", From: "^1", To: "^2.0", Latest: "v2.0.0"}, rep.Major[0])
	assert.Empty(t, rep.Updates)
	assert.Equal(t, before, f.config(), "config.toml is untouched")
	assert.Equal(t, lockBefore, f.lock(), "the lock is untouched")

	updateFormat = ""
	stdout, _ := capture(t, func() { code = runUpdate(nil) })
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, `version = "^2.0"`)
	assert.Contains(t, stdout, "--write-config")
}

func TestUpdateMajor_WriteConfigPatchesOneLineAndMovesThePin(t *testing.T) {
	f := newMajorFixture(t)
	before := f.config()
	updateMajor, updateWriteConfig = true, true

	rep, code := runUpdateJSON(t)

	require.Equal(t, 0, code)
	require.Len(t, rep.Major, 1)
	assert.True(t, rep.Major[0].Written)
	assert.Equal(t, strings.Replace(before, `version = "^1"  # stay on 1.x`, `version = "^2.0"  # stay on 1.x`, 1), f.config(),
		"only the version value changes; the comment and the rest of the file stay")
	e := f.lock().Find(lockfile.KindInclude, "shared")
	assert.Equal(t, "v2.0.0", e.Tag)
	assert.Equal(t, "^2.0", e.Ref, "the lock follows the new constraint")
	require.Len(t, rep.Updates, 1)
	assert.Equal(t, "v2.0.0", rep.Updates[0].To.Tag)

	// Nothing newer is left: a second run finds no major.
	rep2, code2 := runUpdateJSON(t)
	assert.Equal(t, 0, code2)
	assert.Empty(t, rep2.Major)
}

func TestUpdateMajor_DryRunWithWriteConfigWritesNothing(t *testing.T) {
	f := newMajorFixture(t)
	before, lockBefore := f.config(), f.lock()
	updateMajor, updateWriteConfig, updateDryRun = true, true, true

	rep, code := runUpdateJSON(t)

	require.Equal(t, 0, code)
	require.Len(t, rep.Major, 1)
	assert.False(t, rep.Major[0].Written)
	assert.Equal(t, before, f.config())
	assert.Equal(t, lockBefore, f.lock())
}

func TestUpdate_WriteConfigNeedsMajor(t *testing.T) {
	newMajorFixture(t)
	updateWriteConfig = true
	var code int
	_, stderr := capture(t, func() { code = runUpdate(nil) })
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--write-config needs --major")
}

func TestUpdateMajor_NothingToDoWhenNoNewerMajorExists(t *testing.T) {
	f := newAgeFixture(t, "", `version = "^1"`)
	require.Equal(t, 0, writeLockAt("", "", nil))
	updateMajor = true

	rep, code := runUpdateJSON(t)

	assert.Equal(t, 0, code)
	assert.Empty(t, rep.Major)
	_ = f
}

func TestUpdateMajor_ARefusedScanRestoresConfig(t *testing.T) {
	// Arrange: v2.0.0 carries a secret-shaped string, which the scan refuses.
	f := newAgeFixture(t, "", "version = \"^1\"  # keep")
	f.repo.Write(".ai-rulez/rules/leak.md", "# Leak\naws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYQ9d3kGh7Lz\"\nAKIAQYLPMN5HHHFPZAM2\n")
	f.release("four", "v2.0.0", false)
	require.Equal(t, 0, writeLockAt("", "", nil))
	before, lockBefore := f.config(), f.lock()
	updateMajor, updateWriteConfig = true, true

	// Act
	rep, code := runUpdateJSON(t)

	// Assert
	assert.Equal(t, exitDrift, code)
	assert.Equal(t, before, f.config(), "config.toml is restored byte for byte")
	assert.Equal(t, lockBefore, f.lock(), "nothing was written")
	require.Len(t, rep.Updates, 1)
	require.NotNil(t, rep.Updates[0].Scan)
	assert.True(t, rep.Updates[0].Scan.Refused)
	assert.Positive(t, rep.Updates[0].Scan.Errors)
	require.NotEmpty(t, rep.Updates[0].Scan.Findings)
	assert.True(t, strings.HasPrefix(rep.Updates[0].Scan.Findings[0].Code, "AR0"))
	require.Len(t, rep.Major, 1)
	assert.False(t, rep.Major[0].Written, "the report does not claim a write that was rolled back")
}

func TestUpdateMajor_RefusesToGuessWhenConfigCannotBePatched(t *testing.T) {
	// Arrange: the same source written as an inline array, which has no line to rewrite.
	f := newMajorFixture(t)
	cfg := f.config()
	start := strings.Index(cfg, "[[includes]]")
	require.GreaterOrEqual(t, start, 0)
	cfg = cfg[:start] + "includes = [{ name = \"shared\", source = \"" + f.repo.URL + "\", version = \"^1\" }]\n"
	f.setConfig(cfg)
	lockBefore := f.lock()
	updateMajor, updateWriteConfig = true, true

	// Act
	var code int
	_, stderr := capture(t, func() { code = runUpdate(nil) })

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cannot update the constraint")
	assert.Equal(t, cfg, f.config(), "config.toml is left alone")
	assert.Equal(t, lockBefore, f.lock())
}

func TestUpdate_ScanRefusesAPinWithErrorFindingsUnlessAccepted(t *testing.T) {
	// Arrange: v1.3.0 adds a file with a secret-shaped string.
	f := newAgeFixture(t, "", `version = "^1"`)
	require.Equal(t, 0, writeLockAt("", "", nil))
	before := f.lock()
	f.repo.Write(".ai-rulez/rules/leak.md", "# Leak\naws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYQ9d3kGh7Lz\"\nAKIAQYLPMN5HHHFPZAM2\n")
	f.release("one point three", "v1.3.0", false)

	// Act: refused
	rep, code := runUpdateJSON(t)

	// Assert
	assert.Equal(t, exitDrift, code)
	assert.Equal(t, before, f.lock(), "a refused scan writes nothing")
	require.Len(t, rep.Updates, 1)
	assert.True(t, rep.Updates[0].Scan.Refused)
	assert.False(t, rep.Updates[0].Scan.Accepted)

	// --dry-run reports the same refusal and still writes nothing.
	updateDryRun = true
	_, dryCode := runUpdateJSON(t)
	assert.Equal(t, exitDrift, dryCode)
	updateDryRun = false

	// Act: accepted
	updateAcceptFindings = true
	rep, code = runUpdateJSON(t)

	// Assert
	assert.Equal(t, 0, code)
	assert.Equal(t, "v1.3.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)
	require.Len(t, rep.Updates, 1)
	assert.True(t, rep.Updates[0].Scan.Accepted)
	assert.False(t, rep.Updates[0].Scan.Refused)
}

func TestUpdate_CleanTreeReportsZeroFindings(t *testing.T) {
	f := newAgeFixture(t, "", `version = "~1.0.0"`)
	require.Equal(t, 0, writeLockAt("", "", nil))
	f.setConfig(strings.Replace(f.config(), `version = "~1.0.0"`, `version = "^1"`, 1))
	updateDryRun = true
	var code int
	text, _ := capture(t, func() { code = runUpdate(nil) })
	require.Equal(t, 0, code)
	assert.Contains(t, text, "scan: 0 findings")
	updateDryRun = false

	rep, code := runUpdateJSON(t)

	require.Equal(t, 0, code)
	require.Len(t, rep.Updates, 1)
	assert.Equal(t, 0, rep.Updates[0].Scan.Errors)
	assert.False(t, rep.Updates[0].Scan.Refused)
}
