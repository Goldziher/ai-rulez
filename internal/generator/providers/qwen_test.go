package providers_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// batchAContent is one item of every content kind, shared by the qwen, codebuddy,
// crush, reasonix, replit, goose and zed tests.
func batchAContent() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "be-nice", Path: "/p/.ai-rulez/rules/be-nice.md", Content: "Always be nice."},
			{
				Name: "tsx", Path: "/p/.ai-rulez/rules/tsx.md", Content: "TSX_RULE",
				Metadata: &config.Metadata{Globs: []string{"src/**/*.tsx", "*.ts"}},
			},
		},
		Context: []config.ContentFile{{Name: "layout", Path: "/p/.ai-rulez/context/layout.md", Content: "LAYOUT_CTX"}},
		Skills:  []config.ContentFile{{Name: "demo", Path: "/p/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo things."}},
		Agents: []config.ContentFile{{
			Name: "scout", Path: "/p/.ai-rulez/agents/scout.md", Content: "Scout instructions.",
			Metadata: &config.Metadata{
				Extra: map[string]string{"description": "Fast recon", "model": "fast-model"},
				Tools: []string{"read", "grep"}, Effort: "high",
			},
		}},
		Commands: []config.ContentFile{{
			Name: "ship", Path: "/p/.ai-rulez/commands/ship.md", Content: "Ship it.",
			Metadata: &config.Metadata{Extra: map[string]string{"description": "Ship the change"}},
		}},
	}
}

// batchAConfig carries a local, a streamable-HTTP and an SSE MCP server.
func batchAConfig() *config.Config {
	return &config.Config{
		Name: "demo", Description: "Demo project.", BaseDir: "/p", ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez",
		MCPServers: map[string]*config.MCPServer{
			"local": {Name: "local", Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"K": "v"}},
			"http": {
				Name: "http", Transport: config.TransportHTTP, URL: "https://x.test/mcp",
				Headers: map[string]string{"Authorization": "Bearer t"},
			},
			"sse": {Name: "sse", Transport: config.TransportSSE, URL: "https://x.test/sse"},
		},
	}
}

// batchAGenerate renders the named builtin provider over batchAContent.
func batchAGenerate(t *testing.T, name string, cfg *config.Config) []config.OutputFile {
	t.Helper()
	gen, err := providers.LoadBuiltin(name)
	require.NoError(t, err)
	assert.Equal(t, name, gen.GetName())
	outputs, err := gen.Generate(batchAContent(), "/p", cfg)
	require.NoError(t, err)
	return outputs
}

// batchAMCPServers decodes the member at key of a JSON output.
func batchAMCPServers(t *testing.T, out config.OutputFile, key string) map[string]map[string]any {
	t.Helper()
	var doc map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.Content), &doc))
	return doc[key]
}

// batchAGlobal resolves the spec's user-scope paths under a fixed home.
func batchAGlobal(t *testing.T, name string, env map[string]string) providers.GlobalPaths {
	t.Helper()
	gen, err := providers.LoadBuiltin(name)
	require.NoError(t, err)
	got := gen.Spec.GlobalPaths(filepath.FromSlash("/home/u"), func(k string) string { return env[k] })
	require.NotNil(t, got)
	return *got
}

func batchAJoin(p string) string {
	return filepath.Join(filepath.FromSlash("/home/u"), filepath.FromSlash(p))
}

