package mcp

// End-to-end checks of what `ai-rulez mcp --serve-skills` offers: the surface
// follows the tools the server registered, a directory with no configuration
// gets a readable catalog, and the [mcp] table pins the settings.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initializeResult runs the handshake and returns the initialize result.
func (p *serveProc) initializeResult() map[string]any {
	p.t.Helper()
	resp := p.call("initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "e2e", "version": "1"},
	})
	require.Nil(p.t, resp["error"], "initialize: %v", resp)
	p.notify("notifications/initialized")
	result, _ := resp["result"].(map[string]any)
	require.NotNil(p.t, result)
	return result
}

func (p *serveProc) names(method, key string) []string {
	p.t.Helper()
	resp := p.call(method, map[string]any{})
	result, _ := resp["result"].(map[string]any)
	items, _ := result[key].([]any)
	var out []string
	for _, item := range items {
		name, _ := item.(map[string]any)["name"].(string)
		out = append(out, name)
	}
	return out
}

func TestServeSkills_ReadOnlyServerDoesNotPointAtWritingToolsOrPrompts(t *testing.T) {
	skipShort(t)
	cases := []struct {
		name    string
		config  string
		prompts []string
	}{
		// The default read-only skills surface registers only the skill tools, so
		// the review prompt, which needs the authoring read tools, is not offered.
		{"skill tools only", serveBaseConfig, nil},
		{"with authoring", serveBaseConfig + "\n[mcp]\nauthoring = true\n", []string{"review-config"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := serveProject(t, tc.config, map[string]string{
				"skills/kit/SKILL.md": serveSkill("kit", "A served kit", "kit body", "delivery: served\n"),
			})
			p := startServe(t, root, "--read-only", "--no-watch")

			init := p.initializeResult()

			instructions, _ := init["instructions"].(string)
			assert.Contains(t, instructions, "find_skill")
			assert.NotContains(t, instructions, "CRUD")
			assert.NotContains(t, instructions, "generate_outputs")
			assert.NotContains(t, instructions, "--read-only", "the server is already read-only")
			assert.Equal(t, tc.prompts, p.names("prompts/list", "prompts"))
		})
	}
}

func TestServeSkills_WithoutAConfigurationEveryListedResourceCanBeRead(t *testing.T) {
	skipShort(t)
	root := filepath.Join(t.TempDir(), "proj")
	skill := filepath.Join(root, "skills", "demo", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(skill), 0o755))
	require.NoError(t, os.WriteFile(skill, []byte(serveSkill("demo", "A demo skill of the repository", "demo body.", "")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Root\n\nBe terse.\n"), 0o644))
	p := startServe(t, root, "--discover-skills", "--no-watch")
	p.initialize()

	resp := p.call("resources/list", map[string]any{})
	result, _ := resp["result"].(map[string]any)
	listed, _ := result["resources"].([]any)
	require.NotEmpty(t, listed)
	var catalog bool
	for _, r := range listed {
		uri, _ := r.(map[string]any)["uri"].(string)
		assert.NotEqual(t, "ai-rulez://config", uri, "a configuration resource needs a configuration")
		read := p.call("resources/read", map[string]any{"uri": uri})
		assert.Nil(t, read["error"], "%s is listed, so it can be read: %v", uri, read)
		catalog = catalog || uri == "ai-rulez://catalog"
	}
	assert.True(t, catalog, "the catalog is built from what was discovered")
	for _, name := range p.names("tools/list", "tools") {
		assert.NotContains(t, []string{"list_skills", "list_context", "read_config", "validate_config"}, name)
	}
}

func TestServeSkills_ToolsAndReadOnlyFromTheConfiguration(t *testing.T) {
	skipShort(t)
	root := serveProject(t, serveBaseConfig+"\n[mcp]\ntools = [\"find_skill\", \"load_skill\"]\n", map[string]string{
		"skills/kit/SKILL.md": serveSkill("kit", "A served kit", "kit body", "delivery: served\n"),
	})
	p := startServe(t, root, "--no-watch")

	init := p.initializeResult()

	assert.ElementsMatch(t, []string{"find_skill", "load_skill"}, p.names("tools/list", "tools"))
	assert.Empty(t, p.names("prompts/list", "prompts"))
	instructions, _ := init["instructions"].(string)
	assert.NotContains(t, instructions, "CRUD")
	assert.Contains(t, p.load("kit"), "kit body")
}

func TestServeSkills_ToolsFlagOverridesTheConfiguration(t *testing.T) {
	skipShort(t)
	root := serveProject(t, serveBaseConfig+"\n[mcp]\ntools = [\"find_skill\", \"load_skill\"]\n", nil)
	p := startServe(t, root, "--tools", "get_version", "--no-watch")
	p.initialize()

	assert.Equal(t, []string{"get_version"}, p.names("tools/list", "tools"))
}

func TestServeSkills_UnknownToolStopsTheServerAndListsTheValidOnes(t *testing.T) {
	skipShort(t)
	root := serveProject(t, serveBaseConfig, nil)
	p := startServe(t, root, "--tools", "find_skil", "--no-watch")

	p.waitExit(t)

	assert.Contains(t, p.stderrText(), "find_skil")
	assert.Contains(t, p.stderrText(), "find_skill")
}
