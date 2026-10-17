package generator

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	matrixCustomPreset = "doctool"
	matrixCustomSpec   = `name = "doctool"

directories = [".doc", ".doc/rules", ".doc/skills", ".doc/agents", ".doc/commands"]

[root]
file = "DOC.md"
local_file = ".doc/DOC.local.md"
sections = ["header", "title", "description", "rules_inline", "context_inline", "agents_delegation"]

[outputs.rules]
mode = "per_item_file"
dir = ".doc/rules"
filename = "{id}.md"

[outputs.skills]
mode = "per_item_file"
dir = ".doc/skills"
filename = "{id}/SKILL.md"
resources = true

[outputs.skills.body]
sections = ["frontmatter", "content", "resource_index"]

[outputs.skills.frontmatter]
fields = ["description"]

[outputs.agents]
mode = "per_item_file"
dir = ".doc/agents"
filename = "{id}.md"

[outputs.agents.body]
sections = ["frontmatter", "content"]

[outputs.agents.frontmatter]
fields = ["description"]

[outputs.commands]
mode = "per_item_file"
dir = ".doc/commands"
filename = "{id}.md"

[outputs.commands.body]
sections = ["frontmatter", "content"]

[outputs.commands.frontmatter]
fields = ["description"]
omit_name = true
`
)

// matrixPresets are the preset classes of the matrix: one per vendor shape, a
// custom declarative provider and baz.
func matrixPresets() []string {
	return []string{"claude", "codex", "gemini", "cursor", "copilot", matrixCustomPreset, "baz"}
}

// matrixValues are the skip_outputs values.
func matrixValues() []string {
	return []string{"instructions", "agents_md", "claude_md", "rules", "skills", "agents", "commands"}
}

// skipClassesOf names, from the path alone, every skip_outputs value that
// excludes a generated output. Settings, MCP and config files belong to none.
func skipClassesOf(rel string) []string {
	switch {
	case rel == "AGENTS.md" || rel == "AGENTS.override.md":
		return []string{"instructions", "agents_md"}
	case rel == "CLAUDE.md":
		return []string{"instructions", "claude_md"}
	case rel == "GEMINI.md" || rel == "DOC.md" || rel == ".github/copilot-instructions.md":
		return []string{"instructions"}
	case strings.HasPrefix(rel, ".agents/skills/deploy/"):
		return []string{"agents_md", "commands"} // the fixture's command, written as a skill
	case strings.HasPrefix(rel, ".agents/skills/"):
		return []string{"agents_md", "skills"}
	case strings.Contains(rel, "/skills/deploy/"):
		return []string{"commands"}
	case strings.Contains(rel, "/skills/"):
		return []string{"skills"}
	case strings.Contains(rel, "/rules/") || strings.HasPrefix(rel, ".github/instructions/"):
		return []string{"rules"}
	case strings.Contains(rel, "/commands/") || strings.HasPrefix(rel, ".github/prompts/"):
		return []string{"commands"}
	case strings.Contains(rel, "/agents/"):
		return []string{"agents"}
	}
	return nil
}

func matrixSkipped(rel, value string) bool {
	for _, class := range skipClassesOf(rel) {
		if class == value {
			return true
		}
	}
	return false
}

// matrixProject writes a project with one preset, the shared fixture, a command
// and an MCP server. skip is the TOML line for skip_outputs ("" for none).
func matrixProject(t *testing.T, preset, agentsMD, skip string) string {
	t.Helper()
	root := t.TempDir()
	presetEntry := `"` + preset + `"`
	if preset == matrixCustomPreset {
		presetEntry = `{ name = "doctool", provider = ".ai-rulez/providers/doctool.toml" }`
	}
	cfg := "version = \"5.0\"\nname = \"matrix\"\ngitignore = false\npresets = [" + presetEntry + "]\n" +
		"agents_md = " + agentsMD + "\n" + skip + skipMCPTable
	writeAgentsMDProject(t, root, cfg)
	writeAgentsMDFile(t, root, ".ai-rulez/commands/deploy.md", "---\ndescription: Deploy\n---\nDEPLOY_BODY\n")
	if preset == matrixCustomPreset {
		writeAgentsMDFile(t, root, ".ai-rulez/providers/doctool.toml", matrixCustomSpec)
	}
	return root
}

// generatedSet is the sorted list of files below root, the source tree excluded.
func generatedSet(t *testing.T, root string) []string {
	t.Helper()
	paths := agentsMDPaths(t, root)
	if paths == nil {
		return []string{}
	}
	return paths
}

func TestSkipOutputs_MatrixWritesTheUnskippedPathsAndLeavesHandWrittenOnesAlone(t *testing.T) {
	for _, preset := range matrixPresets() {
		for _, agentsMD := range []string{"true", "false"} {
			for _, value := range matrixValues() {
				t.Run(preset+"/agents_md="+agentsMD+"/"+value, func(t *testing.T) {
					// Arrange: hand-written files sit at every path the value skips
					quietWarnings(t)
					baseline := matrixProject(t, preset, agentsMD, "")
					runAgentsMDGenerate(t, baseline)
					skipped, kept := []string{}, []string{}
					for _, rel := range generatedSet(t, baseline) {
						if matrixSkipped(rel, value) {
							skipped = append(skipped, rel)
						} else {
							kept = append(kept, rel)
						}
					}
					root := matrixProject(t, preset, agentsMD, "skip_outputs = [\""+value+"\"]\n")
					for _, rel := range skipped {
						writeAgentsMDFile(t, root, rel, "HAND "+rel+"\n")
					}

					// Act
					lines, err := skipGenerator(t, root).DryRun("")
					require.NoError(t, err)
					dryRunTree := generatedSet(t, root)
					runAgentsMDGenerate(t, root)
					afterGenerate := generatedSet(t, root)
					drift, driftErr := skipGenerator(t, root).CheckDrift("")
					_, cleanErr := skipGenerator(t, root).Clean("", CleanOptions{})

					// Assert: --dry-run wrote nothing and names no skipped path
					require.NoError(t, driftErr)
					require.NoError(t, cleanErr)
					assert.Equal(t, skipped, dryRunTree, "dry-run wrote nothing")
					for _, rel := range skipped {
						for _, line := range lines {
							assert.NotContains(t, line, rel+" ", "the plan lists a skipped path: "+line)
						}
					}
					// generate wrote the rest and left the hand-written files alone
					assert.ElementsMatch(t, append(append([]string{}, skipped...), kept...), afterGenerate)
					assert.Empty(t, drift)
					// clean removed what it wrote and kept the hand-written files
					left := generatedSet(t, root)
					sort.Strings(skipped)
					assert.Subset(t, left, skipped)
					for _, rel := range skipped {
						assert.Equal(t, "HAND "+rel+"\n", readAgentsMDFile(t, root, rel), "after clean: "+rel)
					}
					for _, rel := range kept {
						assert.NotContains(t, left, rel, "clean removed the generated file")
					}
				})
			}
		}
	}
}

func TestSkipOutputs_MatrixOracleCoversEveryValue(t *testing.T) {
	// Arrange
	paths := []string{
		"AGENTS.md", "CLAUDE.md", ".agents/skills/a/SKILL.md", ".claude/skills/a/SKILL.md",
		".claude/rules/a.md", ".claude/commands/a.md", ".claude/agents/a.md",
	}
	seen := map[string]bool{}

	// Act
	for _, rel := range paths {
		for _, class := range skipClassesOf(rel) {
			seen[class] = true
		}
	}

	// Assert
	for _, value := range matrixValues() {
		assert.True(t, seen[value], value)
	}
}
