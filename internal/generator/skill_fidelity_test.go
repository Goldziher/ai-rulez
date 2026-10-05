package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/config"
)

const fidelitySkill = `---
name: demo
description: Use when checking frontmatter passthrough.
license: MIT
compatibility: claude, codex
allowed-tools: Read Grep
disable-model-invocation: true
user-invocable: true
last_verified: 2026-10-01
version: 2
paths:
  - "src/**/*.py"
metadata:
  owner: team-a
  reviewed: {by: alice, date: 2026-10-01}
  tags: [a, b]
---
Body.
`

func fidelityProject(t *testing.T, presets []string, extra, skill string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig(presets, "", extra))
	writeAgentsMDFile(t, root, ".ai-rulez/skills/demo/SKILL.md", skill)
	runAgentsMDGenerate(t, root)
	return root
}

// frontmatterOf parses the YAML frontmatter of a generated file into a map.
func frontmatterOf(t *testing.T, root, rel string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err, rel)
	text := string(data)
	require.True(t, strings.HasPrefix(text, "---\n"), rel)
	end := strings.Index(text[4:], "\n---\n")
	require.GreaterOrEqual(t, end, 0, rel)
	var fm map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(text[4:4+end]), &fm), rel)
	return fm
}

func TestSkillFrontmatter_RoundTripsTypedValuesForClaude(t *testing.T) {
	root := fidelityProject(t, []string{"claude"}, "", fidelitySkill)
	raw, err := os.ReadFile(filepath.Join(root, ".claude/skills/demo/SKILL.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "map[", "no Go map syntax")
	assert.NotContains(t, string(raw), "+0000 UTC", "no Go time syntax")

	want := frontmatterOfString(t, fidelitySkill)
	got := frontmatterOf(t, root, ".claude/skills/demo/SKILL.md")
	for _, key := range []string{"license", "compatibility", "allowed-tools", "disable-model-invocation", "user-invocable", "last_verified", "version", "paths", "metadata"} {
		assert.Equal(t, want[key], got[key], key)
	}
	assert.Equal(t, true, got["disable-model-invocation"], "a boolean stays a boolean")
	assert.Equal(t, true, got["user-invocable"], "the author's value wins")
	assert.Equal(t, 2, got["version"])
	assert.NotContains(t, got, "user_invocable")
}

func frontmatterOfString(t *testing.T, text string) map[string]any {
	t.Helper()
	end := strings.Index(text[4:], "\n---\n")
	var fm map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(text[4:4+end]), &fm))
	return fm
}

func TestSkillFrontmatter_SpecFieldsReachEveryPreset(t *testing.T) {
	presets := map[string]string{
		"codex":       ".agents/skills/demo/SKILL.md",
		"cursor":      ".agents/skills/demo/SKILL.md",
		"copilot":     ".github/skills/demo/SKILL.md",
		"opencode":    ".opencode/skills/demo/SKILL.md",
		"gemini":      ".agents/skills/demo/SKILL.md",
		"devin":       ".devin/skills/demo/SKILL.md",
		"cline":       ".cline/skills/demo/SKILL.md",
		"antigravity": ".agents/skills/demo/SKILL.md",
		"xum":         ".xum/skills/demo/SKILL.md",
	}
	want := frontmatterOfString(t, fidelitySkill)
	for preset, rel := range presets {
		t.Run(preset, func(t *testing.T) {
			root := fidelityProject(t, []string{preset}, "", fidelitySkill)
			got := frontmatterOf(t, root, rel)
			for _, key := range []string{"license", "compatibility", "allowed-tools", "metadata"} {
				assert.Equal(t, want[key], got[key], key)
			}
			assert.Equal(t, "demo", got["name"])
			assert.NotContains(t, got, "last_verified", "vendor-only keys are not spread to other tools")
		})
	}
}

func TestSkillFrontmatter_CursorPathsAndInvocation(t *testing.T) {
	root := fidelityProject(t, []string{"cursor"}, "", fidelitySkill)
	got := frontmatterOf(t, root, ".agents/skills/demo/SKILL.md")
	assert.Equal(t, []any{"src/**/*.py"}, got["paths"])
	assert.Equal(t, true, got["disable-model-invocation"])

	root = fidelityProject(t, []string{"codex"}, "", fidelitySkill)
	got = frontmatterOf(t, root, ".agents/skills/demo/SKILL.md")
	assert.Equal(t, []any{"src/**/*.py"}, got["paths"], "the shared tree carries the union of every writer's keys")
	assert.Equal(t, got, frontmatterOf(t, fidelityProject(t, []string{"cursor", "codex", "pi"}, "", fidelitySkill),
		".agents/skills/demo/SKILL.md"), "enabling more writers does not change the file")
	policy, err := os.ReadFile(filepath.Join(root, ".agents/skills/demo/agents/openai.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "policy:\n  allow_implicit_invocation: false\n", string(policy))
}

func TestSkillFrontmatter_CodexMergesShortDescriptionIntoMetadata(t *testing.T) {
	skill := "---\nname: demo\ndescription: d\nshort-description: Short\nmetadata:\n  owner: team-a\n---\nBody\n"
	root := fidelityProject(t, []string{"codex"}, "", skill)
	got := frontmatterOf(t, root, ".agents/skills/demo/SKILL.md")
	assert.Equal(t, map[string]any{"owner": "team-a", "short-description": "Short"}, got["metadata"])
	assert.NotContains(t, got, "short-description")
}

func TestClaudeSkills_UserInvocableDefaultAndOptIn(t *testing.T) {
	plain := "---\nname: demo\ndescription: d\n---\nBody\n"

	root := fidelityProject(t, []string{"claude"}, "", plain)
	got := frontmatterOf(t, root, ".claude/skills/demo/SKILL.md")
	assert.NotContains(t, got, "user-invocable", "skills stay in the / menu by default")
	assert.NotContains(t, got, "user_invocable")

	hide := "\n[claude.skills]\nhide_from_menu = true\n"
	root = fidelityProject(t, []string{"claude"}, hide, plain)
	got = frontmatterOf(t, root, ".claude/skills/demo/SKILL.md")
	assert.Equal(t, false, got["user-invocable"])

	authored := "---\nname: demo\ndescription: d\nuser-invocable: yes\n---\nBody\n"
	root = fidelityProject(t, []string{"claude"}, hide, authored)
	got = frontmatterOf(t, root, ".claude/skills/demo/SKILL.md")
	assert.Equal(t, true, got["user-invocable"], "an author-set value beats the option")
}

func TestClaudeCommands_AuthorInvocationOverridesConstant(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/commands/go.md", "---\ndescription: d\nuser-invocable: false\n---\nBody\n")
	writeAgentsMDFile(t, root, ".ai-rulez/commands/run.md", "---\ndescription: d\n---\nBody\n")
	runAgentsMDGenerate(t, root)
	assert.Equal(t, false, frontmatterOf(t, root, ".claude/skills/go/SKILL.md")["user-invocable"])
	assert.Equal(t, true, frontmatterOf(t, root, ".claude/skills/run/SKILL.md")["user-invocable"])
}

func TestClaudeSkills_PathsAreEmitted(t *testing.T) {
	skill := "---\nname: demo\ndescription: d\nglobs: \"src/**/*.go, lib/**\"\n---\nBody\n"
	root := fidelityProject(t, []string{"claude"}, "", skill)
	got := frontmatterOf(t, root, ".claude/skills/demo/SKILL.md")
	assert.Equal(t, []any{"src/**/*.go", "lib/**"}, got["paths"])
}

func TestSkillFrontmatter_DeterministicAcrossRuns(t *testing.T) {
	first := fidelityProject(t, []string{"claude", "codex", "cursor"}, "", fidelitySkill)
	second := fidelityProject(t, []string{"claude", "codex", "cursor"}, "", fidelitySkill)
	for _, rel := range []string{".claude/skills/demo/SKILL.md", ".agents/skills/demo/SKILL.md"} {
		a, err := os.ReadFile(filepath.Join(first, rel))
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(second, rel))
		require.NoError(t, err)
		assert.Equal(t, string(a), string(b), rel)
	}
}

func TestCodexProjectDocSizeWarning(t *testing.T) {
	big := strings.Repeat("Lorem ipsum dolor sit amet. ", 400)
	build := func(t *testing.T, extra string) ([]sizeFinding, *config.Config) {
		t.Helper()
		root := t.TempDir()
		writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", extra))
		for _, name := range []string{"big-a", "big-b", "big-c", "small"} {
			body := big
			if name == "small" {
				body = "tiny"
			}
			writeAgentsMDFile(t, root, ".ai-rulez/rules/"+name+".md", "# "+name+"\n\n"+body+"\n")
		}
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		g := NewGenerator(cfg)
		outputs, _, err := g.collectOutputs("")
		require.NoError(t, err)
		return g.instructionSizeFindings(outputs), cfg
	}

	t.Run("over the default limit", func(t *testing.T) {
		findings, _ := build(t, "")
		require.Len(t, findings, 1)
		f := findings[0]
		assert.Equal(t, "AGENTS.md", f.Path)
		assert.Equal(t, 32*1024, f.Limit)
		assert.Greater(t, f.Bytes, f.Limit)
		require.NotEmpty(t, f.Contributors)
		assert.Contains(t, f.Contributors[0].Label, "big-")
		assert.Contains(t, f.message(), "exceeds")
	})

	t.Run("raised limit", func(t *testing.T) {
		findings, _ := build(t, "\n[codex]\nproject_doc_max_bytes = 1048576\n")
		assert.Empty(t, findings)
	})

	t.Run("disabled", func(t *testing.T) {
		findings, _ := build(t, "\n[codex]\nproject_doc_max_bytes = 0\n")
		assert.Empty(t, findings)
	})
}

func TestCodexProjectDocSizeWarning_OnlyWithCodex(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"opencode"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/big.md", "# big\n\n"+strings.Repeat("x", 40000)+"\n")
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	g := NewGenerator(cfg)
	outputs, _, err := g.collectOutputs("")
	require.NoError(t, err)
	assert.Empty(t, g.instructionSizeFindings(outputs))
}

func TestSectionContributors(t *testing.T) {
	md := "intro\n\n## Rules\n\n### small\nx\n\n### large\n" + strings.Repeat("y", 200) + "\n\n```\n### not a heading\n```\n"
	got := sectionContributors(md)
	require.NotEmpty(t, got)
	assert.Equal(t, "Rules > large", got[0].Label)
}
