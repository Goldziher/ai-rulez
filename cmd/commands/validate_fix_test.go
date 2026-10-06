package commands

import (
	"os"
	"path/filepath"
	"testing"

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
