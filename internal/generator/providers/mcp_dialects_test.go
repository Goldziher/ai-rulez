package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

func dialectServers() *config.Config {
	off := false
	return &config.Config{MCPServers: map[string]*config.MCPServer{
		"local": {
			Name: "local", Command: "npx", Args: []string{"-y", "pkg"},
			Env: map[string]string{"API_KEY": "secret"},
		},
		"http": {
			Name: "http", Transport: config.TransportHTTP, URL: "https://mcp.example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer t"}, Description: "Remote server",
		},
		"sse": {Name: "sse", Transport: config.TransportSSE, URL: "https://mcp.example.com/sse"},
		"off": {Name: "off", Command: "off-cmd", Enabled: &off},
	}}
}

// TestMCPDialectEntries pins the exact JSON each dialect renders for the four
// server shapes: local, streamable HTTP with headers, SSE, and a disabled local.
func TestMCPDialectEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{
			name: "standard", dialect: MCPDialectStandard,
			want: `{
				"local": {"command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"url": "https://mcp.example.com/mcp", "description": "Remote server", "headers": {"Authorization": "Bearer t"}},
				"sse": {"url": "https://mcp.example.com/sse"},
				"off": {"command": "off-cmd", "disabled": true}}`,
		},
		{
			name: "claude", dialect: MCPDialectClaude,
			want: `{
				"local": {"command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"type": "http", "url": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}},
				"sse": {"type": "sse", "url": "https://mcp.example.com/sse"},
				"off": {"command": "off-cmd", "disabled": true}}`,
		},
		{
			name: "gemini", dialect: MCPDialectGemini,
			want: `{
				"local": {"command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"httpUrl": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}},
				"sse": {"url": "https://mcp.example.com/sse"},
				"off": {"command": "off-cmd", "disabled": true}}`,
		},
		{
			name: "opencode", dialect: MCPDialectOpencode,
			want: `{
				"local": {"type": "local", "command": ["npx", "-y", "pkg"], "environment": {"API_KEY": "secret"}, "enabled": true},
				"http": {"type": "remote", "url": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}, "enabled": true},
				"sse": {"type": "remote", "url": "https://mcp.example.com/sse", "enabled": true},
				"off": {"type": "local", "command": ["off-cmd"], "enabled": false}}`,
		},
		{
			name: "vscode drops the disabled server", dialect: MCPDialectVSCode,
			want: `{
				"local": {"type": "stdio", "command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"type": "http", "url": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}},
				"sse": {"type": "sse", "url": "https://mcp.example.com/sse"}}`,
		},
		{
			name: "zed drops sse and marks disabled", dialect: MCPDialectZed,
			want: `{
				"local": {"command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"url": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}},
				"off": {"command": "off-cmd", "enabled": false}}`,
		},
		{
			name: "roo types every remote server", dialect: MCPDialectRoo,
			want: `{
				"local": {"command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"type": "streamable-http", "url": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}},
				"sse": {"type": "sse", "url": "https://mcp.example.com/sse"},
				"off": {"command": "off-cmd", "disabled": true}}`,
		},
		{
			name: "codewhale marks sse", dialect: MCPDialectCodewhale,
			want: `{
				"local": {"command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"url": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}},
				"sse": {"url": "https://mcp.example.com/sse", "transport": "sse"},
				"off": {"command": "off-cmd", "disabled": true}}`,
		},
		{
			name: "amp is standard without the description", dialect: MCPDialectAmp,
			want: `{
				"local": {"command": "npx", "args": ["-y", "pkg"], "env": {"API_KEY": "secret"}},
				"http": {"url": "https://mcp.example.com/mcp", "headers": {"Authorization": "Bearer t"}},
				"sse": {"url": "https://mcp.example.com/sse"},
				"off": {"command": "off-cmd", "disabled": true}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			dialect, err := mcpDialectFor(tt.dialect)
			require.NoError(t, err)

			// Act
			entries := mcpDialectEntries(dialect, dialectServers())

			// Assert
			got, err := json.Marshal(entries)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

// TestMCPDialectEntries_CodexTOML pins the exact TOML of the codex dialect.
func TestMCPDialectEntries_CodexTOML(t *testing.T) {
	t.Parallel()

	// Arrange
	dialect, err := mcpDialectFor(MCPDialectCodex)
	require.NoError(t, err)
	cfg := dialectServers()
	delete(cfg.MCPServers, "sse")

	// Act
	got, err := toml.Marshal(map[string]any{"mcp_servers": mcpDialectEntries(dialect, cfg)})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, `[mcp_servers]
[mcp_servers.http]
url = 'https://mcp.example.com/mcp'

[mcp_servers.http.http_headers]
Authorization = 'Bearer t'

[mcp_servers.local]
args = ['-y', 'pkg']
command = 'npx'

[mcp_servers.local.env]
API_KEY = 'secret'

[mcp_servers.off]
command = 'off-cmd'
enabled = false
`, string(got))
}

// TestMCPDialectEntries_YAMLStandard pins the exact YAML of the poolside dialect:
// a remote server carries a transport block with a "Name: value" header list.
func TestMCPDialectEntries_YAMLStandard(t *testing.T) {
	t.Parallel()

	// Arrange
	dialect, err := mcpDialectFor(MCPDialectYAMLStandard)
	require.NoError(t, err)
	cfg := dialectServers()
	delete(cfg.MCPServers, "sse")
	delete(cfg.MCPServers, "off")

	// Act
	got, err := yaml.Marshal(map[string]any{"mcp_servers": mcpDialectEntries(dialect, cfg)})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, `mcp_servers:
    http:
        transport:
            headers:
                - 'Authorization: Bearer t'
            type: http
            url: https://mcp.example.com/mcp
    local:
        args:
            - -y
            - pkg
        command: npx
        env:
            API_KEY: secret
`, string(got))
}

func TestMCPDialectDefaultKeys(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		MCPDialectStandard: {"mcpServers"}, MCPDialectClaude: {"mcpServers"}, MCPDialectGemini: {"mcpServers"},
		MCPDialectOpencode: {"mcp"}, MCPDialectVSCode: {"servers"}, MCPDialectZed: {"context_servers"},
		MCPDialectCodex: {"mcp_servers"}, MCPDialectAmp: {"amp.mcpServers"}, MCPDialectYAMLStandard: {"mcp_servers"},
		"": {"mcpServers"},
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := mcpDialectFor(name)
			require.NoError(t, err)
			assert.Equal(t, want, d.defaultKey)
		})
	}

	_, err := mcpDialectFor("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown mcp dialect "nope"`)
}

