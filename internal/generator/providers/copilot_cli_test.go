package providers_test

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func copilotCLIGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("copilot-cli")
	require.NoError(t, err)
	return gen
}

// copilotCLIContent covers every activation mode, context, skills (with a
// resource-less body) and agents with and without the optional frontmatter.
func copilotCLIContent() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "always-on", Content: "ALWAYS_BODY", Metadata: &config.Metadata{Priority: "high"}},
			{Name: "go-only", Content: "GLOB_BODY", Metadata: &config.Metadata{Globs: []string{"**/*.go", "cmd/**"}}},
			{Name: "auto-rule", Content: "AUTO_BODY", Metadata: &config.Metadata{Activation: "auto", Extra: map[string]string{"description": "when X"}}},
			{Name: "manual-rule", Content: "MANUAL_BODY", Metadata: &config.Metadata{Activation: "manual"}},
		},
		Context: []config.ContentFile{
			{Name: "layout", Content: "CONTEXT_BODY"},
			{Name: "go-ctx", Content: "GLOB_CONTEXT", Metadata: &config.Metadata{Globs: []string{"internal/**"}}},
		},
		Skills: []config.ContentFile{
			{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo things.",
				Metadata: &config.Metadata{Extra: map[string]string{"description": "Demo: a \"quoted\" skill"}}},
			{Name: "bare", Path: "/test/.ai-rulez/skills/bare/SKILL.md", Content: "Bare skill."},
		},
		Agents: []config.ContentFile{
			{Name: "scout", Content: "Scout instructions.", Metadata: &config.Metadata{
				Tools: []string{"read", "grep"},
				Extra: map[string]string{"description": "Fast recon", "target": "github-copilot", "user-invocable": "true"},
			}},
			{Name: "plain", Content: "Plain agent."},
		},
	}
}

// shared returns the outputs both presets may write: everything under .github
// except the MCP document, which only copilot-cli writes (to .github/mcp.json).
func sharedCopilotOutputs(outputs []config.OutputFile, base string) map[string]string {
	shared := map[string]string{}
	for _, o := range outputs {
		if o.IsDir {
			continue
		}
		rel, err := filepath.Rel(base, o.Path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".github/") && rel != ".github/mcp.json" {
			shared[rel] = o.Content
		}
	}
	return shared
}

func TestCopilotCLI_LoadBuiltin(t *testing.T) {
	t.Parallel()

	gen := copilotCLIGen(t)
	assert.Equal(t, "copilot-cli", gen.GetName())

	paths := gen.GetOutputPaths("/repo")
	assert.Contains(t, paths, filepath.Join("/repo", ".github", "copilot-instructions.md"))
	assert.Contains(t, paths, filepath.Join("/repo", ".github", "agents"))
	assert.Contains(t, paths, filepath.Join("/repo", ".github", "skills"))
}

// TestCopilotCLI_Generate pins the paths and frontmatter of the native outputs.
func TestCopilotCLI_Generate(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	cfg := &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeSplit}}
	outputs, err := copilotCLIGen(t).Generate(copilotCLIContent(), baseDir, cfg)
	require.NoError(t, err)

	root := requireFile(t, outputs, ".github/copilot-instructions.md")
	assert.Contains(t, root.Content, "AUTO_BODY", "auto rules have no applyTo, so they stay inline")
	assert.Contains(t, root.Content, "MANUAL_BODY")
	assert.NotContains(t, root.Content, "GLOB_BODY", "a scoped rule moves to .github/instructions")

	always := requireFile(t, outputs, ".github/instructions/always-on.instructions.md")
	assert.Contains(t, always.Content, "ALWAYS_BODY", "split mode writes the always-on rule to a file")

	scoped := requireFile(t, outputs, ".github/instructions/go-only.instructions.md")
	assert.Contains(t, scoped.Content, "applyTo:")
	assert.Contains(t, scoped.Content, "GLOB_BODY")

	skill := requireFile(t, outputs, ".github/skills/demo/SKILL.md")
	assert.Equal(t, "demo", frontmatterValue(skill.Content, "name"))
	assert.Contains(t, skill.Content, "Do demo things.")

	agent := requireFile(t, outputs, ".github/agents/scout.agent.md")
	assert.Equal(t, "scout", frontmatterValue(agent.Content, "name"))
	assert.Equal(t, "Fast recon", frontmatterValue(agent.Content, "description"))
	assert.Equal(t, "github-copilot", frontmatterValue(agent.Content, "target"))
	assert.Contains(t, agent.Content, "- read")
	assert.True(t, hasOutputPathSuffix(outputs, ".github/agents/plain.agent.md"))
}

