package providers_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentFor renders one agent with the given Claude tools and model through a
// builtin preset and returns its file.
func agentFor(t *testing.T, preset, dir string, tools []string, model string) config.OutputFile {
	t.Helper()
	gen, err := providers.LoadBuiltin(preset)
	require.NoError(t, err)
	extra := map[string]string{"description": "d"}
	if model != "" {
		extra["model"] = model
	}
	content := &config.ContentTree{Agents: []config.ContentFile{{
		Name: "scout", Path: "/p/.ai-rulez/agents/scout.md", Content: "Body.",
		Metadata: &config.Metadata{Tools: tools, Extra: extra},
	}}}
	outputs, err := gen.Generate(content, "/p", &config.Config{Name: "t", BaseDir: "/p"})
	require.NoError(t, err)
	out, ok := outputByPath(outputs, dir+"/scout.md")
	require.True(t, ok, dir)
	return out
}

func TestNativeToolAndModelNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		preset    string
		dir       string
		tools     []string
		model     string
		wantTools string // exact "tools:" block, "" when no tools key
		wantModel string // "" when the model must be absent
	}{
		{"qwen maps and drops", "qwen", ".qwen/agents", []string{"Read", "Bash", "NotebookEdit"},
			"sonnet", "tools:\n    - read_file\n    - run_shell_command\n", ""},
		{"qwen keeps inherit and ids", "qwen", ".qwen/agents", []string{"Grep"}, "inherit",
			"tools:\n    - grep_search\n", "inherit"},
		{"qwen unmapped only drops tools key", "qwen", ".qwen/agents", []string{"TodoWrite"}, "glm-5", "", "glm-5"},
		{"cortex lower-cases", "cortex", ".cortex/agents", []string{"Bash", "Read"}, "opus",
			"tools:\n    - bash\n    - read\n", ""},
		{"kiro categories dedupe", "kiro", ".kiro/agents", []string{"Read", "Grep", "Bash"}, "claude-sonnet-4",
			"tools:\n    - read\n    - shell\n", "claude-sonnet-4"},
		{"augment native ids", "augment", ".augment/agents", []string{"Read", "Write"}, "haiku",
			"tools:\n    - view\n    - save-file\n", ""},
		{"rovodev writes no tools", "rovodev", ".rovodev/subagents", []string{"Read"}, "sonnet", "", ""},
		{"omp lower and inherit", "omp", ".omp/agents", []string{"Read"}, "inherit",
			"tools:\n    - read\n", "'@default'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			out := agentFor(t, tt.preset, tt.dir, tt.tools, tt.model)

			// Assert
			if tt.wantTools == "" {
				assert.NotContains(t, out.Content, "tools:")
			} else {
				assert.Contains(t, out.Content, tt.wantTools)
			}
			if tt.wantModel == "" {
				assert.NotContains(t, out.Content, "model:")
			} else {
				assert.Contains(t, out.Content, "model: "+tt.wantModel)
			}
		})
	}
}

func TestToolCaseValidation(t *testing.T) {
	t.Parallel()

	base := "name = \"x\"\n[outputs.agents]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\n" +
		"[outputs.agents.frontmatter]\n"
	tests := []struct {
		name, fm string
		wantErr  bool
	}{
		{"lower with tools", "tools = true\ntool_case = \"lower\"\n", false},
		{"unknown case", "tools = true\ntool_case = \"upper\"\n", true},
		{"names without tools", "tool_case = \"lower\"\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := providers.LoadProviderSpec([]byte(base+tt.fm), "x.toml", providers.FormatAuto)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestJunieCommands_ArgumentsBecomePrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantBody  string
		wantFlag  bool
		wantNoStr string
	}{
		{"placeholder rewritten and flagged", "Run on $ARGUMENTS now", "Run on $prompt now", true, "$ARGUMENTS"},
		{"no placeholder, no flag", "Run it", "Run it", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen, err := providers.LoadBuiltin("junie")
			require.NoError(t, err)
			content := &config.ContentTree{Commands: []config.ContentFile{{
				Name: "go", Path: "/p/.ai-rulez/commands/go.md", Content: tt.body,
				Metadata: &config.Metadata{Extra: map[string]string{"description": "d"}},
			}}}

			// Act
			outputs, err := gen.Generate(content, "/p", &config.Config{Name: "t", BaseDir: "/p"})
			require.NoError(t, err)

			// Assert
			out, ok := outputByPath(outputs, ".junie/commands/go.md")
			require.True(t, ok)
			assert.Contains(t, out.Content, tt.wantBody)
			if tt.wantNoStr != "" {
				assert.NotContains(t, out.Content, tt.wantNoStr)
			}
			assert.Equal(t, tt.wantFlag, strings.Contains(out.Content, "allowPromptArgument: true"))
		})
	}
}