// TestRenderAggregate covers the aggregate checks output through the extension
// point that supplies the items until the checks content kind exists.
func TestRenderAggregate(t *testing.T) {
	// Not parallel: it swaps the package-level checkItems hook.
	spec, err := LoadProviderSpec([]byte(`
name = "bugbot"
[outputs.checks]
mode = "aggregate"
file = ".cursor/BUGBOT.md"
header = "# Bugbot rules\n"
`), "bugbot.toml", FormatAuto)
	require.NoError(t, err)
	gen := New(spec)

	tests := []struct {
		name  string
		items []config.ContentFile
		want  string // "" means no file
	}{
		{name: "no checks writes no file", items: nil, want: ""},
		{
			name: "sections joined under the header",
			items: []config.ContentFile{
				{Name: "sql-injection", Content: "Flag string-built SQL.\n"},
				{Name: "secrets", Content: "Flag hardcoded tokens."},
			},
			want: "# Bugbot rules\n\n<!-- ai-rulez:checks:begin -->\n" +
				"<!-- ai-rulez:check:sql-injection -->\n\n## sql-injection\n\nFlag string-built SQL.\n\n" +
				"<!-- ai-rulez:check:secrets -->\n\n## secrets\n\nFlag hardcoded tokens.\n" +
				"<!-- ai-rulez:checks:end -->\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			orig := checkItems
			t.Cleanup(func() { checkItems = orig })
			checkItems = func(*config.Config, *config.ContentTree) []config.ContentFile { return tt.items }

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, absSlash("/proj"), &config.Config{Name: "t"})

			// Assert
			require.NoError(t, err)
			var got *config.OutputFile
			for i := range outputs {
				if outputs[i].Path == filepath.Join(absSlash("/proj"), ".cursor", "BUGBOT.md") {
					got = &outputs[i]
				}
			}
			if tt.want == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want, string(got.RawContent))
		})
	}
}

