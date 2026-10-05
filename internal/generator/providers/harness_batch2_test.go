package providers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// batch2Content is one rule of each activation, a skill, an agent and a command.
func batch2Content() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "always-rule", Content: "ALWAYS_BODY"},
			{Name: "tsx-rule", Content: "TSX_BODY", Metadata: &config.Metadata{Globs: []string{"**/*.tsx"}}},
		},
		Skills: []config.ContentFile{
			{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "Skill body.",
				Metadata: &config.Metadata{Extra: map[string]string{"description": "Demo skill"}}},
		},
		Agents: []config.ContentFile{
			{Name: "scout", Content: "Scout body.",
				Metadata: &config.Metadata{Extra: map[string]string{"description": "Recon agent"}, Tools: []string{"read"}}},
		},
		Commands: []config.ContentFile{
			{Name: "ship", Content: "Ship body.", Metadata: &config.Metadata{Extra: map[string]string{"description": "Ship it"}}},
		},
	}
}

func batch2Generate(t *testing.T, name string, cfg *config.Config) []config.OutputFile {
	t.Helper()
	gen, err := providers.LoadBuiltin(name)
	require.NoError(t, err)
	base := t.TempDir()
	if cfg == nil {
		cfg = &config.Config{Name: "demo"}
	}
	cfg.BaseDir = base
	outputs, err := gen.Generate(batch2Content(), base, cfg)
	require.NoError(t, err)
	return outputs
}

func TestBatch2_ContentLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset   string
		rules    string // rules folder ("" = rules stay inline)
		skills   string
		agents   string // "" = no agent output
		commands string
		agentFM  map[string]string
	}{
		{"kilo", ".kilo/rules", ".kilo/skills", ".kilo/agents", ".kilo/commands", map[string]string{"mode": "subagent", "description": "Recon agent"}},
		{"mimocode", "", ".mimocode/skills", ".mimocode/agents", ".mimocode/commands", map[string]string{"mode": "subagent", "description": "Recon agent"}},
		{"qoder", ".qoder/rules", ".qoder/skills", ".qoder/agents", ".qoder/commands", map[string]string{"name": "scout", "description": "Recon agent"}},
		{"bob", ".bob/rules", ".bob/skills", "", ".bob/commands", nil},
		{"grok", ".grok/rules", ".grok/skills", ".grok/agents", ".grok/commands", map[string]string{"name": "scout", "description": "Recon agent"}},
		{"codewhale", ".codewhale/rules", ".codewhale/skills", "", ".codewhale/commands", nil},
		{"zcode", "", ".zcode/skills", ".zcode/agents", ".zcode/commands", map[string]string{"name": "scout", "description": "Recon agent"}},
		{"commandcode", "", ".commandcode/skills", ".commandcode/agents", ".commandcode/commands", map[string]string{"name": "scout", "description": "Recon agent"}},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()
			outputs := batch2Generate(t, tt.preset, nil)

			root := requireFile(t, outputs, "AGENTS.md")
			skill := requireFile(t, outputs, tt.skills+"/demo/SKILL.md")
			assert.Equal(t, "demo", frontmatterValue(skill.Content, "name"))
			assert.Equal(t, "Demo skill", frontmatterValue(skill.Content, "description"))

			cmd := requireFile(t, outputs, tt.commands+"/ship.md")
			assert.Equal(t, "Ship it", frontmatterValue(cmd.Content, "description"))
			assert.Empty(t, frontmatterValue(cmd.Content, "name"), "command name comes from the file name")
			assert.Contains(t, cmd.Content, "Ship body.")

			if tt.agents == "" {
				assert.False(t, hasOutputPathContains(outputs, "/agents/scout"), "no native agent files")
			} else {
				agent := requireFile(t, outputs, tt.agents+"/scout.md")
				for k, v := range tt.agentFM {
					assert.Equal(t, v, frontmatterValue(agent.Content, k), k)
				}
				assert.Contains(t, agent.Content, "Scout body.")
			}

			if tt.rules == "" {
				assert.False(t, hasOutputPathContains(outputs, "/rules/always-rule"))
				assert.Contains(t, root.Content, "ALWAYS_BODY")
				assert.Contains(t, root.Content, "TSX_BODY")
			} else {
				// Rules go to the folder in split mode; plain dialects keep scoped ones inline.
				require.True(t, hasOutputPathSuffix(outputs, tt.rules+"/always-rule.md"))
				scoped := requireFile(t, outputs, tt.rules+"/tsx-rule.md")
				assert.Contains(t, scoped.Content, "TSX_BODY")
				assert.NotContains(t, root.Content, "TSX_BODY")
			}
		})
	}
}