func TestNativeModelNames_BareAliasTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, preset, dir, model, wantModel string
	}{
		{"factory drops sonnet", "factory", ".factory/droids", "sonnet", ""},
		{"factory keeps inherit", "factory", ".factory/droids", "inherit", "inherit"},
		{"factory keeps a native id", "factory", ".factory/droids", "gpt-5", "gpt-5"},
		{"kiro keeps a native id", "kiro", ".kiro/agents", "claude-sonnet-4", "claude-sonnet-4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := agentFor(t, tt.preset, tt.dir, nil, tt.model)
			if tt.wantModel == "" {
				assert.NotContains(t, out.Content, "model:")
				return
			}
			assert.Contains(t, out.Content, "model: "+tt.wantModel)
		})
	}
}

func TestCodebuffMCP_DollarReferences(t *testing.T) {
	t.Parallel()

	// Arrange
	gen, err := providers.LoadBuiltin("codebuff")
	require.NoError(t, err)
	cfg := &config.Config{Name: "t", BaseDir: "/p", MCPServers: map[string]*config.MCPServer{
		"s": {
			Name: "s", Command: "npx", Env: map[string]string{"TOKEN": "secret-value", "PLAIN": "x"},
			EnvRefs: map[string]string{"TOKEN": "${TOKEN}"},
		},
	}}

	// Act
	outputs, err := gen.Generate(&config.ContentTree{}, "/p", cfg)
	require.NoError(t, err)

	// Assert
	out, ok := outputByPath(outputs, ".agents/mcp.json")
	require.True(t, ok)
	assert.Contains(t, out.Content, `"TOKEN": "$TOKEN"`)
	assert.Contains(t, out.Content, `"PLAIN": "x"`)
	assert.NotContains(t, out.Content, "secret-value")
}

func TestCommandArgumentHint(t *testing.T) {
	t.Parallel()

	tests := []struct{ preset, dir string }{
		{"codebuddy", ".codebuddy/commands"}, {"reasonix", ".reasonix/commands"}, {"qoder", ".qoder/commands"},
		{"letta", ".commands"},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)
			content := &config.ContentTree{Commands: []config.ContentFile{{
				Name: "go", Path: "/p/.ai-rulez/commands/go.md", Content: "Body",
				Metadata: &config.Metadata{Extra: map[string]string{"description": "d", "argument-hint": "file"}},
			}}}

			// Act
			outputs, err := gen.Generate(content, "/p", &config.Config{Name: "t", BaseDir: "/p"})
			require.NoError(t, err)

			// Assert
			out, ok := outputByPath(outputs, tt.dir+"/go.md")
			require.True(t, ok)
			assert.Equal(t, "file", frontmatterValue(out.Content, "argument-hint"))
		})
	}
}

func TestBareAliasDroppedForForeignModelNamespaces(t *testing.T) {
	t.Parallel()

	tests := []struct{ preset, dir string }{
		{"kilo", ".kilo/agents"}, {"mimocode", ""}, {"goose", ".goose/agents"}, {"deepagents", ""},
		{"reasonix", ".reasonix/agents"}, {"qoder", ".qoder/agents"},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)
			content := &config.ContentTree{Agents: []config.ContentFile{{
				Name: "scout", Path: "/p/.ai-rulez/agents/scout.md", Content: "Body",
				Metadata: &config.Metadata{Extra: map[string]string{"description": "d", "model": "sonnet"}},
			}}}

			// Act
			outputs, err := gen.Generate(content, "/p", &config.Config{Name: "t", BaseDir: "/p"})
			require.NoError(t, err)

			// Assert
			var found bool
			for _, o := range outputs {
				if !o.IsDir && strings.Contains(filepath.ToSlash(o.Path), "/scout") {
					found = true
					assert.NotContains(t, o.Content, "model:", o.Path)
				}
			}
			assert.True(t, found, "agent file written")
		})
	}
}

func TestMCPDialectsPerPreset(t *testing.T) {
	t.Parallel()

	servers := map[string]*config.MCPServer{
		"remote": {Name: "remote", Transport: config.TransportHTTP, URL: "https://x/mcp"},
		"sse":    {Name: "sse", Transport: config.TransportSSE, URL: "https://x/sse"},
	}
	tests := []struct {
		preset, path string
		want         map[string]string // server -> a substring its entry must carry
	}{
		{"augment", ".augment/settings.json", map[string]string{"remote": `"type": "http"`, "sse": `"type": "sse"`}},
		{"zoocode", ".roo/mcp.json", map[string]string{"remote": `"type": "streamable-http"`, "sse": `"type": "sse"`}},
		{"codewhale", ".codewhale/mcp.json", map[string]string{"sse": `"transport": "sse"`}},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)
			outputs, err := gen.Generate(&config.ContentTree{}, "/p", &config.Config{Name: "t", BaseDir: "/p", MCPServers: servers})
			require.NoError(t, err)
			out, ok := outputByPath(outputs, tt.path)
			require.True(t, ok)
			for _, want := range tt.want {
				assert.Contains(t, out.Content, want)
			}
		})
	}
}