func TestRenderChecks_PerItemFile(t *testing.T) {
	spec, err := LoadProviderSpec([]byte(`
name = "x"
[outputs.checks]
mode = "per_item_file"
dir = ".x/checks"
filename = "{id}.md"
[outputs.checks.body]
sections = ["frontmatter", "content"]
`), "x.toml", FormatAuto)
	require.NoError(t, err)
	orig := checkItems
	t.Cleanup(func() { checkItems = orig })
	checkItems = func(*config.Config, *config.ContentTree) []config.ContentFile {
		return []config.ContentFile{{Name: "Perf Check", Content: "Look at hot loops."}}
	}

	outputs, err := New(spec).Generate(&config.ContentTree{}, absSlash("/proj"), &config.Config{Name: "t"})

	require.NoError(t, err)
	var content string
	for _, o := range outputs {
		if o.Path == filepath.Join(absSlash("/proj"), ".x", "checks", "perf-check.md") {
			content = o.Content
		}
	}
	assert.Equal(t, "---\nname: Perf Check\n---\n\nLook at hot loops.", content)
}

// TestMCPDialectEntries_SkipsUnexpressibleServers: a stdio server without a
// command or a remote one without a url cannot be written; it is left out of the
// document instead of emitting an empty entry.
func TestMCPDialectEntries_SkipsUnexpressibleServers(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{MCPServers: map[string]*config.MCPServer{
		"nocmd": {Name: "nocmd"},
		"nourl": {Name: "nourl", Transport: config.TransportHTTP},
		"good":  {Name: "good", Command: "npx"},
	}}
	for _, name := range []string{MCPDialectVSCode, MCPDialectZed, MCPDialectCodex, MCPDialectYAMLStandard,
		MCPDialectStandard, MCPDialectAmp} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			dialect, err := mcpDialectFor(name)
			require.NoError(t, err)

			// Act
			got := mcpDialectEntries(dialect, cfg)

			// Assert
			assert.Len(t, got, 1)
			assert.Contains(t, got, "good")
		})
	}
	assert.Nil(t, ampMCPEntry(&config.MCPServer{}), "amp must not panic on an inexpressible server")
}

func TestMCPDialectEntries_PoolsideStdioAlwaysHasArgs(t *testing.T) {
	t.Parallel()

	dialect, err := mcpDialectFor(MCPDialectYAMLStandard)
	require.NoError(t, err)
	cfg := &config.Config{MCPServers: map[string]*config.MCPServer{"s": {Name: "s", Command: "run"}}}

	got, err := yaml.Marshal(mcpDialectEntries(dialect, cfg))

	require.NoError(t, err)
	assert.Equal(t, "s:\n    args: []\n    command: run\n", string(got))
}

func TestSidecarDocFormat_DialectDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sidecar SidecarSpec
		want    string
	}{
		{SidecarSpec{Kind: SidecarMCP, Dialect: MCPDialectZed, Path: ".zed/settings.json"}, DocFormatJSONC},
		{SidecarSpec{Kind: SidecarMCP, Dialect: MCPDialectVSCode, Path: ".vscode/mcp.json"}, DocFormatJSONC},
		{SidecarSpec{Kind: SidecarMCP, Dialect: MCPDialectZed, Path: "s.json", Format: DocFormatJSON}, DocFormatJSON},
		{SidecarSpec{Kind: SidecarMCP, Dialect: MCPDialectStandard, Path: ".mcp.json"}, DocFormatJSON},
		{SidecarSpec{Kind: SidecarMCP, Dialect: MCPDialectCodex, Path: "c.toml"}, DocFormatTOML},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.sidecar.DocFormat(), "%s %s", tt.sidecar.Dialect, tt.sidecar.Path)
	}
}

