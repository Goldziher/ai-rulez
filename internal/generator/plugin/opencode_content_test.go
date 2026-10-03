package plugin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contentHarness = `
import { registerBundledContent } from "./.opencode/ai-rulez-content.js";

const out = { skills: [], commands: [], agents: {}, mcp: {}, prompts: [] };
const ctx = {
  skill: { transform: async (fn) => fn({ get: () => undefined, add: (skill) => out.skills.push(skill) }) },
  command: { transform: async (fn) => fn({ add: (command) => out.commands.push(command) }) },
  agent: {
    transform: async (fn) =>
      fn({
        update: (id, edit) => {
          const item = { request: { body: {} } };
          edit(item);
          out.agents[id] = item;
        },
      }),
  },
  mcp: { transform: async (fn) => fn({ set: (name, config) => (out.mcp[name] = config) }) },
  session: { prompt: async (prompt) => out.prompts.push(prompt) },
};
await registerBundledContent(ctx);
for (const command of out.commands) {
  await command.execute({ sessionID: "s", prompt: { text: process.argv[2] }, delivery: "now" });
}
out.commands = out.commands.map((c) => ({ name: c.name, description: c.description }));
console.log(JSON.stringify(out));
`

// runContentHelper renders the OpenCode bundle of m into a temp directory and runs
// the embedded helper there under node with a recording context.
func runContentHelper(t *testing.T, m *Manifest, argument string, prepare func(dir string)) map[string]any {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	outputs, err := renderOpenCode(m, dir)
	require.NoError(t, err)
	for _, o := range outputs {
		if o.IsDir {
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(o.Path), 0o755))
		data := o.RawContent
		if data == nil {
			data = []byte(o.Content)
		}
		require.NoError(t, os.WriteFile(o.Path, data, 0o644))
	}
	if prepare != nil {
		prepare(dir)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"type":"module"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.mjs"), []byte(contentHarness), 0o644))

	cmd := exec.Command(node, "run.mjs", argument)
	cmd.Dir = dir
	stdout, err := cmd.Output()
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout, &result))
	return result
}

func writeSource(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestOpenCodeContentHelper_Registers(t *testing.T) {
	src := t.TempDir()
	m := &Manifest{
		Name: "p", Version: "1.0.0", SourceDir: src,
		Commands: []config.ContentFile{{
			Name:     "ask",
			Path:     writeSource(t, src, "ask.md", "---\ndescription: \"Say \\\"hi\\\": ok\"\n---\nAsk: $ARGUMENTS\n"),
			Metadata: &config.Metadata{Extra: map[string]string{"description": `Say "hi": ok`}},
		}},
		Skills: []config.ContentFile{{
			Name:     "review",
			Path:     writeSource(t, src, "skills/review/SKILL.md", "---\nname: review\ndescription: Reviews code\n---\nBody\n"),
			Metadata: &config.Metadata{Extra: map[string]string{"name": "review", "description": "Reviews code"}},
		}},
	}

	tests := []struct {
		name, argument, want string
	}{
		{"replacement patterns are literal", "$& $$ $1 $` $'", "Ask: $& $$ $1 $` $'\n"},
		{"plain text", "hello", "Ask: hello\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result := runContentHelper(t, m, tt.argument, nil)

			// Assert
			prompts := result["prompts"].([]any)
			require.Len(t, prompts, 1)
			assert.Equal(t, tt.want, prompts[0].(map[string]any)["text"])
			command := result["commands"].([]any)[0].(map[string]any)
			assert.Equal(t, `Say "hi": ok`, command["description"], "the description comes from the bundle, not a YAML parse")
			skill := result["skills"].([]any)[0].(map[string]any)
			assert.Equal(t, "Reviews code", skill["description"])
			assert.Equal(t, "Body\n", skill["content"])
		})
	}
}

func TestOpenCodeContentHelper_SkipsAnUnreadableItemAndKeepsTheRest(t *testing.T) {
	src := t.TempDir()
	m := &Manifest{
		Name: "p", Version: "1.0.0", SourceDir: src,
		Commands: []config.ContentFile{
			{Name: "good", Path: writeSource(t, src, "good.md", "Good\n")},
			{Name: "bad", Path: writeSource(t, src, "bad.md", "Bad\n")},
		},
	}

	// A directory in place of the file fails the read on every platform,
	// unlike permission bits, which Windows and root ignore.
	result := runContentHelper(t, m, "x", func(dir string) {
		bad := filepath.Join(dir, ".opencode", "commands", "bad.md")
		require.NoError(t, os.Remove(bad))
		require.NoError(t, os.Mkdir(bad, 0o755))
	})

	commands := result["commands"].([]any)
	require.Len(t, commands, 1)
	assert.Equal(t, "good", commands[0].(map[string]any)["name"])
}

func TestOpenCodeContentHelper_RegistersDisabledServers(t *testing.T) {
	m := &Manifest{
		Name: "p", Version: "1.0.0", SourceDir: t.TempDir(),
		MCP: []config.PluginMCPLaunch{
			{Name: "off", Command: "x", Transport: config.TransportStdio, Disabled: true},
			{Name: "on", Command: "y", Transport: config.TransportStdio},
			{Name: "remote-off", Transport: config.TransportHTTP, URL: "https://e.example", Disabled: true},
		},
	}

	result := runContentHelper(t, m, "x", nil)

	servers := result["mcp"].(map[string]any)
	assert.Equal(t, true, servers["off"].(map[string]any)["disabled"])
	assert.NotContains(t, servers["on"], "disabled")
	assert.Equal(t, true, servers["remote-off"].(map[string]any)["disabled"])
}

func TestOpenCodeScaffoldQuotesTheNameAsJavaScript(t *testing.T) {
	tests := []struct{ name, wantID, wantComment string }{
		{"plain", `id: "plain",`, "adapter for plain."},
		{"a\"b", `id: "a\"b",`, "adapter for a\"b."},
		{"x*/y", `id: "x*/y",`, "adapter for x* /y."},
		{"line\nbreak", `id: "line\nbreak",`, "adapter for line break."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			scaffold := openCodeScaffold(tt.name, false)

			// Assert
			assert.Contains(t, scaffold, tt.wantID)
			assert.Contains(t, scaffold, tt.wantComment)
			head := scaffold[:strings.Index(scaffold, "export default")]
			assert.Equal(t, 2, strings.Count(head, "*/"), "only the two comments of the scaffold end")
		})
	}
}

func TestOpenCodePublishedFilesSplitsOnBothSeparators(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "scripts"), 0o755))
	m := &Manifest{
		Name: "p", SourceDir: src,
		MCP: []config.PluginMCPLaunch{{Name: "s", Command: `${PLUGIN_ROOT}\scripts\run.cmd`, Transport: config.TransportStdio}},
	}

	assert.Contains(t, openCodePublishedFiles(m), "scripts/")
}

func TestResolveMCPCarriesDisabledServers(t *testing.T) {
	off := false
	cfg := &config.Config{MCPServers: map[string]*config.MCPServer{
		"off": {Name: "off", Command: "x", Enabled: &off},
		"on":  {Name: "on", Command: "y"},
	}}

	got := resolveMCP(&config.PluginAuthoring{}, cfg, []string{config.PluginRuntimeOpenCode})

	require.Len(t, got, 2)
	assert.Equal(t, "off", got[0].Name)
	assert.True(t, got[0].Disabled)
	assert.False(t, got[1].Disabled)
}
