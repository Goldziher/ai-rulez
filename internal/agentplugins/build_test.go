package agentplugins

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMatchesTheGoldens(t *testing.T) {
	for _, spec := range Specs {
		t.Run(spec, func(t *testing.T) {
			// Arrange
			dir := goldenDir(spec)

			// Act
			files, findings, err := Build(fixturePlugin(), Options{Spec: spec})

			// Assert
			require.NoError(t, err)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				writeTree(t, dir, files)
			}
			want := readTree(t, dir)
			assert.Equal(t, slices.Sorted(maps.Keys(want)), slices.Sorted(maps.Keys(files)))
			for name, data := range want {
				assert.Equal(t, string(data), string(files[name]), name)
			}
			assert.Equal(t, []string{
				"placeholder-rewritten@mcp.json#/mcpServers/validator",
				"placeholder-rewritten@mcp.json#/mcpServers/validator",
			}, codes(findings))
		})
	}
}

func TestBuildDefaultsTo100(t *testing.T) {
	files, _, err := Build(&Plugin{Metadata: Metadata{Name: "a"}}, Options{})

	require.NoError(t, err)
	assert.Contains(t, string(files["plugin.json"]), PluginSchemaID(Spec100))
}

func TestBuildMCPSchemaMatchesThePluginSchemaVersion(t *testing.T) {
	for _, spec := range Specs {
		files, _, err := Build(fixturePlugin(), Options{Spec: spec})
		require.NoError(t, err)

		var plugin, mcp map[string]any
		require.NoError(t, json.Unmarshal(files["plugin.json"], &plugin))
		require.NoError(t, json.Unmarshal(files["mcp.json"], &mcp))

		assert.Equal(t, PluginSchemaID(spec), plugin["$schema"])
		assert.Equal(t, MCPSchemaID(spec), mcp["$schema"])
	}
}

func TestBuildIsIndependentOfInputOrder(t *testing.T) {
	// Arrange
	forward := fixturePlugin()
	reversed := fixturePlugin()
	slices.Reverse(reversed.Skills)
	slices.Reverse(reversed.MCPServers)
	slices.Reverse(reversed.Extensions)

	// Act
	a, fa, errA := Build(forward, Options{})
	b, fb, errB := Build(reversed, Options{})

	// Assert
	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Equal(t, a, b)
	assert.Equal(t, fa, fb)
}

func TestBuildOmitsMCPWhenNoServerSurvives(t *testing.T) {
	p := &Plugin{
		Metadata:   Metadata{Name: "a"},
		MCPServers: []MCPServer{{Name: "bad", Command: "npx -y server"}},
	}

	files, findings, err := Build(p, Options{})

	require.NoError(t, err)
	assert.NotContains(t, files, "mcp.json")
	assert.Equal(t, []string{"mcp-server-invalid@mcp.json#/mcpServers/bad"}, codes(findings))
}

