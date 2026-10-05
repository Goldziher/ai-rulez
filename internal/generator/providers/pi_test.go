package providers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// piGen loads the embedded "pi" provider. Fails fast if the spec stops parsing.
func piGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("pi")
	require.NoError(t, err)
	return gen
}

// TestPi_LoadBuiltin pins the registered name and the directories the stale-file
// cleanup needs to see.
func TestPi_LoadBuiltin(t *testing.T) {
	t.Parallel()

	gen := piGen(t)
	assert.Equal(t, "pi", gen.GetName())

	paths := gen.GetOutputPaths("/repo")
	assert.Contains(t, paths, filepath.Join("/repo", "AGENTS.md"))
	assert.Contains(t, paths, filepath.Join("/repo", ".pi"))
	assert.Contains(t, paths, filepath.Join("/repo", ".pi", "agents"))
}

// TestPi_Generate covers the three faces of the pi preset: an inline-only
// AGENTS.md (no rules folder), skills under the pi-preferred .agents/skills, and
// subagent files under .pi/agents with the subagents frontmatter shape.
func TestPi_Generate(t *testing.T) {
	t.Parallel()

	gen := piGen(t)
	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "be-nice", Content: "Always be nice.", Metadata: &config.Metadata{Priority: "high"}},
			// A path-scoped rule has no native rules folder to land in, so pi
			// inlines it (unlike claude/antigravity which write a rule file).
			{Name: "tsx", Content: "TSX_RULE", Metadata: &config.Metadata{Globs: []string{"**/*.tsx"}}},
		},
		Context: []config.ContentFile{{Name: "layout", Content: "Use the existing module layout."}},
		Skills: []config.ContentFile{
			{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo things."},
		},
		Agents: []config.ContentFile{
			{
				Name:    "scout",
				Content: "Scout instructions here.",
				Metadata: &config.Metadata{
					Extra:  map[string]string{"description": "Fast codebase recon"},
					Tools:  []string{"read", "grep"},
					Effort: "high",
				},
			},
		},
	}
	cfg := &config.Config{Name: "demo", Description: "Demo project.", BaseDir: "/test"}

	outputs, err := gen.Generate(content, "/test", cfg)
	require.NoError(t, err)

	agentsMD := requireFile(t, outputs, "AGENTS.md")
	assert.Contains(t, agentsMD.Content, "## Rules")
	assert.Contains(t, agentsMD.Content, "Always be nice.")
	assert.Contains(t, agentsMD.Content, "TSX_RULE", "pi has no rules folder, so scoped rules inline")
	assert.Contains(t, agentsMD.Content, "## Context")
	assert.Contains(t, agentsMD.Content, "Use the existing module layout.")

	// Skills go to .agents/skills, not .pi/skills.
	_, ok := findOutput(outputs, ".agents/skills/demo/SKILL.md")
	assert.True(t, ok, "skill must land in .agents/skills")
	_, ok = findOutput(outputs, ".pi/skills/demo/SKILL.md")
	assert.False(t, ok, "pi must not write skills to .pi/skills")

	agentFile := requireFile(t, outputs, ".pi/agents/scout.md")
	assert.Equal(t, "scout", frontmatterValue(agentFile.Content, "name"))
	assert.Equal(t, "Fast codebase recon", frontmatterValue(agentFile.Content, "description"))
	assert.Equal(t, "high", frontmatterValue(agentFile.Content, "thinking"), "effort maps to the thinking key")
	assert.Contains(t, agentFile.Content, "tools:")
	assert.Contains(t, agentFile.Content, "read")
	assert.Contains(t, agentFile.Content, "Scout instructions here.")
}