func TestQwen_Generate(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "qwen", batchAConfig())

	// Every content kind lands where Qwen Code reads it.
	for _, path := range []string{
		"QWEN.md", ".qwen/rules/be-nice.md", ".qwen/rules/tsx.md", ".qwen/skills/demo/SKILL.md",
		".qwen/agents/scout.md", ".qwen/commands/ship.md", ".qwen/settings.json",
	} {
		_, ok := outputByPath(outputs, path)
		assert.True(t, ok, path)
	}

	root, _ := outputByPath(outputs, "QWEN.md")
	assert.Contains(t, root.Content, "LAYOUT_CTX")
	assert.NotContains(t, root.Content, "TSX_RULE", "split mode keeps rules in the rules folder")

	always, _ := outputByPath(outputs, ".qwen/rules/be-nice.md")
	assert.NotContains(t, always.Content, "paths:", "a baseline rule has no frontmatter")
	scoped, _ := outputByPath(outputs, ".qwen/rules/tsx.md")
	assert.Contains(t, scoped.Content, "paths:\n    - src/**/*.tsx\n    - '*.ts'\n")

	agent, _ := outputByPath(outputs, ".qwen/agents/scout.md")
	assert.Equal(t, "scout", frontmatterValue(agent.Content, "name"))
	assert.Equal(t, "Fast recon", frontmatterValue(agent.Content, "description"))
	assert.Equal(t, "fast-model", frontmatterValue(agent.Content, "model"))
	assert.Contains(t, agent.Content, "tools:\n    - read\n    - grep\n")

	skill, _ := outputByPath(outputs, ".qwen/skills/demo/SKILL.md")
	assert.Equal(t, "demo", frontmatterValue(skill.Content, "name"))

	command, _ := outputByPath(outputs, ".qwen/commands/ship.md")
	assert.Equal(t, "Ship the change", frontmatterValue(command.Content, "description"))
	assert.Empty(t, frontmatterValue(command.Content, "name"), "a command takes its name from the file")
}

func TestQwen_InlineModeKeepsScopedRulesNative(t *testing.T) {
	t.Parallel()

	cfg := batchAConfig()
	cfg.Rules = &config.RulesConfig{ModeByPreset: map[string]string{"qwen": config.RulesModeInline}}

	outputs := batchAGenerate(t, "qwen", cfg)

	root, _ := outputByPath(outputs, "QWEN.md")
	assert.Contains(t, root.Content, "Always be nice.")
	_, scoped := outputByPath(outputs, ".qwen/rules/tsx.md")
	assert.True(t, scoped, "a glob-scoped rule keeps its native file in inline mode")
	_, always := outputByPath(outputs, ".qwen/rules/be-nice.md")
	assert.False(t, always)
}

func TestQwen_LocalRoot(t *testing.T) {
	t.Parallel()

	gen, err := providers.LoadBuiltin("qwen")
	require.NoError(t, err)
	assert.Equal(t, ".qwen/QWEN.local.md", gen.Spec.Root.LocalFile)
}

// TestQwen_MCP pins the gemini-style document: httpUrl for streamable HTTP, url for SSE.
func TestQwen_MCP(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "qwen", batchAConfig())
	settings, ok := outputByPath(outputs, ".qwen/settings.json")
	require.True(t, ok)
	servers := batchAMCPServers(t, settings, "mcpServers")

	assert.Equal(t, map[string]any{
		"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "v"},
	}, servers["local"])
	assert.Equal(t, map[string]any{
		"httpUrl": "https://x.test/mcp", "headers": map[string]any{"Authorization": "Bearer t"},
	}, servers["http"])
	assert.Equal(t, map[string]any{"url": "https://x.test/sse"}, servers["sse"])

	cfg := batchAConfig()
	cfg.MCPServers = nil
	outputs = batchAGenerate(t, "qwen", cfg)
	_, ok = outputByPath(outputs, ".qwen/settings.json")
	assert.False(t, ok, "no settings file without servers")
}

func TestQwen_Global(t *testing.T) {
	t.Parallel()

	want := providers.GlobalPaths{
		RootFile: batchAJoin(".qwen/QWEN.md"), SkillsDir: batchAJoin(".qwen/skills"),
		AgentsDir: batchAJoin(".qwen/agents"), CommandsDir: batchAJoin(".qwen/commands"),
		RulesDir: batchAJoin(".qwen/rules"),
		Sidecars: map[string]string{".qwen/settings.json": batchAJoin(".qwen/settings.json")},
	}
	assert.Equal(t, want, batchAGlobal(t, "qwen", nil))

	relocated := batchAGlobal(t, "qwen", map[string]string{"QWEN_HOME": filepath.FromSlash("/data/qwen")})
	assert.Equal(t, filepath.FromSlash("/data/qwen/rules"), relocated.RulesDir)
}
