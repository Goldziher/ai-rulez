package presets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestXumPresetGenerator_GetName(t *testing.T) {
	g := &XumPresetGenerator{}
	if got := g.GetName(); got != "xum" {
		t.Errorf("GetName() = %q, want xum", got)
	}
}

func TestXumPresetGenerator_LocalRootFile(t *testing.T) {
	g := &XumPresetGenerator{}
	if got := g.LocalRootFile(); got != "AGENTS.local.md" {
		t.Errorf("LocalRootFile() = %q, want AGENTS.local.md", got)
	}
}

func TestXumPresetGenerator_GetOutputPaths(t *testing.T) {
	g := &XumPresetGenerator{}
	paths := g.GetOutputPaths("/test")
	want := map[string]bool{
		filepath.Join("/test", "AGENTS.md"):         false,
		filepath.Join("/test", ".xum"):              false,
		filepath.Join("/test", ".xum", "skills"):    false,
		filepath.Join("/test", ".xum", "agents"):    false,
		filepath.Join("/test", ".xum", "mcp.jsonc"): false,
	}
	for _, p := range paths {
		if _, ok := want[p]; ok {
			want[p] = true
		}
	}
	for p, seen := range want {
		if !seen {
			t.Errorf("GetOutputPaths() missing %q", p)
		}
	}
}

