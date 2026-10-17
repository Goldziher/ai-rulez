package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func writeAgentsFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readAgentsFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func gitInitAgents(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, exec.Command("git", "-C", dir, "init", "-q").Run())
}

const (
	e2eAgentsMD = "# Demo\n\nUse tabs.\n"
	e2eSkill    = "---\nname: pr-checklist\ndescription: Review a change before it merges into the main branch\n---\n\nRead the diff first.\n"
)

func TestAgentsSourceLifecycle(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange: init the project, then keep its knowledge in the standards.
	dir := testutil.CreateTempDir(t)
	gitInitAgents(t, dir)
	testutil.RunCLIExpectSuccess(t, dir, "init", "--source", "agents", "--skip-content", "demo")
	assert.FileExists(t, filepath.Join(dir, ".agents", "ai-rulez.toml"))
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"), "init --source agents creates no .ai-rulez tree")
	writeAgentsFile(t, dir, "AGENTS.md", e2eAgentsMD)
	writeAgentsFile(t, dir, ".agents/skills/pr-checklist/SKILL.md", e2eSkill)
	writeAgentsFile(t, dir, ".agents/rules/always.md", "---\ndescription: Always on\n---\n\nALWAYS_ON_BODY keep functions small.\n")
	writeAgentsFile(t, dir, ".agents/ai-rulez.toml",
		"version = \"5.0\"\nname = \"demo\"\ngitignore = false\npresets = [\"claude\", \"cursor\", \"codex\"]\n")

	// Act and assert: generate writes the other tools and leaves the source alone
	first := testutil.RunCLI(t, dir, "generate", "--yes")
	require.Equal(t, 0, first.ExitCode, first.Stderr)
	assert.Equal(t, e2eAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
	assert.Equal(t, e2eSkill, readAgentsFile(t, dir, ".agents/skills/pr-checklist/SKILL.md"))
	assert.Contains(t, readAgentsFile(t, dir, "CLAUDE.md"), "Use tabs.", "CLAUDE.md is the full file with the text of AGENTS.md")
	assert.NotContains(t, readAgentsFile(t, dir, "CLAUDE.md"), "@AGENTS.md")
	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "pr-checklist", "SKILL.md"))
	assert.Contains(t, readAgentsFile(t, dir, ".claude/rules/always.md"), "ALWAYS_ON_BODY", "an always-on .agents rule reaches Claude")
	assert.Contains(t, first.Stderr+first.Stdout, "AGENTS.md and .agents/ are the source")

	// a second run changes nothing and --check agrees
	second := testutil.RunCLI(t, dir, "generate", "--yes")
	require.Equal(t, 0, second.ExitCode, second.Stderr)
	check := testutil.RunCLI(t, dir, "generate", "--check")
	assert.Equal(t, 0, check.ExitCode, "generate --check is clean: %s%s", check.Stdout, check.Stderr)

	// editing a skill is drift until the next generate
	writeAgentsFile(t, dir, ".agents/skills/pr-checklist/SKILL.md", e2eSkill+"\nThen the tests.\n")
	drift := testutil.RunCLI(t, dir, "generate", "--check")
	assert.Equal(t, 2, drift.ExitCode, "an edited source is drift (exit 2): %s%s", drift.Stdout, drift.Stderr)
	testutil.RunCLIExpectSuccess(t, dir, "generate", "--yes")
	assert.Contains(t, readAgentsFile(t, dir, ".claude/skills/pr-checklist/SKILL.md"), "Then the tests.")
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "generate", "--check").ExitCode)

	// validate and doctor accept the project
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "validate").ExitCode)
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "doctor").ExitCode)

	// the lock pins the standards' files and lives beside the config
	testutil.RunCLIExpectSuccess(t, dir, "lock")
	assert.FileExists(t, filepath.Join(dir, ".agents", ".ai-rulez", "ai-rulez.lock"))
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "lock", "--check").ExitCode)
	writeAgentsFile(t, dir, "AGENTS.md", e2eAgentsMD+"More.\n")
	assert.Equal(t, 2, testutil.RunCLI(t, dir, "lock", "--check").ExitCode, "an edit to AGENTS.md moves its pin")
	writeAgentsFile(t, dir, "AGENTS.md", e2eAgentsMD)

	// clean removes the generated files and nothing of the source
	testutil.RunCLIExpectSuccess(t, dir, "clean", "--yes")
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"))
	assert.Equal(t, e2eAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
	assert.Contains(t, readAgentsFile(t, dir, ".agents/skills/pr-checklist/SKILL.md"), "Then the tests.")
	assert.FileExists(t, filepath.Join(dir, ".agents", "ai-rulez.toml"))
}

func TestAgentsSourceCRUDWritesIntoTheStandardsLayout(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange
	dir := testutil.CreateTempDir(t)
	testutil.RunCLIExpectSuccess(t, dir, "init", "--source", "agents", "--skip-content", "demo")

	// Act
	testutil.RunCLIExpectSuccess(t, dir, "add", "rule", "style", "--content", "# Style\n")
	list := testutil.RunCLIExpectSuccess(t, dir, "list", "rules")

	// Assert
	assert.FileExists(t, filepath.Join(dir, ".agents", "rules", "style.md"))
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	list.AssertStdoutContains(t, "style")
}

func TestConvertIntoAgentsKeepsTheStandardsInPlace(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	// Arrange: a project with an AGENTS.md, skills in the standard place and a Cursor rule
	dir := testutil.CreateTempDir(t)
	writeAgentsFile(t, dir, "AGENTS.md", e2eAgentsMD)
	writeAgentsFile(t, dir, ".agents/skills/pr-checklist/SKILL.md", e2eSkill)
	writeAgentsFile(t, dir, "main.go", "package main\n")
	writeAgentsFile(t, dir, ".cursor/rules/style.mdc", "---\nglobs: \"**/*.go\"\ndescription: Go style\n---\nWrap errors.\n")

	// Act
	testutil.RunCLIExpectSuccess(t, dir, "convert", "--into", ".agents", "--write")

	// Assert: the Cursor rule joined the standards' tree, the rest stayed where it was
	assert.FileExists(t, filepath.Join(dir, ".agents", "ai-rulez.toml"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "rules", "style.md"))
	assert.Equal(t, e2eAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
	assert.Equal(t, e2eSkill, readAgentsFile(t, dir, ".agents/skills/pr-checklist/SKILL.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".agents", "context", "agents.md"))
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	assert.Equal(t, 0, testutil.RunCLI(t, dir, "validate").ExitCode)
}

func TestInitRejectsAnUnknownSource(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)

	result := testutil.RunCLIExpectError(t, dir, "init", "--source", "claude")

	result.AssertOutputContains(t, `unknown --source "claude"`)
}