// TestCopilotCLI_MatchesCopilotPreset enforces the dedup contract: the generator
// keeps a single copy of a path two presets write, so every path both emit must
// hold the same bytes. A divergence would let the preset sorted last silently
// replace the other's file.
func TestCopilotCLI_MatchesCopilotPreset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  func(baseDir string) *config.Config
	}{
		{"split rules", func(b string) *config.Config {
			return &config.Config{Name: "demo", Description: "Demo project.", BaseDir: b, Rules: &config.RulesConfig{Mode: config.RulesModeSplit}}
		}},
		{"inline rules", func(b string) *config.Config {
			return &config.Config{Name: "demo", Description: "Demo project.", BaseDir: b, Rules: &config.RulesConfig{Mode: config.RulesModeInline}}
		}},
		{"detailed header", func(b string) *config.Config {
			return &config.Config{Name: "demo", BaseDir: b, Header: &config.HeaderConfig{Style: "detailed"}}
		}},
		{"agents_md split", func(b string) *config.Config {
			return &config.Config{Name: "demo", Description: "Demo project.", BaseDir: b, AgentsMD: true,
				Rules: &config.RulesConfig{Mode: config.RulesModeSplit}}
		}},
		{"agents_md inline", func(b string) *config.Config {
			return &config.Config{Name: "demo", Description: "Demo project.", BaseDir: b, AgentsMD: true,
				Rules: &config.RulesConfig{Mode: config.RulesModeInline}}
		}},
		{"compact", func(b string) *config.Config {
			compact := true
			return &config.Config{Name: "demo", BaseDir: b, Compact: &compact}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			baseDir := t.TempDir()
			legacy, err := (&presets.CopilotPresetGenerator{}).Generate(copilotCLIContent(), baseDir, tt.cfg(baseDir))
			require.NoError(t, err)
			dsl, err := copilotCLIGen(t).Generate(copilotCLIContent(), baseDir, tt.cfg(baseDir))
			require.NoError(t, err)

			// Act
			want, got := sharedCopilotOutputs(legacy, baseDir), sharedCopilotOutputs(dsl, baseDir)

			// Assert
			wantPaths, gotPaths := keys(want), keys(got)
			assert.Equal(t, wantPaths, gotPaths, "both presets must write the same shared paths")
			for _, p := range wantPaths {
				assert.Equal(t, want[p], got[p], "%s must render identically in both presets", p)
			}
		})
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestCopilotCLI_MCPJSON covers .github/mcp.json: an explicit type on every
// entry under mcpServers, with disabled servers left out.
func TestCopilotCLI_MCPJSON(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Name: "demo", MCPServers: map[string]*config.MCPServer{
		"filesystem": {Name: "filesystem", Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"FOO": "BAR"}},
		"docs": {
			Name: "docs", Transport: config.TransportHTTP, URL: "https://example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer token"},
		},
	}}
	outputs, err := copilotCLIGen(t).Generate(&config.ContentTree{}, "/test", cfg)
	require.NoError(t, err)

	mcp := requireFile(t, outputs, ".github/mcp.json")
	var doc map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(mcp.Content), &doc))
	servers := doc["mcpServers"]
	require.Len(t, servers, 2)
	assert.Equal(t, "stdio", servers["filesystem"]["type"])
	assert.Equal(t, "npx", servers["filesystem"]["command"])
	assert.Equal(t, "http", servers["docs"]["type"])
	assert.Equal(t, "https://example.com/mcp", servers["docs"]["url"])

	outputs, err = copilotCLIGen(t).Generate(&config.ContentTree{}, "/test", &config.Config{Name: "demo"})
	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, ".github/mcp.json"), "no servers, no document")
}

func TestCopilotCLI_GlobalPaths(t *testing.T) {
	t.Parallel()

	g := copilotCLIGen(t).Spec.GlobalPaths("/home/u", func(string) string { return "" })
	require.NotNil(t, g)
	assert.Equal(t, filepath.FromSlash("/home/u/.copilot/copilot-instructions.md"), g.RootFile)
	assert.Equal(t, filepath.FromSlash("/home/u/.copilot/skills"), g.SkillsDir)
	assert.Equal(t, filepath.FromSlash("/home/u/.copilot/agents"), g.AgentsDir)
	assert.Equal(t, filepath.FromSlash("/home/u/.copilot/instructions"), g.RulesDir)
	assert.Equal(t, filepath.FromSlash("/home/u/.copilot/mcp-config.json"), g.Sidecars[".github/mcp.json"])
}

// TestCopilotCLI_ExtensionsValidate pins the loader checks of the generic DSL
// options copilot-cli relies on.
func TestCopilotCLI_ExtensionsValidate(t *testing.T) {
	t.Parallel()

	base := `
name = "x"
[root]
file = "AGENTS.md"
sections = ["rules_inline"]
`
	tests := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{"inline_unscoped without split", "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\ninline_unscoped = true\n",
			"inline_unscoped"},
		{"join_lists without lists", "[outputs.agents]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\n" +
			"[outputs.agents.body]\nsections = [\"frontmatter\"]\n[outputs.agents.frontmatter]\njoin_lists = true\n", "join_lists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := providers.LoadProviderSpec([]byte(base+tt.extra), "x.toml", providers.FormatTOML)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestCopilotCLI_InlineUnscopedKeepsAutoAndManualInline covers the generic
// inline_unscoped option in the inline rules mode as well: auto and manual items
// never become files Copilot would not load.
func TestCopilotCLI_InlineUnscopedKeepsAutoAndManualInline(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	cfg := &config.Config{Name: "demo", BaseDir: baseDir, Rules: &config.RulesConfig{Mode: config.RulesModeInline}}
	outputs, err := copilotCLIGen(t).Generate(copilotCLIContent(), baseDir, cfg)
	require.NoError(t, err)

	root := requireFile(t, outputs, ".github/copilot-instructions.md")
	for _, body := range []string{"ALWAYS_BODY", "AUTO_BODY", "MANUAL_BODY", "CONTEXT_BODY"} {
		assert.Contains(t, root.Content, body)
	}
	assert.False(t, hasOutputPathSuffix(outputs, "auto-rule.instructions.md"))
	assert.False(t, hasOutputPathSuffix(outputs, "manual-rule.instructions.md"))
	assert.True(t, hasOutputPathSuffix(outputs, "go-only.instructions.md"), "scoped rules keep their file")
}