// TestPi_MCPJSON covers .pi/mcp.json: the stdio command/args/env form and the
// remote url/headers/description form, in the `mcpServers` object, emitted only
// when servers are configured, and merged into a hand-authored document.
func TestPi_MCPJSON(t *testing.T) {
	t.Parallel()

	t.Run("a disabled server is not written as an active one", func(t *testing.T) {
		t.Parallel()
		off := false
		cfg := &config.Config{Name: "demo", MCPServers: map[string]*config.MCPServer{
			"on":  {Name: "on", Command: "npx"},
			"off": {Name: "off", Command: "npx", Enabled: &off},
		}}
		outputs, err := piGen(t).Generate(&config.ContentTree{}, "/test", cfg)
		require.NoError(t, err)
		var doc map[string]map[string]any
		require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, ".pi/mcp.json").Content), &doc))
		assert.Contains(t, doc["mcpServers"], "on")
		assert.NotContains(t, doc["mcpServers"], "off")
	})

	t.Run("stdio and remote entries", func(t *testing.T) {
		t.Parallel()
		gen := piGen(t)
		cfg := &config.Config{
			Name: "demo",
			MCPServers: map[string]*config.MCPServer{
				"filesystem": {
					Name: "filesystem", Command: "npx",
					Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
					Env:  map[string]string{"FOO": "BAR"},
				},
				"docs": {
					Name: "docs", Transport: config.TransportHTTP, URL: "https://example.com/mcp",
					Headers:     map[string]string{"Authorization": "Bearer token"},
					Description: "Search and read the product documentation",
				},
			},
		}
		outputs, err := gen.Generate(&config.ContentTree{}, "/test", cfg)
		require.NoError(t, err)

		mcp := requireFile(t, outputs, ".pi/mcp.json")
		var doc map[string]map[string]any
		require.NoError(t, json.Unmarshal([]byte(mcp.Content), &doc))
		servers := doc["mcpServers"]
		require.Len(t, servers, 2)

		fs, ok := servers["filesystem"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "npx", fs["command"])
		assert.Equal(t, []any{"-y", "@modelcontextprotocol/server-filesystem", "."}, fs["args"])
		assert.Equal(t, map[string]any{"FOO": "BAR"}, fs["env"])

		docs, ok := servers["docs"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "https://example.com/mcp", docs["url"])
		assert.Equal(t, "Bearer token", docs["headers"].(map[string]any)["Authorization"])
		assert.Equal(t, "Search and read the product documentation", docs["description"])
	})

	t.Run("no file without servers", func(t *testing.T) {
		t.Parallel()
		outputs, err := piGen(t).Generate(&config.ContentTree{}, "/test", &config.Config{Name: "demo"})
		require.NoError(t, err)
		assert.False(t, hasOutputPathSuffix(outputs, ".pi/mcp.json"))
	})

	t.Run("merges into a hand-authored document", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		path := filepath.Join(baseDir, ".pi", "mcp.json")
		require.NoError(t, mkdirAndWrite(path, `{"theme":"dark","mcpServers":{"mine":{"url":"https://mine"}}}`))

		cfg := &config.Config{Name: "demo", MCPServers: map[string]*config.MCPServer{
			"shared": {Command: "npx", Args: []string{"-y", "pkg"}},
		}}
		outputs, err := piGen(t).Generate(&config.ContentTree{}, baseDir, cfg)
		require.NoError(t, err)

		mcp := requireFile(t, outputs, ".pi/mcp.json")
		assert.Contains(t, mcp.Content, `"theme"`, "user key is preserved")
		assert.Contains(t, mcp.Content, `"mine"`, "hand-written server survives")
		assert.Contains(t, mcp.Content, `"shared"`, "configured server is written")
	})
}

func mkdirAndWrite(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// TestPi_AgentsMDShared verifies pi renders byte-for-byte the same AGENTS.md as
// the other presets that share the file (codex, opencode, xum, amp): the shared
// document must not depend on which preset writes it last.
func TestPi_AgentsMDShared(t *testing.T) {
	t.Parallel()

	gens := presetGenerators(t)
	var first, firstName string
	for _, name := range []string{"codex", "opencode", "xum", "amp", "pi"} {
		baseDir := t.TempDir()
		cfg := &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeInline}}
		outputs, err := gens[name].Generate(targetedContent(), baseDir, cfg)
		require.NoError(t, err)
		doc := rootFile(t, outputs, "AGENTS.md")
		if first == "" {
			first, firstName = doc, name
			continue
		}
		assert.Equal(t, first, doc, "AGENTS.md from %s differs from %s", name, firstName)
	}
}

// TestPi_AgentsMDSkillsLocation pins the issue requirement: with agents_md on,
// the shared .agents/skills tree replaces the pi skills output entirely, so no
// skill is ever written under .pi.
func TestPi_AgentsMDSkillsLocation(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	content := &config.ContentTree{
		Skills: []config.ContentFile{{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "x"}},
	}
	cfg := &config.Config{Name: "demo", BaseDir: baseDir, AgentsMD: true}
	outputs, err := piGen(t).Generate(content, baseDir, cfg)
	require.NoError(t, err)

	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), "/.pi/skills/") {
			t.Errorf("pi must not write skills under .pi with agents_md on: %s", o.Path)
		}
	}
}
