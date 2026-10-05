package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetStrictFlags restores every package-level flag the strict pipeline reads.
func resetStrictFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		validateStrict, validateFormat, validateFailOn, validateOutput = false, "", "", ""
		validateBaseline, validateUpdateBaseline, validateBaselineReason, validateStrictBaseline, validateToday = "", false, "", false, ""
		strictTreeCache = lint.Loader{}
	})
	validateStrict = true
}

func runStrict(t *testing.T, root string, cfg *config.Config) int {
	t.Helper()
	validateOutput = filepath.Join(t.TempDir(), "report.json")
	validateFormat = "json"
	report := lintProject(t, cfg)
	return reportStrict([]*lint.Report{report}, []*config.Config{cfg})
}

func TestBaselineRatchetEndToEnd(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	basePath := filepath.Join(root, ".ai-rulez", lint.BaselineFile)

	assert.Equal(t, exitStrictFindings, runStrict(t, root, cfg), "an unaccepted error fails")

	validateUpdateBaseline = true
	assert.Equal(t, 0, runStrict(t, root, cfg), "--update-baseline exits 0")
	validateUpdateBaseline = false
	b, err := lint.LoadBaseline(basePath)
	require.NoError(t, err)
	require.Len(t, b.Entries, 1)
	assert.Equal(t, "AR201", b.Entries[0].Code)

	assert.Equal(t, 0, runStrict(t, root, cfg), "the accepted finding no longer counts")

	// Moving the finding down the file keeps it accepted.
	moved := strings.Replace(brokenLinkRule, "# Title\n", "# Title\n\nmore text\n\nand more\n", 1)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "a.md"), moved)
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)))

	// A new finding is not covered.
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "b.md"), strings.ReplaceAll(brokenLinkRule, "docs/missing.md", "docs/other.md"))
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)), "a new finding fails")
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "rules", "b.md")))

	// Fixing the accepted one leaves a stale entry: fine by default, a failure with --strict-baseline.
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "a.md"), "---\ndescription: a rule\n---\n# Title\n")
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)))
	validateStrictBaseline = true
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)), "the ratchet rejects stale entries")
	validateStrictBaseline = false

	validateUpdateBaseline = true
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)))
	validateUpdateBaseline = false
	b, err = lint.LoadBaseline(basePath)
	require.NoError(t, err)
	assert.Empty(t, b.Entries, "updating prunes stale entries")
}

func TestBaselineExpiryUsesPinnedDate(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	validateUpdateBaseline = true
	require.Equal(t, 0, runStrict(t, root, cfg))
	validateUpdateBaseline = false

	basePath := filepath.Join(root, ".ai-rulez", lint.BaselineFile)
	b, err := lint.LoadBaseline(basePath)
	require.NoError(t, err)
	b.Entries[0].Expires = "2026-06-30"
	require.NoError(t, b.Save(basePath))

	validateToday = "2026-06-30"
	assert.Equal(t, 0, runStrict(t, root, cfg), "still accepted on the expiry day")
	validateToday = "2026-07-01"
	assert.Equal(t, exitStrictFindings, runStrict(t, root, cfg), "expired entries stop accepting")
	validateToday = "soon"
	assert.Equal(t, 1, runStrict(t, root, cfg), "a malformed date is an error, not a silent pass")
}

func TestExplicitBaselineMustExist(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	validateBaseline = filepath.Join(root, "nope.json")
	assert.Equal(t, 1, runStrict(t, root, cfg))
}

func TestBudgetToleratesAndRatchets(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "\n[lint.budget]\nAR201 = 1\n", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	assert.Equal(t, 0, runStrict(t, root, cfg), "one AR201 is within a budget of 1")

	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "b.md"), strings.ReplaceAll(brokenLinkRule, "docs/missing.md", "docs/other.md"))
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)), "two exceed it")
}

func TestUpdateBaselineRefusesUnexplainedSecurityFinding(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": "---\ndescription: a rule\n---\nkey AKIAIOSFODNN7EXAMPLE\n"})
	validateUpdateBaseline = true
	assert.Equal(t, 1, runStrict(t, root, cfg))
	validateBaselineReason = "documentation fixture"
	assert.Equal(t, 0, runStrict(t, root, cfg))
}

func TestBaselineFlagValidation(t *testing.T) {
	resetStrictFlags(t)
	validateStrict = false
	validateUpdateBaseline = true
	assert.Error(t, checkStrictFlags(), "baseline flags need --strict")
	validateStrict = true
	validateStrictBaseline = true
	assert.Error(t, checkStrictFlags(), "update and strict-baseline conflict")
	validateStrictBaseline, validateUpdateBaseline, validateBaselineReason = false, false, "why"
	assert.Error(t, checkStrictFlags(), "a reason without --update-baseline")
}
