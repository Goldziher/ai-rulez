package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetStrictFlags restores every package-level flag the strict pipeline reads.
func resetStrictFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		validateStrict, validateFormat, validateFailOn, validateOutput = false, "", "", ""
		validateBaseline, validateUpdateBaseline, validateBaselineReason, validateStrictBaseline, validateToday = "", false, "", false, ""
		validateLintProfile, validateSince, validateChanged, validateAnalyzers = "", "", false, nil
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

func TestRatchetToleratesUpToTheCount(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "\n[lint.ratchet]\nAR201 = 1\n", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	assert.Equal(t, 0, runStrict(t, root, cfg), "one AR201 is within a ratchet of 1")

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

func TestRiskInReportDoesNotBlock(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "\n[lint.risk]\nerror = 0\n", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	code := runStrict(t, root, cfg)
	assert.Equal(t, exitStrictFindings, code, "an error still fails regardless of weights")
	data, err := os.ReadFile(validateOutput)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"risk"`)
	assert.Contains(t, string(data), `"label": "high"`, "the severity floor applies even with a zero weight")
	assert.Contains(t, string(data), `"score": 0`)

	validateFailOn = "none"
	assert.Equal(t, 0, runStrict(t, root, cfg), "--fail-on none never blocks either")
}

const missingPathRule = "---\ndescription: a rule\n---\nEdit `src/nope.go` first.\n"

func TestLintProfileEndToEnd(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{
		".ai-rulez/rules/a.md": missingPathRule,
		"src/real.go":          "package src\n",
	})
	assert.Equal(t, 0, runStrict(t, root, cfg), "AR401 is a warning by default")

	validateLintProfile = "strict"
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)), "strict promotes it and fails on warnings")
	data, err := os.ReadFile(validateOutput)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"profile": "strict"`)

	validateFailOn = "none"
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)), "--fail-on beats the preset threshold")
	validateFailOn = ""

	validateLintProfile = ""
	root2, _ := strictProject(t, "\n[lint]\nprofile = \"permissive\"\n", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	assert.Equal(t, 0, runStrict(t, root2, loadStrictProject(t, root2)), "permissive turns the broken link into a warning")
}

func TestFailOnPrecedenceWithProfile(t *testing.T) {
	resetStrictFlags(t)
	strictCfg := &config.Config{Lint: &config.LintConfig{Profile: "strict"}}
	assert.Equal(t, "warning", failOnFor(strictCfg))
	strictCfg.Lint.FailOn = "none"
	assert.Equal(t, "none", failOnFor(strictCfg), "[lint] fail_on beats the preset")
	validateFailOn = "error"
	assert.Equal(t, "error", failOnFor(strictCfg), "--fail-on beats both")
}

func TestLintProfileFlagBeatsConfigFailOn(t *testing.T) {
	resetStrictFlags(t)
	cfg := &config.Config{Lint: &config.LintConfig{FailOn: "warning"}}
	assert.Equal(t, "warning", failOnFor(cfg))
	validateLintProfile = "permissive"
	assert.Equal(t, "error", failOnFor(cfg), "--lint-profile beats [lint] fail_on")
	validateLintProfile = "default"
	assert.Equal(t, "error", failOnFor(cfg), "the default profile fails on errors")
	validateLintProfile = "strict"
	validateFailOn = "none"
	assert.Equal(t, "none", failOnFor(cfg), "--fail-on still beats --lint-profile")
}

func TestLintProfileFlagValidation(t *testing.T) {
	resetStrictFlags(t)
	validateLintProfile = "paranoid"
	assert.Error(t, checkStrictFlags())
	validateLintProfile = "strict"
	assert.NoError(t, checkStrictFlags())
	validateStrict = false
	assert.Error(t, checkStrictFlags(), "--lint-profile needs --strict on validate")
	assert.NotNil(t, ValidateCmd.Flags().Lookup("lint-profile"))
	assert.Nil(t, ValidateCmd.Flags().Lookup("profile"), "the generation --profile flag is not reused")
	t.Cleanup(func() { validateLintProfile = "" })
}

func TestAnalyzerFilter(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	assert.Equal(t, exitStrictFindings, runStrict(t, root, cfg))

	validateAnalyzers = []string{"security"}
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)), "the broken link belongs to the references analyzer")
	data, err := os.ReadFile(validateOutput)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "AR201")

	validateAnalyzers = []string{"references"}
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)))
	data, err = os.ReadFile(validateOutput)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"analyzer": "references"`)
	assert.Contains(t, string(data), `"scope": "file"`)

	validateAnalyzers = []string{"nope"}
	assert.Error(t, checkStrictFlags())
	validateStrict = false
	validateAnalyzers = []string{"security"}
	assert.Error(t, checkStrictFlags(), "--analyzer needs --strict")
}

func TestScanUpdateBaselineKeepsNonSecurityEntries(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	basePath := filepath.Join(root, ".ai-rulez", lint.BaselineFile)

	validateUpdateBaseline, validateBaselineReason = true, "legacy"
	require.Equal(t, 0, runStrict(t, root, cfg))
	validateUpdateBaseline, validateBaselineReason = false, ""
	before, err := lint.LoadBaseline(basePath)
	require.NoError(t, err)
	require.Len(t, before.Entries, 1)

	strictSecurityOnly = true
	t.Cleanup(func() { strictSecurityOnly = false })
	validateUpdateBaseline = true
	require.Equal(t, 0, runStrict(t, root, cfg))
	validateUpdateBaseline = false
	after, err := lint.LoadBaseline(basePath)
	require.NoError(t, err)
	assert.Equal(t, before.Entries, after.Entries, "scan must not delete the entries of rules it does not run")

	validateStrictBaseline = true
	assert.Equal(t, 0, runStrict(t, root, cfg), "scan does not report non-security entries as stale")
}

func TestUpdateBaselineRejectsNarrowedRuns(t *testing.T) {
	resetStrictFlags(t)
	validateUpdateBaseline = true
	validateLintProfile = "permissive"
	assert.Error(t, checkStrictFlags(), "a profile override hides findings")
	validateLintProfile = ""
	validateAnalyzers = []string{"security"}
	assert.Error(t, checkStrictFlags(), "an analyzer filter is a narrowed run")
	validateAnalyzers = nil

	validateBaseline = "shared.json"
	err := updateBaselines([]*lint.Report{{}, {}}, []*config.Config{nil, nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single root")
}

func TestAnalyzerRunKeepsBaselineEntriesOfAnalyzersThatDidNotRun(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	basePath := filepath.Join(root, ".ai-rulez", lint.BaselineFile)
	validateUpdateBaseline = true
	require.Equal(t, 0, runStrict(t, root, cfg))
	validateUpdateBaseline = false
	before, err := lint.LoadBaseline(basePath)
	require.NoError(t, err)
	require.Len(t, before.Entries, 1)
	require.Equal(t, "AR201", before.Entries[0].Code)

	// Act: a security-only run does not run the references analyzer
	validateAnalyzers = []string{lint.AnalyzerSecurity}
	validateStrictBaseline = true
	strictRun := runStrict(t, root, loadStrictProject(t, root))
	validateStrictBaseline = false
	validateUpdateBaseline = true
	updateRun := runStrict(t, root, loadStrictProject(t, root))
	validateUpdateBaseline = false

	// Assert
	assert.Equal(t, 0, strictRun, "an entry of an analyzer that did not run is not stale")
	assert.Equal(t, 0, updateRun)
	after, err := lint.LoadBaseline(basePath)
	require.NoError(t, err)
	assert.Equal(t, before.Entries, after.Entries, "--update-baseline keeps the entry untouched")
}