func TestBatch2_QoderRuleTriggers(t *testing.T) {
	t.Parallel()

	outputs := batch2Generate(t, "qoder", nil)
	assert.Equal(t, "always_on", frontmatterValue(requireFile(t, outputs, ".qoder/rules/always-rule.md").Content, "trigger"))

	scoped := requireFile(t, outputs, ".qoder/rules/tsx-rule.md")
	assert.Equal(t, "glob", frontmatterValue(scoped.Content, "trigger"))
	assert.Contains(t, scoped.Content, "**/*.tsx")
	assert.Contains(t, scoped.Content, "TSX_BODY")
}

var disabled = false

func batch2Servers() map[string]*config.MCPServer {
	return map[string]*config.MCPServer{
		"fs":   {Name: "fs", Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"K": "V"}},
		"docs": {Name: "docs", Transport: config.TransportHTTP, URL: "https://example.com/mcp", Headers: map[string]string{"A": "B"}},
	}
}

func TestBatch2_MCPDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset string
		path   string
		key    []string
		stdio  map[string]any
		remote map[string]any
	}{
		{"kilo", "kilo.jsonc", []string{"mcp"},
			map[string]any{"type": "local", "command": []any{"npx", "-y", "pkg"}, "environment": map[string]any{"K": "V"}, "enabled": true},
			map[string]any{"type": "remote", "url": "https://example.com/mcp", "headers": map[string]any{"A": "B"}, "enabled": true}},
		{"mimocode", ".mimocode/mimocode.jsonc", []string{"mcp"},
			map[string]any{"type": "local", "command": []any{"npx", "-y", "pkg"}, "environment": map[string]any{"K": "V"}, "enabled": true},
			map[string]any{"type": "remote", "url": "https://example.com/mcp", "headers": map[string]any{"A": "B"}, "enabled": true}},
		{"qoder", ".mcp.json", []string{"mcpServers"},
			map[string]any{"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "V"}, "disabled": false},
			map[string]any{"type": "http", "url": "https://example.com/mcp", "headers": map[string]any{"A": "B"}, "disabled": false}},
		{"commandcode", ".mcp.json", []string{"mcpServers"},
			map[string]any{"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "V"}, "disabled": false},
			map[string]any{"type": "http", "url": "https://example.com/mcp", "headers": map[string]any{"A": "B"}, "disabled": false}},
		{"bob", ".bob/mcp.json", []string{"mcpServers"},
			map[string]any{"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "V"}},
			map[string]any{"type": "streamable-http", "url": "https://example.com/mcp", "headers": map[string]any{"A": "B"}}},
		{"codewhale", ".codewhale/mcp.json", []string{"servers"},
			map[string]any{"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "V"}},
			map[string]any{"url": "https://example.com/mcp", "headers": map[string]any{"A": "B"}}},
		{"zcode", ".zcode/config.json", []string{"mcp", "servers"},
			map[string]any{"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "V"}},
			map[string]any{"type": "http", "url": "https://example.com/mcp", "headers": map[string]any{"A": "B"}}},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()
			outputs := batch2Generate(t, tt.preset, &config.Config{Name: "demo", MCPServers: batch2Servers()})
			doc := requireFile(t, outputs, tt.path)

			var tree map[string]any
			require.NoError(t, json.Unmarshal([]byte(doc.Content), &tree))
			var servers any = tree
			for _, k := range tt.key {
				servers = servers.(map[string]any)[k]
			}
			assert.Equal(t, tt.stdio, servers.(map[string]any)["fs"])
			assert.Equal(t, tt.remote, servers.(map[string]any)["docs"])
		})
	}
}

func TestBatch2_MCPExtras(t *testing.T) {
	t.Parallel()

	t.Run("grok toml", func(t *testing.T) {
		t.Parallel()
		servers := batch2Servers()
		servers["off"] = &config.MCPServer{Name: "off", Command: "x", Enabled: &disabled}
		outputs := batch2Generate(t, "grok", &config.Config{Name: "demo", MCPServers: servers})
		doc := requireFile(t, outputs, ".grok/config.toml").Content
		assert.Contains(t, doc, "[mcp_servers.fs]")
		assert.Contains(t, doc, `command = "npx"`)
		assert.Contains(t, doc, "[mcp_servers.docs]")
		assert.Contains(t, doc, `url = "https://example.com/mcp"`)
		assert.Contains(t, doc, "enabled = false")
		assert.NotContains(t, doc, "disabled")
	})

	t.Run("zcode disabled server uses enable false", func(t *testing.T) {
		t.Parallel()
		outputs := batch2Generate(t, "zcode", &config.Config{Name: "demo", MCPServers: map[string]*config.MCPServer{
			"off": {Name: "off", Command: "x", Enabled: &disabled},
		}})
		doc := requireFile(t, outputs, ".zcode/config.json").Content
		assert.Contains(t, doc, `"enable": false`)
		assert.NotContains(t, doc, "disabled")
	})

	t.Run("no mcp document without servers", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ preset, path string }{
			{"mimocode", ".mimocode/mimocode.jsonc"}, {"qoder", ".mcp.json"}, {"bob", ".bob/mcp.json"},
			{"grok", ".grok/config.toml"}, {"codewhale", ".codewhale/mcp.json"}, {"zcode", ".zcode/config.json"},
			{"commandcode", ".mcp.json"},
		} {
			outputs := batch2Generate(t, tc.preset, nil)
			assert.False(t, hasOutputPathSuffix(outputs, tc.path), "%s writes no MCP document without servers", tc.preset)
		}
	})
}