func TestBuildRejectsWhatCannotFormAPlugin(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Plugin)
		opts    Options
		wantErr string
	}{
		{"unsupported spec", func(*Plugin) {}, Options{Spec: "2.0.0"}, "unsupported Agent Plugins version"},
		{"invalid plugin name", func(p *Plugin) { p.Metadata.Name = "Acme_Tools" }, Options{}, "plugin name"},
		{"empty plugin name", func(p *Plugin) { p.Metadata.Name = "" }, Options{}, "plugin name"},
		{"duplicate skill", func(p *Plugin) { p.Skills = append(p.Skills, p.Skills[0]) }, Options{}, "duplicate skill"},
		{"duplicate server", func(p *Plugin) { p.MCPServers = append(p.MCPServers, p.MCPServers[0]) }, Options{}, "duplicate MCP server"},
		{"empty server name", func(p *Plugin) { p.MCPServers[0].Name = "" }, Options{}, "MCP server name"},
		{"skill file escapes", func(p *Plugin) { p.Skills[0].Files["../x"] = nil }, Options{}, "invalid path"},
		{"skill file absolute", func(p *Plugin) { p.Skills[0].Files["/etc/x"] = nil }, Options{}, "invalid path"},
		{"skill file shadows SKILL.md", func(p *Plugin) { p.Skills[0].Files["SKILL.md"] = nil }, Options{}, "SKILL.md"},
		{"invalid namespace", func(p *Plugin) { p.Extensions[0].Namespace = "claude" }, Options{}, "namespace"},
		{"reserved namespace", func(p *Plugin) { p.Extensions[0].Namespace = "mcp.json" }, Options{}, "namespace"},
		{"duplicate namespace", func(p *Plugin) { p.Extensions[1].Namespace = p.Extensions[0].Namespace }, Options{}, "duplicate extension"},
		{"extension file escapes", func(p *Plugin) { p.Extensions[0].Files["a/../../x"] = nil }, Options{}, "invalid path"},
		{"backslash path", func(p *Plugin) { p.Files[`bin\x`] = nil }, Options{}, "invalid path"},
		{"root file over plugin.json", func(p *Plugin) { p.Files["plugin.json"] = nil }, Options{}, "reserved"},
		{"root file under skills", func(p *Plugin) { p.Files["skills/x/SKILL.md"] = nil }, Options{}, "reserved"},
		{"root file in a namespace dir", func(p *Plugin) { p.Files["com.acme.client/x"] = nil }, Options{}, "extension"},
		{"unmarshalable manifest data", func(p *Plugin) {
			p.Extensions[1].Manifest = map[string]any{"f": func() {}}
		}, Options{}, "extension"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := fixturePlugin()
			tt.mutate(p)

			// Act
			_, _, err := Build(p, tt.opts)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBuildMapsMCPServers(t *testing.T) {
	const skipped = ""
	tests := []struct {
		name   string
		server MCPServer
		want   string // the mcpServers entry as compact JSON, or skipped
		codes  []string
	}{
		{"bare command", MCPServer{Command: "uvx", Args: []string{"srv"}},
			`{"type":"stdio","command":"uvx","args":["srv"]}`, nil},
		{"explicit stdio", MCPServer{Transport: "stdio", Command: "uvx"}, `{"type":"stdio","command":"uvx"}`, nil},
		{"plugin-relative command", MCPServer{Command: "./bin/s"}, `{"type":"stdio","command":"./bin/s"}`, nil},
		{"PLUGIN_ROOT command rewritten", MCPServer{Command: "${PLUGIN_ROOT}/bin/s"},
			`{"type":"stdio","command":"./bin/s"}`, []string{"placeholder-rewritten"}},
		{"shell command string", MCPServer{Command: "npx -y srv"}, skipped, []string{"mcp-server-invalid"}},
		{"absolute command", MCPServer{Command: "/usr/bin/srv"}, skipped, []string{"mcp-server-invalid"}},
		{"parent command", MCPServer{Command: "../srv"}, skipped, []string{"mcp-server-invalid"}},
		{"dot-slash parent command", MCPServer{Command: "./a/../../srv"}, skipped, []string{"mcp-server-invalid"}},
		{"bare PLUGIN_ROOT command", MCPServer{Command: "${PLUGIN_ROOT}"}, skipped, []string{"mcp-server-invalid"}},
		{"PLUGIN_DATA command", MCPServer{Command: "${PLUGIN_DATA}/srv"}, skipped, []string{"mcp-server-invalid"}},
		{"env placeholder in command", MCPServer{Command: "${HOME}/srv"}, skipped, []string{"mcp-server-invalid"}},
		{"no command", MCPServer{Transport: "stdio"}, skipped, []string{"mcp-server-invalid"}},
		{"plugin placeholders in args env cwd", MCPServer{Command: "srv", Args: []string{"${PLUGIN_DATA}/x"},
			Env: map[string]string{"A": "${PLUGIN_ROOT}/a"}, Cwd: "${PLUGIN_DATA}/work"},
			`{"type":"stdio","command":"srv","args":["${PLUGIN_DATA}/x"],"env":{"A":"${PLUGIN_ROOT}/a"},"cwd":"${PLUGIN_DATA}/work"}`, nil},
		{"user placeholder in args", MCPServer{Command: "srv", Args: []string{"--key", "${API_KEY}"}},
			skipped, []string{"placeholder-unsupported"}},
		{"env passthrough dropped", MCPServer{Command: "srv", Env: map[string]string{"TOKEN": "${TOKEN}", "MODE": "ci"}},
			`{"type":"stdio","command":"srv","env":{"MODE":"ci"}}`, []string{"placeholder-rewritten"}},
		{"env placeholder inside a value", MCPServer{Command: "srv", Env: map[string]string{"AUTH": "Bearer ${TOKEN}"}},
			skipped, []string{"placeholder-unsupported"}},
		{"env placeholder of another name", MCPServer{Command: "srv", Env: map[string]string{"A": "${B}"}},
			skipped, []string{"placeholder-unsupported"}},
		{"reserved env key", MCPServer{Command: "srv", Env: map[string]string{"PLUGIN_ROOT": "/x"}},
			skipped, []string{"mcp-server-invalid"}},
		{"cwd plugin-relative", MCPServer{Command: "srv", Cwd: "./data"}, `{"type":"stdio","command":"srv","cwd":"./data"}`, nil},
		{"cwd bare relative", MCPServer{Command: "srv", Cwd: "data"}, skipped, []string{"mcp-server-invalid"}},
		{"cwd escapes", MCPServer{Command: "srv", Cwd: "./../x"}, skipped, []string{"mcp-server-invalid"}},
		{"cwd escapes PLUGIN_ROOT", MCPServer{Command: "srv", Cwd: "${PLUGIN_ROOT}/../x"}, skipped, []string{"mcp-server-invalid"}},
		{"cwd absolute", MCPServer{Command: "srv", Cwd: "/tmp"}, skipped, []string{"mcp-server-invalid"}},
		{"stdio drops a url", MCPServer{Command: "srv", URL: "https://x.example"},
			`{"type":"stdio","command":"srv"}`, []string{"field-ignored"}},
		{"http to streamable-http", MCPServer{Transport: "http", URL: "https://x.example/mcp"},
			`{"type":"streamable-http","url":"https://x.example/mcp"}`, nil},
		{"streamable-http spelled out", MCPServer{Transport: "streamable-http", URL: "https://x.example/mcp"},
			`{"type":"streamable-http","url":"https://x.example/mcp"}`, nil},
		{"url without transport", MCPServer{URL: "https://x.example/mcp"},
			`{"type":"streamable-http","url":"https://x.example/mcp"}`, nil},
		{"explicit sse", MCPServer{Transport: "sse", URL: "https://x.example/sse"},
			`{"type":"sse","url":"https://x.example/sse"}`, nil},
		{"http to localhost", MCPServer{Transport: "http", URL: "http://localhost:8080/mcp"},
			`{"type":"streamable-http","url":"http://localhost:8080/mcp"}`, nil},
		{"http to loopback v4", MCPServer{Transport: "http", URL: "http://127.0.0.2/mcp"},
			`{"type":"streamable-http","url":"http://127.0.0.2/mcp"}`, nil},
		{"http to loopback v6", MCPServer{Transport: "http", URL: "http://[::1]:3000/mcp"},
			`{"type":"streamable-http","url":"http://[::1]:3000/mcp"}`, nil},
		{"http to a remote host", MCPServer{Transport: "http", URL: "http://x.example/mcp"}, skipped, []string{"mcp-server-invalid"}},
		{"http to localhost lookalike", MCPServer{Transport: "http", URL: "http://localhost.x.example/mcp"}, skipped, []string{"mcp-server-invalid"}},
		{"url with user info", MCPServer{Transport: "http", URL: "https://u:p@x.example/mcp"}, skipped, []string{"mcp-server-invalid"}},
		{"url with fragment", MCPServer{Transport: "http", URL: "https://x.example/mcp#a"}, skipped, []string{"mcp-server-invalid"}},
		{"relative url", MCPServer{Transport: "http", URL: "/mcp"}, skipped, []string{"mcp-server-invalid"}},
		{"non-http url", MCPServer{Transport: "http", URL: "ftp://x.example/mcp"}, skipped, []string{"mcp-server-invalid"}},
		{"placeholder in url", MCPServer{Transport: "http", URL: "https://${HOST}/mcp"}, skipped, []string{"placeholder-unsupported"}},
		{"placeholder in header", MCPServer{Transport: "http", URL: "https://x.example", Headers: map[string]string{"X-Key": "${KEY}"}},
			skipped, []string{"placeholder-unsupported"}},
		{"credential header", MCPServer{Transport: "http", URL: "https://x.example", Headers: map[string]string{"authorization": "Bearer abc"}},
			skipped, []string{"credential-header"}},
		{"duplicate header names", MCPServer{Transport: "http", URL: "https://x.example", Headers: map[string]string{"X-A": "1", "x-a": "2"}},
			skipped, []string{"mcp-server-invalid"}},
		{"invalid header name", MCPServer{Transport: "http", URL: "https://x.example", Headers: map[string]string{"X A": "1"}},
			skipped, []string{"mcp-server-invalid"}},
		{"invalid header value", MCPServer{Transport: "http", URL: "https://x.example", Headers: map[string]string{"X-A": "1\r\nX-B: 2"}},
			skipped, []string{"mcp-server-invalid"}},
		{"remote drops a command", MCPServer{Transport: "sse", URL: "https://x.example/sse", Command: "srv"},
			`{"type":"sse","url":"https://x.example/sse"}`, []string{"field-ignored"}},
		{"remote without url", MCPServer{Transport: "http"}, skipped, []string{"mcp-server-invalid"}},
		{"unknown transport", MCPServer{Transport: "websocket", URL: "wss://x.example"}, skipped, []string{"mcp-server-invalid"}},
		{"disabled", MCPServer{Command: "srv", Enabled: new(false)}, skipped, []string{"mcp-server-disabled"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			tt.server.Name = "s"
			p := &Plugin{Metadata: Metadata{Name: "a"}, MCPServers: []MCPServer{tt.server}}

			// Act
			files, findings, err := Build(p, Options{})

			// Assert
			require.NoError(t, err)
			got := make([]string, 0, len(findings))
			for _, f := range findings {
				got = append(got, f.Code)
				assert.Equal(t, "mcp.json#/mcpServers/s", f.Path)
				assert.NotEmpty(t, f.Message)
			}
			assert.Equal(t, tt.codes, nilIfEmpty(got))
			if tt.want == skipped {
				assert.NotContains(t, files, "mcp.json")
				return
			}
			var doc struct {
				MCPServers map[string]json.RawMessage `json:"mcpServers"`
			}
			require.NoError(t, json.Unmarshal(files["mcp.json"], &doc))
			assert.JSONEq(t, tt.want, string(doc.MCPServers["s"]))
		})
	}
}

func TestBuildSkipsSkillsThatViolateAgentSkills(t *testing.T) {
	tests := []struct {
		name    string
		skill   Skill
		kept    bool
		codes   []string
		message string
	}{
		{"valid", Skill{Name: "ok", SkillMD: []byte("---\nname: ok\ndescription: Does it.\n---\nbody\n")}, true, nil, ""},
		{"crlf frontmatter", Skill{Name: "ok", SkillMD: []byte("---\r\nname: ok\r\ndescription: Does it.\r\n---\r\nbody\r\n")}, true, nil, ""},
		{"non-spec field kept", Skill{Name: "ok", SkillMD: []byte("---\nname: ok\ndescription: d\nuser-invocable: true\n---\n")},
			true, []string{"skill-unknown-field"}, "user-invocable"},
		{"invalid directory name", Skill{Name: "Bad_Name", SkillMD: []byte("---\nname: Bad_Name\ndescription: d\n---\n")},
			false, []string{"skill-invalid"}, "name"},
		{"name differs from directory", Skill{Name: "ok", SkillMD: []byte("---\nname: other\ndescription: d\n---\n")},
			false, []string{"skill-invalid"}, "directory"},
		{"no description", Skill{Name: "ok", SkillMD: []byte("---\nname: ok\n---\n")}, false, []string{"skill-invalid"}, "description"},
		{"long description", Skill{Name: "ok", SkillMD: []byte("---\nname: ok\ndescription: " + strings.Repeat("x", 1025) + "\n---\n")},
			false, []string{"skill-invalid"}, "1024"},
		{"long compatibility", Skill{Name: "ok", SkillMD: []byte("---\nname: ok\ndescription: d\ncompatibility: " + strings.Repeat("x", 501) + "\n---\n")},
			false, []string{"skill-invalid"}, "500"},
		{"non-string metadata", Skill{Name: "ok", SkillMD: []byte("---\nname: ok\ndescription: d\nmetadata:\n  a: [1]\n---\n")},
			false, []string{"skill-invalid"}, "metadata"},
		{"no frontmatter", Skill{Name: "ok", SkillMD: []byte("# ok\n")}, false, []string{"skill-invalid"}, "frontmatter"},
		{"unterminated frontmatter", Skill{Name: "ok", SkillMD: []byte("---\nname: ok\n")}, false, []string{"skill-invalid"}, "frontmatter"},
		{"broken yaml", Skill{Name: "ok", SkillMD: []byte("---\nname: [ok\n---\n")}, false, []string{"skill-invalid"}, "frontmatter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := &Plugin{Metadata: Metadata{Name: "a"}, Skills: []Skill{tt.skill}}

			// Act
			files, findings, err := Build(p, Options{})

			// Assert
			require.NoError(t, err)
			_, kept := files["skills/"+tt.skill.Name+"/SKILL.md"]
			assert.Equal(t, tt.kept, kept)
			got := make([]string, 0, len(findings))
			for _, f := range findings {
				got = append(got, f.Code)
				assert.Equal(t, "skills/"+tt.skill.Name+"/SKILL.md", f.Path)
				assert.Contains(t, f.Message, tt.message)
			}
			assert.Equal(t, tt.codes, nilIfEmpty(got))
		})
	}
}

func TestBuildDropsAnEmptyAuthor(t *testing.T) {
	files, _, err := Build(&Plugin{Metadata: Metadata{Name: "a", Author: &Author{}}}, Options{})

	require.NoError(t, err)
	assert.NotContains(t, string(files["plugin.json"]), "author")
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
