package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func TestCLI_ValidateReportsMalformedFrontmatterOnce(t *testing.T) {
	// Arrange
	dir := testutil.CreateTempDir(t)
	writeIn(t, dir+"/.ai-rulez", "config.toml", "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
	writeIn(t, dir+"/.ai-rulez/agents", "bad.md", "---\nname: a: b: c\n---\nAgent\n")

	// Act
	validate := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "validate")
	list := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "list", "agents")

	// Assert
	require.NotEqual(t, 0, validate.ExitCode)
	vout := validate.Stdout + validate.Stderr
	assert.Equal(t, 0, strings.Count(vout, "Ignoring malformed YAML frontmatter"), vout)
	assert.Contains(t, vout, "bad.md", "the error names the file")
	require.Equal(t, 0, list.ExitCode, list.Stderr)
	assert.Equal(t, 1, strings.Count(list.Stdout+list.Stderr, "Ignoring malformed YAML frontmatter"), "list has no error, so it keeps the warning")
}

const malformedSkillProject = "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\ngitignore = false\n"

func malformedSkill(t *testing.T) string {
	t.Helper()
	dir := testutil.CreateTempDir(t)
	writeIn(t, dir+"/.ai-rulez", "config.toml", malformedSkillProject)
	writeIn(t, dir+"/.ai-rulez/skills/bad", "SKILL.md", "---\nname: bad\ndescription: [unclosed\n---\nBody\n")
	return dir
}

func TestCLI_GenerateNamesTheLineAndParseErrorOfMalformedFrontmatter(t *testing.T) {
	dir := malformedSkill(t)

	res := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "generate")

	assert.Equal(t, 1, res.ExitCode)
	out := res.Stdout + res.Stderr
	assert.Contains(t, out, "AR306")
	assert.Regexp(t, `SKILL\.md:\d+`, out, "file:line")
	assert.Contains(t, out, "yaml:", "the parse error")
	assert.NotContains(t, out, "skill missing 'description'", "noise caused by the malformed parse")
}

func TestCLI_ValidateDoesNotContradictItselfOnMalformedFrontmatter(t *testing.T) {
	dir := malformedSkill(t)

	res := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), "validate")

	assert.Equal(t, 2, res.ExitCode)
	out := res.Stdout + res.Stderr
	assert.Contains(t, out, "AR306")
	assert.NotContains(t, out, "Configuration is valid")
	assert.NotContains(t, out, "skill missing 'description'")
	assert.NotContains(t, out, "AR801", "the description is unreadable, not missing")
}

func TestCLI_PresetTypoGetsADidYouMeanFromGenerateAndValidate(t *testing.T) {
	dir := testutil.CreateTempDir(t)
	writeIn(t, dir+"/.ai-rulez", "config.toml", "version = \"5.0\"\nname = \"x\"\npresets = [\"claudee\"]\ngitignore = false\n")

	for _, cmd := range []string{"generate", "validate"} {
		res := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), cmd)

		assert.Equal(t, 1, res.ExitCode, cmd)
		out := res.Stdout + res.Stderr
		assert.Contains(t, out, `did you mean "claude"?`, cmd)
		assert.NotContains(t, out, "oneOf", cmd+": no raw schema dump")
		assert.NotContains(t, out, "should not match the not schema", cmd)
	}
}