// TestKilo_InstructionsRegistersRulesFolder covers the elements extension: the
// .kilo/rules folder is only loaded when listed in kilo.jsonc `instructions`.
func TestKilo_InstructionsRegistersRulesFolder(t *testing.T) {
	t.Parallel()

	t.Run("written without MCP servers and no empty mcp key", func(t *testing.T) {
		t.Parallel()
		outputs := batch2Generate(t, "kilo", nil)
		var tree map[string]any
		require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, "kilo.jsonc").Content), &tree))
		assert.Equal(t, []any{".kilo/rules/*.md"}, tree["instructions"])
		assert.NotContains(t, tree, "mcp")
	})

	t.Run("the project-relative glob is left out of the user scope", func(t *testing.T) {
		t.Parallel()
		gen, err := providers.LoadBuiltin("kilo")
		require.NoError(t, err)
		cfg := &config.Config{Name: "demo", BaseDir: t.TempDir(), UserScope: true, MCPServers: batch2Servers()}
		outputs, err := gen.Generate(&config.ContentTree{}, cfg.BaseDir, cfg)
		require.NoError(t, err)
		var tree map[string]any
		require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, "kilo.jsonc").Content), &tree))
		assert.NotContains(t, tree, "instructions")
		assert.Contains(t, tree, "mcp")
	})

	t.Run("keeps user entries, comments and is idempotent", func(t *testing.T) {
		t.Parallel()
		gen, err := providers.LoadBuiltin("kilo")
		require.NoError(t, err)
		base := t.TempDir()
		path := filepath.Join(base, "kilo.jsonc")
		require.NoError(t, os.WriteFile(path,
			[]byte("{\n  // mine\n  \"instructions\": [\"docs/*.md\"],\n  \"model\": \"x\"\n}\n"), 0o644))

		cfg := &config.Config{Name: "demo", BaseDir: base, MCPServers: batch2Servers()}
		outputs, err := gen.Generate(&config.ContentTree{}, base, cfg)
		require.NoError(t, err)
		doc := requireFile(t, outputs, "kilo.jsonc")
		assert.Contains(t, doc.Content, "// mine")
		assert.Contains(t, doc.Content, `"docs/*.md"`)
		assert.Contains(t, doc.Content, `".kilo/rules/*.md"`)
		assert.Contains(t, doc.Content, `"model"`)
		assert.True(t, doc.PartiallyOwned)
		require.NoError(t, os.WriteFile(path, []byte(doc.Content), 0o644))

		again, err := gen.Generate(&config.ContentTree{}, base, cfg)
		require.NoError(t, err)
		assert.Equal(t, doc.Content, requireFile(t, again, "kilo.jsonc").Content)
	})

	t.Run("a non-array instructions value is left alone", func(t *testing.T) {
		t.Parallel()
		gen, err := providers.LoadBuiltin("kilo")
		require.NoError(t, err)
		base := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(base, "kilo.jsonc"), []byte(`{"instructions":"x.md"}`), 0o644))
		outputs, err := gen.Generate(&config.ContentTree{}, base, &config.Config{Name: "demo", BaseDir: base})
		require.NoError(t, err)
		assert.Contains(t, requireFile(t, outputs, "kilo.jsonc").Content, `"instructions": "x.md"`)
	})
}

