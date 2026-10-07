package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const typoSkill = "---\nname: Typo_Skill\ndescription: Use when you need the typo skill for tests.\nallowed_tools: Read\n---\nbody\n"

func resetFixFlags(t *testing.T) {
	t.Helper()
	resetStrictFlags(t)
	t.Cleanup(func() { validateFix, validateFixUnsafe, validateDryRun = false, false, false })
}

func TestFixEndToEnd(t *testing.T) {
	resetFixFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/skills/typo-skill/SKILL.md": typoSkill})
	skill := filepath.Join(root, ".ai-rulez", "skills", "typo-skill", "SKILL.md")
	validateFailOn = "warning"
	require.Equal(t, exitStrictFindings, runStrict(t, root, cfg))

	// A dry run changes nothing and still reports the findings.
	validateFix, validateDryRun = true, true
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)))
	data, err := os.ReadFile(skill)
	require.NoError(t, err)
	assert.Equal(t, typoSkill, string(data))

	// --fix repairs the key (safe) but not the name (unsafe): one warning is left.
	validateDryRun = false
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)))
	data, err = os.ReadFile(skill)
	require.NoError(t, err)
	assert.Contains(t, string(data), "allowed-tools: Read")
	assert.Contains(t, string(data), "name: Typo_Skill")

	// --fix-unsafe finishes the job, so the run is clean.
	validateFixUnsafe = true
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)))
	data, err = os.ReadFile(skill)
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: typo-skill")

	// And it is idempotent.
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)))
}

func TestPrintFixSummaryNamesHandWrittenFiles(t *testing.T) {
	const hand = "1 in hand-written files outside the configuration directory"
	tests := []struct {
		name     string
		applied  int
		outside  int
		wantGen  bool
		wantHand bool
	}{
		{"sources only", 2, 0, true, false},
		{"hand-written files only", 1, 1, false, true},
		{"both", 3, 1, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var buf strings.Builder
			// Act
			printFixSummary(&buf, tt.applied, tt.outside, nil)
			// Assert
			out := buf.String()
			assert.Equal(t, tt.wantGen, strings.Contains(out, "ai-rulez generate"), out)
			assert.Equal(t, tt.wantHand, strings.Contains(out, hand), out)
		})
	}
}

func TestAppliedOutsideCountsOnlyFilesOutsideTheConfigDir(t *testing.T) {
	root := gitutil.Resolve(t.TempDir())
	inside := filepath.Join(root, ".ai-rulez", "skills", "a", "SKILL.md")
	dotted := filepath.Join(root, ".ai-rulez", "..foo.md")
	outside := filepath.Join(root, "CLAUDE.md")
	applied := []lint.FixApplied{
		{File: ".ai-rulez/skills/a/SKILL.md", Targets: []string{inside}},
		{File: "CLAUDE.md", Targets: []string{outside}},
		{File: "x", Targets: []string{dotted}},
	}
	assert.Equal(t, 1, appliedOutside(applied, filepath.Join(root, ".ai-rulez")))
	assert.Equal(t, 0, appliedOutside(applied, ""))
}

func TestFixFlagValidation(t *testing.T) {
	resetFixFlags(t)
	validateStrict = false
	validateFix = true
	assert.Error(t, checkStrictFlags(), "--fix needs --strict")
	validateStrict, validateFix = true, false
	validateDryRun = true
	assert.Error(t, checkStrictFlags(), "--dry-run needs --fix")
	validateFixUnsafe = true
	assert.NoError(t, checkStrictFlags(), "--fix-unsafe implies --fix")
	validateUpdateBaseline = true
	assert.Error(t, checkStrictFlags(), "--fix and --update-baseline conflict")
	assert.NotNil(t, ValidateCmd.Flags().Lookup("fix"))
	assert.NotNil(t, ValidateCmd.Flags().Lookup("fix-unsafe"))
	assert.NotNil(t, ValidateCmd.Flags().Lookup("dry-run"))
}

func TestFixRepairsBooleansFencesAndFinalNewline(t *testing.T) {
	resetFixFlags(t)
	broken := "---\nalwaysApply: \"true\"\ndescription: a rule\n---\n# Title\n```bash\nls"
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": broken})
	rule := filepath.Join(root, ".ai-rulez", "rules", "a.md")
	validateFailOn = "info"
	require.Equal(t, exitStrictFindings, runStrict(t, root, cfg))

	// A dry run shows the diff and writes nothing.
	validateFix, validateDryRun = true, true
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)))
	data, err := os.ReadFile(rule)
	require.NoError(t, err)
	assert.Equal(t, broken, string(data))

	// --fix repairs all three and the run is clean; a second run changes nothing.
	validateDryRun = false
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)))
	data, err = os.ReadFile(rule)
	require.NoError(t, err)
	assert.Equal(t, "---\nalwaysApply: true\ndescription: a rule\n---\n# Title\n```bash\nls\n```\n", string(data))
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)))
	again, err := os.ReadFile(rule)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again))
}

func TestFixNeverRewritesBaselineAcceptedFindings(t *testing.T) {
	resetFixFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/skills/typo-skill/SKILL.md": typoSkill})
	skill := filepath.Join(root, ".ai-rulez", "skills", "typo-skill", "SKILL.md")
	validateUpdateBaseline = true
	require.Equal(t, 0, runStrict(t, root, cfg))
	validateUpdateBaseline = false

	validateFixUnsafe = true
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)), "accepted findings do not fail")
	data, err := os.ReadFile(skill)
	require.NoError(t, err)
	assert.Equal(t, typoSkill, string(data), "a baseline-accepted finding is not fixed")

	validateStrictBaseline = true
	assert.Equal(t, 0, runStrict(t, root, loadStrictProject(t, root)), "the entries stay live")
}

func TestFixRespectsAnalyzerFilter(t *testing.T) {
	resetFixFlags(t)
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/skills/typo-skill/SKILL.md": typoSkill})
	skill := filepath.Join(root, ".ai-rulez", "skills", "typo-skill", "SKILL.md")
	validateFixUnsafe, validateAnalyzers = true, []string{"security"}
	runStrict(t, root, cfg)
	data, err := os.ReadFile(skill)
	require.NoError(t, err)
	assert.Equal(t, typoSkill, string(data), "--analyzer security must not fix other rules")
}

func TestFixRespectsChangedScope(t *testing.T) {
	resetFixFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), validRootConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "typo-skill", "SKILL.md"), typoSkill)
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "other-skill", "SKILL.md"), "---\nname: Other_Skill\ndescription: Use when you need the other skill for tests.\n---\nbody\n")
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "init")
	changedSkill := filepath.Join(root, ".ai-rulez", "skills", "typo-skill", "SKILL.md")
	writeFile(t, changedSkill, typoSkill+"edited\n")

	validateFixUnsafe, validateChanged = true, true
	runStrict(t, root, loadStrictProject(t, root))

	fixed, err := os.ReadFile(changedSkill)
	require.NoError(t, err)
	assert.Contains(t, string(fixed), "name: typo-skill", "the changed file is fixed")
	untouched, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "skills", "other-skill", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(untouched), "name: Other_Skill", "a file outside --changed is not rewritten")
}
