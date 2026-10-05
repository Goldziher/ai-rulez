package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const passthroughAgent = `---
name: reviewer
description: Reviews code.
tools: Read, Grep
disallowedTools: [Write, Edit]
permissionMode: acceptEdits
memory: project
maxTurns: 12
background: true
isolation: worktree
color: blue
initialPrompt: Start by listing changes.
mcpServers:
  github:
    type: http
    url: https://example.com/mcp
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: ./check.sh
---
Review carefully.
`

func agentProject(t *testing.T, presets []string, agent string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig(presets, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/agents/reviewer.md", agent)
	runAgentsMDGenerate(t, root)
	return root
}

func TestAgentFrontmatter_ClaudePassthroughKeepsTypes(t *testing.T) {
	root := agentProject(t, []string{"claude"}, passthroughAgent)
	want := frontmatterOfString(t, passthroughAgent)
	got := frontmatterOf(t, root, ".claude/agents/reviewer.md")
	for _, key := range []string{"disallowedTools", "permissionMode", "memory", "maxTurns", "background", "isolation", "color", "initialPrompt", "mcpServers", "hooks"} {
		assert.Equal(t, want[key], got[key], key)
	}
	assert.Equal(t, true, got["background"])
	assert.Equal(t, 12, got["maxTurns"])
}

func TestAgentFrontmatter_ClaudeOnlyKeysStayOutOfOtherHarnesses(t *testing.T) {
	root := agentProject(t, []string{"claude", "gemini", "opencode", "codex"}, passthroughAgent)
	for _, rel := range []string{".gemini/agents/reviewer.md", ".opencode/agents/reviewer.md", ".codex/agents/reviewer.toml"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		for _, key := range []string{"disallowedTools", "permissionMode", "initialPrompt", "mcpServers"} {
			assert.NotContains(t, string(data), key, rel)
		}
	}
}