func TestBatch2_GlobalPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset  string
		root    string
		skills  string
		agents  string
		cmds    string
		rules   string
		sidecar [2]string // project path, user path
	}{
		{"kilo", ".config/kilo/AGENTS.md", ".kilo/skills", ".config/kilo/agents", ".config/kilo/commands", ".kilo/rules", [2]string{"kilo.jsonc", ".config/kilo/kilo.jsonc"}},
		{"mimocode", ".config/mimocode/AGENTS.md", ".config/mimocode/skills", ".config/mimocode/agents", ".config/mimocode/commands", "", [2]string{".mimocode/mimocode.jsonc", ".config/mimocode/mimocode.jsonc"}},
		{"qoder", ".qoder/AGENTS.md", ".qoder/skills", ".qoder/agents", ".qoder/commands", ".qoder/rules", [2]string{".mcp.json", ".qoder/settings.json"}},
		{"bob", ".bob/AGENTS.md", ".bob/skills", "", ".bob/commands", ".bob/rules", [2]string{".bob/mcp.json", ".bob/settings/mcp.json"}},
		{"grok", ".grok/AGENTS.md", ".grok/skills", ".grok/agents", ".grok/commands", ".grok/rules", [2]string{".grok/config.toml", ".grok/config.toml"}},
		{"codewhale", ".codewhale/AGENTS.md", ".codewhale/skills", "", ".codewhale/commands", "", [2]string{".codewhale/mcp.json", ".codewhale/mcp.json"}},
		{"zcode", ".zcode/AGENTS.md", ".zcode/skills", ".zcode/agents", ".zcode/commands", "", [2]string{".zcode/config.json", ".zcode/cli/config.json"}},
		{"commandcode", ".commandcode/AGENTS.md", ".commandcode/skills", ".commandcode/agents", ".commandcode/commands", "", [2]string{".mcp.json", ".commandcode/mcp.json"}},
	}
	home := "/home/u"
	join := func(rel string) string {
		if rel == "" {
			return ""
		}
		return filepath.Join(home, filepath.FromSlash(rel))
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)
			g := gen.Spec.GlobalPaths(home, func(string) string { return "" })
			require.NotNil(t, g)
			assert.Equal(t, join(tt.root), g.RootFile)
			assert.Equal(t, join(tt.skills), g.SkillsDir)
			assert.Equal(t, join(tt.agents), g.AgentsDir)
			assert.Equal(t, join(tt.cmds), g.CommandsDir)
			assert.Equal(t, join(tt.rules), g.RulesDir)
			assert.Equal(t, join(tt.sidecar[1]), g.Sidecars[tt.sidecar[0]])
		})
	}
}

func TestElementsSpec_Validation(t *testing.T) {
	t.Parallel()

	base := "name = \"x\"\ndisplay_name = \"X\"\n[root]\nfile = \"AGENTS.md\"\nsections = [\"header\"]\n[[sidecars]]\n"
	tests := []struct {
		name    string
		sidecar string
		wantErr string
	}{
		{"valid", "kind = \"mcp\"\npath = \"a.jsonc\"\n[sidecars.elements]\nkey = [\"instructions\"]\nvalues = [\"a\"]\n", ""},
		{"toml document", "kind = \"mcp\"\npath = \"a.toml\"\n[sidecars.elements]\nkey = [\"i\"]\nvalues = [\"a\"]\n", "only valid on a json or jsonc"},
		{"missing values", "kind = \"mcp\"\npath = \"a.json\"\n[sidecars.elements]\nkey = [\"i\"]\nvalues = []\n", "at least one value"},
		{"tool specific kind", "kind = \"pi_mcp_json\"\npath = \"a.json\"\n[sidecars.elements]\nkey = [\"i\"]\nvalues = [\"a\"]\n", "only valid on the generic kinds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := providers.LoadProviderSpec([]byte(base+tt.sidecar), "x.toml", providers.FormatTOML)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