func TestXumPresetGenerator_Generate(t *testing.T) {
	g := &XumPresetGenerator{}
	cfg := &config.Config{
		Name: "demo",
		MCPServers: map[string]*config.MCPServer{
			"memory": {
				Name:    "memory",
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
			},
			"remote": {
				Name:      "remote",
				Transport: config.TransportHTTP,
				URL:       "https://example.com/mcp",
			},
		},
	}

	content := &config.ContentTree{
		Rules: []config.ContentFile{{Name: "code-quality", Content: "Be tidy."}},
		Skills: []config.ContentFile{
			{Name: "deploy", Content: "Deploy skill", Path: "/test/.ai-rulez/skills/deploy/SKILL.md"},
		},
		Agents: []config.ContentFile{
			{
				Name:    "reviewer",
				Content: "Review code.",
				Metadata: &config.Metadata{
					Extra: map[string]string{"description": "Reviews code"},
					Tools: []string{"file_read"},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var agentsMD, agentFile, mcpFile string
	for _, o := range outputs {
		switch {
		case filepath.ToSlash(o.Path) == "/test/AGENTS.md":
			agentsMD = o.Content
		case strings.HasSuffix(filepath.ToSlash(o.Path), ".xum/agents/reviewer.md"):
			agentFile = o.Content
		case strings.HasSuffix(filepath.ToSlash(o.Path), ".xum/mcp.jsonc"):
			mcpFile = o.Content
		}
	}

	if !strings.Contains(agentsMD, "## Rules") || !strings.Contains(agentsMD, "code-quality") {
		t.Errorf("AGENTS.md missing rules section:\n%s", agentsMD)
	}
	if !strings.Contains(agentFile, "name: reviewer") {
		t.Errorf("agent file missing name:\n%s", agentFile)
	}
	if !strings.Contains(agentFile, "tools:") || !strings.Contains(agentFile, "file_read") {
		t.Errorf("agent file missing tools:\n%s", agentFile)
	}
	if !strings.Contains(mcpFile, `"servers"`) || !strings.Contains(mcpFile, "npx -y @modelcontextprotocol/server-memory") {
		t.Errorf("mcp.jsonc missing stdio server:\n%s", mcpFile)
	}
	if !strings.Contains(mcpFile, `"remote"`) || !strings.Contains(mcpFile, `"transport": "http"`) {
		t.Errorf("mcp.jsonc missing remote server:\n%s", mcpFile)
	}
}

func TestXumThinkingLevel(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"inherit": "",
		"low":     "low",
		"medium":  "medium",
		"high":    "high",
		"xhigh":   "high",
		"max":     "high",
	}
	for in, want := range cases {
		if got := xumThinkingLevel(in); got != want {
			t.Errorf("xumThinkingLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestXumPresetGenerator_AgentSubagentRunnable(t *testing.T) {
	g := &XumPresetGenerator{}
	agent := config.ContentFile{Name: "spawnable", Content: "x"}
	result, err := g.renderAgentFile(agent, &config.Config{})
	if err != nil {
		t.Fatalf("renderAgentFile() error: %v", err)
	}
	if !strings.Contains(result, "runnable: true") {
		t.Errorf("xum agents must be subagent-runnable, got:\n%s", result)
	}
}

func renderXumMCP(t *testing.T, baseDir string, servers map[string]*config.MCPServer) (string, bool) {
	t.Helper()
	g := &XumPresetGenerator{}
	out, err := g.renderMCPConfig(baseDir, &config.Config{MCPServers: servers})
	if err != nil {
		t.Fatalf("renderMCPConfig() error: %v", err)
	}
	if out == nil {
		return "", false
	}
	return out.Content, true
}

func TestXumPresetGenerator_RenderMCPConfig(t *testing.T) {
	disabled := false
	tests := []struct {
		name    string
		servers map[string]*config.MCPServer
		want    map[string]interface{}
	}{
		{
			name: "http with headers",
			servers: map[string]*config.MCPServer{"api": {
				Transport: config.TransportHTTP, URL: "https://example.com/mcp",
				Headers: map[string]string{"Authorization": "Bearer abc"},
			}},
			want: map[string]interface{}{"api": map[string]interface{}{
				"transport": "http", "url": "https://example.com/mcp",
				"headers": map[string]interface{}{"Authorization": "Bearer abc"},
			}},
		},
		{
			name:    "sse",
			servers: map[string]*config.MCPServer{"ev": {Transport: config.TransportSSE, URL: "https://example.com/sse"}},
			want:    map[string]interface{}{"ev": map[string]interface{}{"transport": "sse", "url": "https://example.com/sse"}},
		},
		{
			name:    "stdio without env keeps string form",
			servers: map[string]*config.MCPServer{"mem": {Command: "npx", Args: []string{"-y", "pkg"}}},
			want:    map[string]interface{}{"mem": "npx -y pkg"},
		},
		{
			name: "stdio env becomes a sorted shell assignment prefix",
			servers: map[string]*config.MCPServer{"mem": {
				Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"TOKEN": "x", "API_URL": "https://e.com"},
			}},
			want: map[string]interface{}{"mem": "API_URL=https://e.com TOKEN=x npx -y pkg"},
		},
		{
			name: "stdio env values are shell-quoted",
			servers: map[string]*config.MCPServer{"mem": {
				Command: "npx", Env: map[string]string{"GREETING": "it's $HOME; rm -rf /"},
			}},
			want: map[string]interface{}{"mem": `GREETING='it'\''s $HOME; rm -rf /' npx`},
		},
		{
			name: "env names that are not shell identifiers are skipped",
			servers: map[string]*config.MCPServer{"mem": {
				Command: "npx", Env: map[string]string{"BAD-NAME": "x", "1X": "y", "OK": "z"},
			}},
			want: map[string]interface{}{"mem": "OK=z npx"},
		},
		{
			name: "mixed",
			servers: map[string]*config.MCPServer{
				"mem": {Command: "npx"},
				"api": {Transport: config.TransportHTTP, URL: "https://example.com/mcp"},
			},
			want: map[string]interface{}{
				"mem": "npx",
				"api": map[string]interface{}{"transport": "http", "url": "https://example.com/mcp"},
			},
		},
		{
			name: "disabled servers",
			servers: map[string]*config.MCPServer{
				"mem": {Command: "npx", Env: map[string]string{"K": "v"}, Enabled: &disabled},
				"api": {Transport: config.TransportHTTP, URL: "https://example.com/mcp", Enabled: &disabled},
			},
			want: map[string]interface{}{
				"mem": map[string]interface{}{
					"transport": "stdio", "command": "K=v npx", "disabled": true,
				},
				"api": map[string]interface{}{
					"transport": "http", "url": "https://example.com/mcp", "disabled": true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			body, ok := renderXumMCP(t, t.TempDir(), tt.servers)

			// Assert
			if !ok {
				t.Fatal("expected mcp.jsonc output")
			}
			var doc struct {
				Servers map[string]interface{} `json:"servers"`
			}
			if err := json.Unmarshal([]byte(body), &doc); err != nil {
				t.Fatalf("output is not valid JSON: %v\n%s", err, body)
			}
			if !reflect.DeepEqual(doc.Servers, tt.want) {
				t.Errorf("servers = %#v, want %#v", doc.Servers, tt.want)
			}
		})
	}
}

func TestXumPresetGenerator_RenderMCPConfig_NoServers(t *testing.T) {
	if _, ok := renderXumMCP(t, t.TempDir(), map[string]*config.MCPServer{"x": {Transport: config.TransportHTTP}}); ok {
		t.Error("expected no output for a remote server without a url")
	}
}

func TestXumPresetGenerator_RenderMCPConfig_MergesAndIsIdempotent(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, filepath.FromSlash(MergedDocXumMCP))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"enabledPluginServers": ["plugin:abc:x"], "servers": {"old": "echo hi"}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	servers := map[string]*config.MCPServer{
		"api": {Transport: config.TransportSSE, URL: "https://example.com/sse"},
	}

	// Act
	first, _ := renderXumMCP(t, baseDir, servers)
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	second, _ := renderXumMCP(t, baseDir, servers)

	// Assert
	if !strings.Contains(first, "enabledPluginServers") || !strings.Contains(first, "plugin:abc:x") {
		t.Errorf("user key not preserved:\n%s", first)
	}
	if !strings.Contains(first, `"sse"`) {
		t.Errorf("sse server missing:\n%s", first)
	}
	if first != second {
		t.Errorf("not idempotent:\n%s\n---\n%s", first, second)
	}
}