// TestGenericMCPSidecar_DottedFlatKey: the amp key "amp.mcpServers" is one flat
// member name, not a path, in TOML and YAML as well as JSON.
func TestGenericMCPSidecar_DottedFlatKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format, path, existing string
		wantQuoted             string
	}{
		{"toml", "s.toml", "# mine\nx = 1\n", `"amp.mcpServers"`},
		{"yaml", "s.yaml", "# mine\nx: 1\n", `amp.mcpServers:`},
		{"json", "s.json", "{\"x\": 1}\n", `"amp.mcpServers"`},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()

			// Arrange
			dir := t.TempDir()
			spec := &ProviderSpec{Name: "t", Sidecars: []*SidecarSpec{{Kind: SidecarMCP, Dialect: MCPDialectAmp, Path: tt.path}}}
			require.NoError(t, validateSpec(spec))
			require.NoError(t, os.WriteFile(filepath.Join(dir, tt.path), []byte(tt.existing), 0o600))
			cfg := &config.Config{BaseDir: dir, MCPServers: map[string]*config.MCPServer{"s": {Name: "s", Command: "run"}}}

			// Act
			out, err := New(spec).renderGenericSidecar(spec.Sidecars[0], cfg, dir+"/"+tt.path)

			// Assert
			require.NoError(t, err)
			assert.Contains(t, out.Body, tt.wantQuoted)
			assert.Contains(t, out.Body, "x")
			assert.Contains(t, out.Body, "run")
			if tt.format != "json" {
				assert.Contains(t, out.Body, "# mine", "comments survive")
			}
			var doc map[string]any
			switch tt.format {
			case "toml":
				require.NoError(t, toml.Unmarshal([]byte(out.Body), &doc))
			case "yaml":
				require.NoError(t, yaml.Unmarshal([]byte(out.Body), &doc))
			default:
				require.NoError(t, json.Unmarshal([]byte(out.Body), &doc))
			}
			assert.Contains(t, doc, "amp.mcpServers", "one flat member, not a nested path")
			assert.NotContains(t, doc, "amp")
		})
	}
}

// TestMCPDialects_AllResolve pins the dialect registry: every constant resolves
// and carries a builder, and every builtin spec's sidecar dialect is known. The
// registry is one map literal, so this does not depend on init order.
func TestMCPDialects_AllResolve(t *testing.T) {
	t.Parallel()

	tests := []string{
		"", MCPDialectStandard, MCPDialectClaude, MCPDialectGemini, MCPDialectOpencode, MCPDialectVSCode,
		MCPDialectZed, MCPDialectCodex, MCPDialectAmp, MCPDialectYAMLStandard, MCPDialectBob,
		MCPDialectZcode, MCPDialectGrok, MCPDialectTransport, MCPDialectVibe, MCPDialectRoo, MCPDialectCodewhale,
	}
	for _, name := range tests {
		t.Run("dialect "+name, func(t *testing.T) {
			t.Parallel()
			d, err := mcpDialectFor(name)
			require.NoError(t, err)
			assert.NotNil(t, d.build)
			assert.NotEmpty(t, d.defaultKey)
		})
	}
	for _, spec := range loadBuiltinSpecs() {
		for _, sidecar := range spec.Sidecars {
			if sidecar.Dialect == "" {
				continue
			}
			if sidecar.Kind == SidecarChecks {
				assert.True(t, isChecksDialect(sidecar.Dialect), "%s sidecar %s uses unknown checks dialect %q", spec.Name, sidecar.Path, sidecar.Dialect)
				continue
			}
			if sidecar.Kind == SidecarPermissions {
				assert.True(t, settings.IsPermissionDialect(sidecar.Dialect), "%s sidecar %s uses unknown permissions dialect %q", spec.Name, sidecar.Path, sidecar.Dialect)
				continue
			}
			if sidecar.Kind == SidecarHooks {
				assert.True(t, settings.HasHookDialect(sidecar.Dialect), "%s sidecar %s uses unknown hooks dialect %q", spec.Name, sidecar.Path, sidecar.Dialect)
				continue
			}
			assert.True(t, IsMCPDialect(sidecar.Dialect), "%s sidecar %s uses unknown dialect %q", spec.Name, sidecar.Path, sidecar.Dialect)
		}
	}
}
