package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

const servedSkillsConfig = "version = \"5.0\"\nname = \"served\"\npresets = [\"claude\", \"codex\", \"cline\"]\n" +
	"gitignore = false\n\n[skills]\ndelivery = \"served\"\n"

func servedSkillsRepo(t *testing.T, extra string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), servedSkillsConfig+extra)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "r.md"), "---\npriority: high\n---\nA rule\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "lint", "SKILL.md"), "---\nname: lint\ndescription: Lint things\n---\nBody\n")
	chdir(t, root)
	progress.SetQuiet(true)
	t.Cleanup(func() {
		progress.SetQuiet(false)
		pluginMode, dryRun, profile, validateRecursive = false, false, "", false
	})
	pluginMode, dryRun, profile = false, false, ""
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// Served skills are announced in every instruction file of a harness that can
// call MCP, with the on-disk fallback (#359).
func TestSkillsInstructions_WrittenIntoInstructionFiles(t *testing.T) {
	root := servedSkillsRepo(t, "")
	require.Equal(t, 0, runRecursiveGenerate())

	body := readFile(t, filepath.Join(root, "AGENTS.md"))
	assert.Contains(t, body, "## Unlisted skills")
	assert.Contains(t, body, "`find_skill`")
	assert.Contains(t, body, "`load_skill`")
	assert.Contains(t, body, "`ai-rulez` MCP server")
	assert.Contains(t, body, "`.ai-rulez/skills/<name>/SKILL.md`")
	assert.Contains(t, readFile(t, filepath.Join(root, "CLAUDE.md")), "@AGENTS.md", "CLAUDE.md imports the shared file")
	assert.Equal(t, 0, runDriftCheck(true), "the section is part of what --check renders")
}

// With agents_md off each MCP-capable harness's own root file carries the section.
func TestSkillsInstructions_PerHarnessFiles(t *testing.T) {
	root := servedSkillsRepo(t, "")
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		strings.Replace(servedSkillsConfig, "gitignore = false\n", "gitignore = false\nagents_md = false\n", 1))
	require.Equal(t, 0, runRecursiveGenerate())

	assert.Contains(t, readFile(t, filepath.Join(root, "CLAUDE.md")), "## Unlisted skills")
	assert.Contains(t, readFile(t, filepath.Join(root, "AGENTS.md")), "## Unlisted skills")
	assert.Equal(t, 0, runDriftCheck(true))
}

func TestSkillsInstructions_OptOutAndCustomText(t *testing.T) {
	t.Run("opt out", func(t *testing.T) {
		root := servedSkillsRepo(t, "\n[mcp]\nskills_instructions = false\n")
		require.Equal(t, 0, runRecursiveGenerate())
		assert.NotContains(t, readFile(t, filepath.Join(root, "AGENTS.md")), "Unlisted skills")
		assert.NotContains(t, readFile(t, filepath.Join(root, "CLAUDE.md")), "find_skill")
	})
	t.Run("custom text", func(t *testing.T) {
		root := servedSkillsRepo(t, "\n[mcp]\nskills_instructions_text = \"Skills: ask {{server}} (files in {{skills_dir}}).\"\n")
		require.Equal(t, 0, runRecursiveGenerate())
		body := readFile(t, filepath.Join(root, "AGENTS.md"))
		assert.Contains(t, body, "Skills: ask ai-rulez (files in .ai-rulez/skills).")
		assert.NotContains(t, body, "Unlisted skills")
	})
}

// [delivery.stub] customises the generated stub skill (#362), and the freshness
// check follows an edit to the file behind body_file.
func TestDeliveryStub_CustomStubIsGenerated(t *testing.T) {
	root := servedSkillsRepo(t, "\n[delivery.stub]\ndescription = \"Team skills.\"\nbody_file = \"stub.md\"\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "stub.md"), "Ask {{server}}: find_skill, then load_skill.\n")
	require.Equal(t, 0, runRecursiveGenerate())

	stub := readFile(t, filepath.Join(root, ".claude", "skills", "dynamic-skills", "SKILL.md"))
	assert.Contains(t, stub, "description: Team skills.")
	assert.Contains(t, stub, "Ask ai-rulez: find_skill, then load_skill.")
	assert.NotContains(t, stub, "are available on demand")
	assert.Equal(t, 0, runDriftCheck(true))

	writeFile(t, filepath.Join(root, ".ai-rulez", "stub.md"), "Ask {{server}} again: find_skill, load_skill.\n")
	assert.Equal(t, exitDrift, runDriftCheck(true), "an edit to body_file is drift")
	require.Equal(t, 0, runRecursiveGenerate())
	assert.Contains(t, readFile(t, filepath.Join(root, ".claude", "skills", "dynamic-skills", "SKILL.md")), "Ask ai-rulez again")
}

func TestSkillsInstructions_AbsentWhenNothingIsServed(t *testing.T) {
	root := servedSkillsRepo(t, "")
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"static\"\npresets = [\"claude\"]\ngitignore = false\n")
	require.Equal(t, 0, runRecursiveGenerate())
	assert.NotContains(t, readFile(t, filepath.Join(root, "CLAUDE.md")), "Unlisted skills")
}
