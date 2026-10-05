package providers_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadSpec(t *testing.T, toml string) *providers.Generator {
	t.Helper()
	spec, err := providers.LoadProviderSpec([]byte(toml), "spec.toml", providers.FormatAuto)
	require.NoError(t, err)
	return providers.New(spec)
}

func oneServer() *config.Config {
	return &config.Config{
		Name: "t", BaseDir: "/",
		MCPServers: map[string]*config.MCPServer{"srv": {Name: "srv", Command: "npx", Args: []string{"-y", "srv"}}},
	}
}

// TestGenericMCPSidecar_MergesIntoExistingDocument checks, per dialect, that the
// owned member is written and every hand-authored sibling survives.
func TestGenericMCPSidecar_MergesIntoExistingDocument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sidecar  string
		path     string
		existing string
		wantKeys []string // top-level keys, in document order
		check    func(t *testing.T, doc string)
	}{
		{
			name:     "zed context_servers",
			sidecar:  `kind = "mcp"` + "\n" + `dialect = "zed"` + "\n" + `path = ".zed/settings.json"`,
			path:     ".zed/settings.json",
			existing: `{"theme": "One Dark", "context_servers": {"mine": {"command": "mine"}}}`,
			wantKeys: []string{"theme", "context_servers"},
			check: func(t *testing.T, doc string) {
				assert.Contains(t, doc, `"theme": "One Dark"`)
				assert.Contains(t, doc, `"mine"`)
				assert.Contains(t, doc, `"srv"`)
			},
		},
		{
			name:     "amp flat dotted key is one literal member",
			sidecar:  `kind = "mcp"` + "\n" + `dialect = "amp"` + "\n" + `path = ".amp/settings.json"`,
			path:     ".amp/settings.json",
			existing: `{"amp.anthropic.effort": "high"}`,
			wantKeys: []string{"amp.anthropic.effort", "amp.mcpServers"},
			check: func(t *testing.T, doc string) {
				assert.Contains(t, doc, `"amp.anthropic.effort": "high"`)
				assert.Contains(t, doc, `"command": "npx"`)
			},
		},
		{
			name:     "opencode nested under mcp keeps siblings",
			sidecar:  `kind = "mcp"` + "\n" + `dialect = "opencode"` + "\n" + `path = "opencode.json"`,
			path:     "opencode.json",
			existing: `{"$schema": "https://opencode.ai/config.json", "mcp": {"keep": {"type": "local", "command": ["x"]}}}`,
			wantKeys: []string{"$schema", "mcp"},
			check: func(t *testing.T, doc string) {
				assert.Contains(t, doc, `"keep"`)
				assert.Contains(t, doc, `"type": "local"`)
			},
		},
		{
			name:     "explicit nested key",
			sidecar:  `kind = "mcp"` + "\n" + `path = "cfg.json"` + "\n" + `key = ["tools", "mcp"]`,
			path:     "cfg.json",
			existing: `{"tools": {"other": true}}`,
			wantKeys: []string{"tools"},
			check: func(t *testing.T, doc string) {
				assert.Contains(t, doc, `"other": true`)
				assert.Contains(t, doc, `"srv"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			baseDir := writeFixture(t, tt.path, tt.existing)
			gen := loadSpec(t, "name = \"tool\"\n[[sidecars]]\n"+tt.sidecar+"\n")
			cfg := oneServer()
			cfg.BaseDir = baseDir

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, baseDir, cfg)

			// Assert
			require.NoError(t, err)
			out := requireFile(t, outputs, tt.path)
			var keys []string
			for _, m := range jsonMembers(t, out.Content) {
				keys = append(keys, m.key)
			}
			assert.Equal(t, tt.wantKeys, keys)
			assert.True(t, out.PartiallyOwned, "a document with consumer keys is partially owned")
			assert.NotEmpty(t, out.MergeClaims)
			tt.check(t, out.Content)
		})
	}
}

func TestGenericMCPSidecar_FreshDocumentIsWhollyOwned(t *testing.T) {
	t.Parallel()

	// Arrange
	gen := loadSpec(t, "name = \"tool\"\n[[sidecars]]\nkind = \"mcp\"\ndialect = \"vscode\"\npath = \".vscode/mcp.json\"\n")
	baseDir := t.TempDir()
	cfg := oneServer()
	cfg.BaseDir = baseDir

	// Act
	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, cfg)

	// Assert
	require.NoError(t, err)
	out := requireFile(t, outputs, ".vscode/mcp.json")
	assert.False(t, out.PartiallyOwned)
	assert.JSONEq(t, `{"servers": {"srv": {"type": "stdio", "command": "npx", "args": ["-y", "srv"]}}}`, out.Content)
}

func TestGenericMCPSidecar_NoServersEmitsNothing(t *testing.T) {
	t.Parallel()

	gen := loadSpec(t, "name = \"tool\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"mcp.json\"\n")

	outputs, err := gen.Generate(&config.ContentTree{}, t.TempDir(), &config.Config{Name: "t"})

	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, "mcp.json"), "emit_when defaults to has_mcp_servers")
}

// TestHooksSidecar_NeedsADialect pins that a hooks sidecar without the name of a
// harness fails at load time, not at generation.
func TestHooksSidecar_NeedsADialect(t *testing.T) {
	t.Parallel()

	// Act
	_, err := providers.LoadProviderSpec([]byte("name = \"tool\"\n[[sidecars]]\nkind = \"hooks\"\npath = \"p.json\"\nemit_when = \"always\"\n"),
		"spec.toml", providers.FormatAuto)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs the name of a harness with hook support")
}

// TestGenericMCPSidecar_MergesTOMLAndYAML checks that TOML and YAML documents are
// merged like JSON ones: the owned member is written, comments and siblings stay.
func TestGenericMCPSidecar_MergesTOMLAndYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, sidecar, path, existing string
		want                          []string
	}{
		{
			"toml codex", "kind = \"mcp\"\ndialect = \"codex\"\npath = \".codex/config.toml\"", ".codex/config.toml",
			"# mine\nmodel = \"x\"\n", []string{"# mine", `model = "x"`, "[mcp_servers.srv]", `command = "npx"`},
		},
		{
			"yaml standard", "kind = \"mcp\"\ndialect = \"yaml-standard\"\npath = \".poolside/settings.yaml\"", ".poolside/settings.yaml",
			"# mine\nmodel: x\n", []string{"# mine", "model: x", "mcp_servers:", "srv:", "command: npx"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			baseDir := writeFixture(t, tt.path, tt.existing)
			gen := loadSpec(t, "name = \"tool\"\n[[sidecars]]\n"+tt.sidecar+"\n")
			cfg := oneServer()
			cfg.BaseDir = baseDir

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, baseDir, cfg)

			// Assert
			require.NoError(t, err)
			out := requireFile(t, outputs, tt.path)
			for _, want := range tt.want {
				assert.Contains(t, out.Content, want)
			}
			assert.True(t, out.PartiallyOwned)
		})
	}
}

func TestSidecarDocFormat(t *testing.T) {
	t.Parallel()

	tests := []struct{ path, format, want string }{
		{"a.json", "", "json"}, {"a.jsonc", "", "jsonc"}, {"a.toml", "", "toml"},
		{"a.yaml", "", "yaml"}, {"a.yml", "", "yaml"}, {"A.JSON", "", "json"},
		{"a.cfg", "", ""}, {"a.cfg", "toml", "toml"}, {"a.json", "yaml", "yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.path+"/"+tt.format, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, (&providers.SidecarSpec{Path: tt.path, Format: tt.format}).DocFormat())
		})
	}
}

// TestMergedSidecarPaths_IncludesGenericKinds makes sure unmerge, clean and
// gitignore see the generic merged sidecars like the tool-specific ones.
func TestMergedSidecarPaths_IncludesGenericKinds(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"mcp", "permissions", "hooks", "mcp_json", "claude_settings_json"} {
		assert.True(t, providers.SidecarIsMergedDocument(kind), kind)
	}
	assert.False(t, providers.SidecarIsMergedDocument("claude_plugins_json"))
	assert.Contains(t, providers.MergedSidecarPaths(), ".claude/settings.json")

	docs := providers.MergedSidecarDocs()
	require.NotEmpty(t, docs)
	for _, d := range docs {
		if d.Path == ".mcp.json" {
			assert.Equal(t, "json", d.Format)
			return
		}
	}
	t.Fatal(".mcp.json missing from MergedSidecarDocs")
}

func TestLoadProviderSpec_Validation(t *testing.T) {
	t.Parallel()

	rulesBase := "name = \"t\"\n[root]\nfile = \"AGENTS.md\"\nsections = [\"rules_inline\"]\n"
	tests := []struct {
		name    string
		toml    string
		wantErr string
	}{
		{"unknown sidecar format", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"a.json\"\nformat = \"ini\"", `sidecars[0].format: unknown format "ini"`},
		{"format not inferable", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"a.cfg\"", "cannot infer it from the extension"},
		{"unknown dialect", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"a.json\"\ndialect = \"nope\"", `unknown mcp dialect "nope"`},
		{"mcp dialect on permissions", "name = \"t\"\n[[sidecars]]\nkind = \"permissions\"\npath = \"a.json\"\ndialect = \"standard\"", "needs a permissions dialect"},
		{"user_only without global_path", "name = \"t\"\n[[sidecars]]\nkind = \"permissions\"\npath = \"a.json\"\ndialect = \"zed\"\nuser_only = true", "user_only needs a global_path"},
		{"empty key segment", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"a.json\"\nkey = [\"a\", \"\"]", "segments must not be empty"},
		{"generic fields on a legacy kind", "name = \"t\"\n[[sidecars]]\nkind = \"mcp_json\"\npath = \".mcp.json\"\nformat = \"json\"", "only valid on the generic kinds"},
		{"global_path escapes", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"a.json\"\nglobal_path = \"../x.json\"", "global_path"},
		{"unknown output type", "name = \"t\"\n[outputs.nope]\nmode = \"per_item_file\"", "unknown content type"},
		{"aggregate on skills", "name = \"t\"\n[outputs.skills]\nmode = \"aggregate\"\nfile = \"a.md\"", `only valid on outputs.checks`},
		{"aggregate needs file", "name = \"t\"\n[outputs.checks]\nmode = \"aggregate\"", "outputs[\"checks\"].file is required"},
		{"aggregate rejects dir", "name = \"t\"\n[outputs.checks]\nmode = \"aggregate\"\nfile = \"a.md\"\ndir = \"x\"", "takes only file"},
		{"file with per_item_file", "name = \"t\"\n[outputs.checks]\nmode = \"per_item_file\"\nfile = \"a.md\"", "only valid with mode"},
		{"unknown mode", "name = \"t\"\n[outputs.checks]\nmode = \"bulk\"", `unknown mode "bulk"`},
		{"mapped dialect without activation", rulesBase + "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\nsplit = true\ndialect = \"mapped\"", "requires an [outputs.rules.activation]"},
		{"activation with another dialect", rulesBase + "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\nsplit = true\ndialect = \"cursor\"\n[outputs.rules.activation]\nalways = {a = \"b\"}", "needs dialect \"mapped\""},
		{"activation without split", rulesBase + "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\n[outputs.rules.activation]\nalways = {a = \"b\"}", "require split = true"},
		{"activation on skills", "name = \"t\"\n[outputs.skills]\nmode = \"per_item_file\"\n[outputs.skills.activation]\nalways = {a = \"b\"}", "only valid on outputs.rules"},
		{"activation bad format", rulesBase + "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\nsplit = true\n[outputs.rules.activation]\nformat = \"xml\"", "activation.format"},
		{"activation list value", rulesBase + "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\nsplit = true\n[outputs.rules.activation]\nalways = {a = [\"b\"]}", "must be a string, boolean or number"},
		{"home_env without home_dir", "name = \"t\"\n[global]\nhome_env = \"X_HOME\"", "set together"},
		{"bad home_env", "name = \"t\"\n[global]\nhome_env = \"1X\"\nhome_dir = \".x\"", "not a valid environment variable name"},
		{"global path escapes", "name = \"t\"\n[global]\nroot_file = \"../AGENTS.md\"", "global.root_file"},
		{"global absolute", "name = \"t\"\n[global]\nskills_dir = \"/etc/skills\"", "global.skills_dir"},
		{"global path outside home_dir", "name = \"t\"\n[global]\nhome_env = \"X_HOME\"\nhome_dir = \".x\"\nroot_file = \".y/AGENTS.md\"", "must be inside global.home_dir"},
		{"unknown global field", "name = \"t\"\n[global]\nnope = \"x\"", "strict mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			_, err := providers.LoadProviderSpec([]byte(tt.toml), "spec.toml", providers.FormatAuto)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestGlobalPaths(t *testing.T) {
	t.Parallel()

	spec := loadSpec(t, `
name = "hermes-like"
[global]
home_env = "TOOL_HOME"
home_dir = ".tool"
root_file = ".tool/RULES.md"
skills_dir = ".tool/skills"

[[sidecars]]
kind = "mcp"
path = ".tool/mcp.json"
global_path = ".tool/mcp.json"
`).Spec
	home := filepath.FromSlash("/home/me")

	tests := []struct {
		name string
		env  map[string]string
		want providers.GlobalPaths
	}{
		{
			name: "default home",
			want: providers.GlobalPaths{
				RootFile:  filepath.FromSlash("/home/me/.tool/RULES.md"),
				SkillsDir: filepath.FromSlash("/home/me/.tool/skills"),
				Sidecars:  map[string]string{".tool/mcp.json": filepath.FromSlash("/home/me/.tool/mcp.json")},
			},
		},
		{
			name: "home_env re-roots everything under home_dir",
			env:  map[string]string{"TOOL_HOME": filepath.FromSlash("/opt/tool")},
			want: providers.GlobalPaths{
				RootFile:      filepath.FromSlash("/opt/tool/RULES.md"),
				SkillsDir:     filepath.FromSlash("/opt/tool/skills"),
				Sidecars:      map[string]string{".tool/mcp.json": filepath.FromSlash("/opt/tool/mcp.json")},
				RelocatedHome: filepath.FromSlash("/opt/tool"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := spec.GlobalPaths(home, func(k string) string { return tt.env[k] })

			// Assert
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestGlobalPaths_NoneDeclared(t *testing.T) {
	t.Parallel()
	assert.Nil(t, loadSpec(t, `name = "plain"`).Spec.GlobalPaths("/home/me", os.Getenv))
}

func TestBuiltinGlobalPaths(t *testing.T) {
	t.Parallel()

	home := filepath.FromSlash("/h")
	j := func(p string) string { return filepath.Join(home, filepath.FromSlash(p)) }
	tests := []struct {
		preset string
		env    map[string]string
		want   providers.GlobalPaths
	}{
		{"claude", nil, providers.GlobalPaths{
			RootFile: j(".claude/CLAUDE.md"), SkillsDir: j(".claude/skills"), AgentsDir: j(".claude/agents"),
			CommandsDir: j(".claude/commands"), RulesDir: j(".claude/rules"),
			Sidecars: map[string]string{".claude/settings.json": j(".claude/settings.json")},
			// Claude Code reads user-scope MCP servers from ~/.claude.json, not settings.json.
			MCPSidecars:     map[string]string{".claude/settings.json": j(".claude.json")},
			SkillPrecedence: "Claude Code runs the user-level skill (personal over project)",
		}},
		{"amp", nil, providers.GlobalPaths{
			RootFile: j(".config/amp/AGENTS.md"), SkillsDir: j(".config/agents/skills"),
			Sidecars: map[string]string{
				".amp/settings.json":             j(".config/amp/settings.json"),
				".amp/plugins/ai-rulez-hooks.ts": j(".config/amp/plugins/ai-rulez-hooks.ts"),
			},
		}},
		{"pi", nil, providers.GlobalPaths{
			RootFile: j(".pi/agent/AGENTS.md"), SkillsDir: j(".pi/agent/skills"), AgentsDir: j(".pi/agent/agents"),
			CommandsDir: j(".pi/agent/prompts"),
			Sidecars:    map[string]string{".pi/extensions/ai-rulez-hooks.ts": j(".pi/agent/extensions/ai-rulez-hooks.ts")},
		}},
		{"junie", nil, providers.GlobalPaths{
			RootFile: j(".junie/AGENTS.md"), SkillsDir: j(".junie/skills"), AgentsDir: j(".junie/agents"),
			CommandsDir: j(".junie/commands"), Sidecars: map[string]string{
				".junie/mcp/mcp.json": j(".junie/mcp/mcp.json"),
				".junie/config.json":  j(".junie/config.json"),
			},
		}},
		{"hermes", map[string]string{"HERMES_HOME": filepath.FromSlash("/data/hermes")}, providers.GlobalPaths{
			SkillsDir:     filepath.FromSlash("/data/hermes/skills"),
			Sidecars:      map[string]string{".hermes/config.yaml": filepath.FromSlash("/data/hermes/config.yaml")},
			RelocatedHome: filepath.FromSlash("/data/hermes"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)

			// Act
			got := gen.Spec.GlobalPaths(home, func(k string) string { return tt.env[k] })

			// Assert
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

// mappedRulesSpec builds a split rules spec whose frontmatter comes from the
// given activation tables.
func mappedRulesSpec(activation string) string {
	return "name = \"tool\"\n[root]\nfile = \"AGENTS.md\"\nsections = [\"rules_inline\"]\n" +
		"[outputs.rules]\nmode = \"per_item_file\"\ndir = \".tool/rules\"\nfilename = \"{id}.md\"\nsplit = true\n" +
		"[outputs.rules.activation]\n" + activation
}

func activationRules() *config.ContentTree {
	return &config.ContentTree{Rules: []config.ContentFile{
		{Name: "always", Path: "/p/.ai-rulez/rules/always.md", Content: "ALWAYS"},
		{Name: "scoped", Path: "/p/.ai-rulez/rules/scoped.md", Content: "SCOPED", Metadata: &config.Metadata{Globs: []string{"src/**/*.ts", "*.go"}}},
		{Name: "auto", Path: "/p/.ai-rulez/rules/auto.md", Content: "AUTO", Metadata: &config.Metadata{Extra: map[string]string{"trigger": "model_decision", "description": "When editing SQL"}}},
		{Name: "manual", Path: "/p/.ai-rulez/rules/manual.md", Content: "MANUAL", Metadata: &config.Metadata{Extra: map[string]string{"trigger": "manual"}}},
	}}
}

// frontmatterOf returns the text between the first two --- fences of a rule file.
func frontmatterOf(t *testing.T, content string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(content, "---\n"), content)
	end := strings.Index(content[4:], "---\n")
	require.GreaterOrEqual(t, end, 0, content)
	return content[4 : 4+end]
}

// TestMappedRules_Activation pins the frontmatter of each activation mode for
// the vocabularies of the tools the mapping replaces Go dialects for.
func TestMappedRules_Activation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		activation string
		want       map[string]string // rule -> exact frontmatter; "" means no frontmatter block
	}{
		{
			name: "kiro steering",
			activation: "always = {inclusion = \"always\"}\n" +
				"glob = {inclusion = \"fileMatch\", fileMatchPattern = \"{globs_list}\"}\n" +
				"auto = {inclusion = \"auto\", description = \"{description}\"}\n" +
				"manual = {inclusion = \"manual\"}\n",
			want: map[string]string{
				"always": "inclusion: always\n",
				"scoped": "fileMatchPattern:\n    - src/**/*.ts\n    - '*.go'\ninclusion: fileMatch\n",
				"auto":   "description: When editing SQL\ninclusion: auto\n",
				"manual": "inclusion: manual\n",
			},
		},
		{
			name: "trae alwaysApply and joined globs",
			activation: "always = {alwaysApply = true}\n" +
				"glob = {alwaysApply = false, globs = \"{globs}\"}\n" +
				"auto = {alwaysApply = false, description = \"{description}\"}\n" +
				"manual = {alwaysApply = false}\n",
			want: map[string]string{
				"always": "alwaysApply: true\n",
				"scoped": "alwaysApply: false\nglobs: src/**/*.ts,*.go\n",
				"auto":   "alwaysApply: false\ndescription: When editing SQL\n",
				"manual": "alwaysApply: false\n",
			},
		},
		{
			name: "qwen paths, plain otherwise",
			activation: "glob = {paths = \"{globs_list}\"}\n" +
				"auto = {}\nmanual = {}\n",
			want: map[string]string{
				"always": "",
				"scoped": "paths:\n    - src/**/*.ts\n    - '*.go'\n",
				"auto":   "",
				"manual": "",
			},
		},
		{
			name: "augment type with description",
			activation: "always = {type = \"always_apply\"}\n" +
				"glob = {type = \"auto\"}\n" +
				"auto = {type = \"agent_requested\", description = \"{description}\"}\n" +
				"manual = {type = \"manual\"}\n",
			want: map[string]string{
				"always": "type: always_apply\n",
				"scoped": "type: auto\n",
				"auto":   "description: When editing SQL\ntype: agent_requested\n",
				"manual": "type: manual\n",
			},
		},
		{
			name: "aiassistant lines format",
			activation: "format = \"lines\"\n" +
				"always = {apply = \"always\"}\n" +
				"glob = {apply = \"by file patterns\", patterns = \"{globs_list}\"}\n" +
				"auto = {apply = \"by model decision\", instructions = \"{description}\"}\n" +
				"manual = {apply = \"manually\"}\n",
			want: map[string]string{
				"always": "apply: always\n",
				"scoped": "apply: by file patterns\npatterns: src/**/*.ts, *.go\n",
				"auto":   "apply: by model decision\ninstructions: When editing SQL\n",
				"manual": "apply: manually\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen := loadSpec(t, mappedRulesSpec(tt.activation))

			// Act
			outputs, err := gen.Generate(activationRules(), "/test", splitCfg("tool", "split"))

			// Assert
			require.NoError(t, err)
			for rule, want := range tt.want {
				out, ok := outputByPath(outputs, ".tool/rules/"+rule+".md")
				require.True(t, ok, rule)
				if want == "" {
					assert.False(t, strings.HasPrefix(out.Content, "---\n"), "%s must have no frontmatter:\n%s", rule, out.Content)
					continue
				}
				assert.Equal(t, want, frontmatterOf(t, out.Content), rule)
			}
		})
	}
}

// TestMappedRules_MissingModeFallsBackToAlways: a mode without a table is loaded
// always, with the always table's frontmatter.
func TestMappedRules_MissingModeFallsBackToAlways(t *testing.T) {
	t.Parallel()

	// Arrange: only always is mapped, so a glob rule degrades to it
	gen := loadSpec(t, mappedRulesSpec("always = {alwaysApply = true}\n"))

	// Act
	outputs, err := gen.Generate(activationRules(), "/test", splitCfg("tool", "split"))

	// Assert
	require.NoError(t, err)
	scoped, ok := outputByPath(outputs, ".tool/rules/scoped.md")
	require.True(t, ok)
	assert.Equal(t, "alwaysApply: true\n", frontmatterOf(t, scoped.Content))
}

// TestCommandsOutput_PromptFiles confirms [outputs.commands] already covers the
// prompt-file layouts of the commands-capable tools: a "{id}.prompt.md" filename,
// description/argument-hint/model frontmatter, and no name key with omit_name.
func TestCommandsOutput_PromptFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		toml string
		path string
		want string
	}{
		{
			name: "copilot prompt file keeps name",
			toml: "name = \"tool\"\n[outputs.commands]\nmode = \"per_item_file\"\ndir = \".github/prompts\"\nfilename = \"{id}.prompt.md\"\n" +
				"[outputs.commands.body]\nsections = [\"frontmatter\", \"content\"]\n" +
				"[outputs.commands.frontmatter]\nfields = [\"description\", \"argument-hint\"]\n",
			path: ".github/prompts/review-pr.prompt.md",
			want: "---\nargument-hint: <pr>\ndescription: Review a PR\nname: Review PR\n---\n\nBODY",
		},
		{
			name: "qwen command without name",
			toml: "name = \"tool\"\n[outputs.commands]\nmode = \"per_item_file\"\ndir = \".qwen/commands\"\nfilename = \"{id}.md\"\n" +
				"[outputs.commands.body]\nsections = [\"frontmatter\", \"content\"]\n" +
				"[outputs.commands.frontmatter]\nfields = [\"description\", \"argument-hint\"]\nomit_name = true\n",
			path: ".qwen/commands/review-pr.md",
			want: "---\nargument-hint: <pr>\ndescription: Review a PR\n---\n\nBODY",
		},
		{
			name: "model resolved like an agent",
			toml: "name = \"tool\"\n[model]\nfield = \"model\"\n[outputs.commands]\nmode = \"per_item_file\"\ndir = \".kilo/commands\"\nfilename = \"{id}.md\"\n" +
				"[outputs.commands.body]\nsections = [\"frontmatter\", \"content\"]\n" +
				"[outputs.commands.frontmatter]\nfields = [\"description\"]\nemit_model = true\nomit_name = true\n",
			path: ".kilo/commands/review-pr.md",
			want: "---\ndescription: Review a PR\nmodel: fast\n---\n\nBODY",
		},
		{
			name: "no frontmatter left writes no block",
			toml: "name = \"tool\"\n[outputs.commands]\nmode = \"per_item_file\"\ndir = \".kiro/prompts\"\nfilename = \"{id}.md\"\n" +
				"[outputs.commands.body]\nsections = [\"frontmatter\", \"content\"]\n" +
				"[outputs.commands.frontmatter]\nomit_name = true\n",
			path: ".kiro/prompts/review-pr.md",
			want: "BODY",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen := loadSpec(t, tt.toml)
			content := &config.ContentTree{Commands: []config.ContentFile{{
				Name: "Review PR", Path: "/p/.ai-rulez/commands/review-pr.md", Content: "BODY",
				Metadata: &config.Metadata{Extra: map[string]string{"description": "Review a PR", "argument-hint": "<pr>", "model": "fast"}},
			}}}

			// Act
			outputs, err := gen.Generate(content, "/test", &config.Config{Name: "t"})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, requireFile(t, outputs, tt.path).Content)
		})
	}
}
