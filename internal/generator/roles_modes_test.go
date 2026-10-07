package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const modesProjectConfig = `version = "5.0"
agents_md = false
name = "modes"
presets = [%s]
gitignore = false
%s
[[roles]]
name = "r"
[roles.skill_mode]
user = "user-invocable-only"
hidden = "off"
listed = "name-only"
`

func modesProject(t *testing.T, presets, extra string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".ai-rulez")
	write := func(rel, body string) {
		p := filepath.Join(cfg, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("config.toml", sprintfModes(presets, extra))
	for _, name := range []string{"user", "hidden", "listed", "plain"} {
		write("skills/"+name+"/SKILL.md", "---\nname: "+name+"\ndescription: Use when you need "+name+".\n---\n# "+name+"\n")
	}
	return dir
}

func sprintfModes(presets, extra string) string {
	return fmt.Sprintf(modesProjectConfig, presets, extra)
}

func modesRead(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func TestRoleSkillModeRendersWhereDocumented(t *testing.T) {
	// Arrange
	dir := modesProject(t, `"claude", "cursor", "codex", "copilot"`, "")

	// Act
	require.NoError(t, roleGenerator(t, dir, "r").Generate(""))

	// Assert: user-invocable-only is the documented frontmatter key, or Codex's policy.
	assert.Contains(t, modesRead(t, dir, ".agents/skills/user/SKILL.md"), "disable-model-invocation: true", "cursor")
	assert.Contains(t, modesRead(t, dir, ".github/skills/user/SKILL.md"), "disable-model-invocation: true", "copilot")
	assert.Contains(t, modesRead(t, dir, ".agents/skills/user/agents/openai.yaml"), "allow_implicit_invocation: false", "codex")
	assert.NotContains(t, modesRead(t, dir, ".agents/skills/plain/SKILL.md"), "disable-model-invocation")
	assert.NotContains(t, modesRead(t, dir, ".agents/skills/listed/SKILL.md"), "disable-model-invocation", "name-only has no equivalent")

	// off: cursor and codex have no documented setting, so the skill follows the fallback (drop).
	assert.NoFileExists(t, filepath.Join(dir, ".agents", "skills", "hidden", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".github", "skills", "hidden", "SKILL.md"))
	assert.Equal(t, "user-invocable-only", readSettings(t, dir)["skillOverrides"].(map[string]any)["user"]) //nolint:errcheck // asserted
	assert.NotContains(t, readSettings(t, dir)["skillOverrides"], "hidden", "a dropped skill gets no override")
}

func TestRoleSkillModeOffWhereEveryHarnessDocumentsIt(t *testing.T) {
	// Arrange: claude (skillOverrides) and copilot (both keys) can both hide a skill.
	dir := modesProject(t, `"claude", "copilot"`, "")

	// Act
	require.NoError(t, roleGenerator(t, dir, "r").Generate(""))

	// Assert
	hidden := modesRead(t, dir, ".github/skills/hidden/SKILL.md")
	assert.Contains(t, hidden, "disable-model-invocation: true")
	assert.Contains(t, hidden, "user-invocable: false")
	assert.Equal(t, "off", readSettings(t, dir)["skillOverrides"].(map[string]any)["hidden"]) //nolint:errcheck // asserted
}

func TestRoleSkillModeOffFallbackServe(t *testing.T) {
	// Arrange
	dir := modesProject(t, `"claude", "cursor"`, "[role_manifest]\nskill_mode_fallback = \"serve\"\n")

	// Act
	require.NoError(t, roleGenerator(t, dir, "r").Generate(""))

	// Assert: served skills leave the static trees, the stub replaces them.
	assert.NoFileExists(t, filepath.Join(dir, ".agents", "skills", "hidden", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "hidden", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "user", "SKILL.md"))
}

func TestRoleSkillModeClaudeOnlyKeepsSettingsOnly(t *testing.T) {
	// Arrange
	dir := modesProject(t, `"claude"`, "")

	// Act
	require.NoError(t, roleGenerator(t, dir, "r").Generate(""))

	// Assert: nothing else is needed, so the skill is still rendered and hidden by the setting.
	assert.NotContains(t, modesRead(t, dir, ".claude/skills/hidden/SKILL.md"), "disable-model-invocation: true\nuser-invocable: false")
	assert.Equal(t, "off", readSettings(t, dir)["skillOverrides"].(map[string]any)["hidden"]) //nolint:errcheck // asserted
}

func TestRoleSkillModeKeepsAuthorKeys(t *testing.T) {
	// Arrange
	dir := modesProject(t, `"claude", "cursor"`, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "skills", "user", "SKILL.md"),
		[]byte("---\nname: user\ndescription: Use when you need user.\ndisable-model-invocation: false\n---\n# user\n"), 0o644))

	// Act
	require.NoError(t, roleGenerator(t, dir, "r").Generate(""))

	// Assert: the author's value stands.
	assert.Contains(t, modesRead(t, dir, ".agents/skills/user/SKILL.md"), "disable-model-invocation: false")
}
