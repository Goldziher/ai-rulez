package presets

import (
	"path/filepath"
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
	if strings.Contains(mcpFile, "remote") {
		t.Errorf("mcp.jsonc should skip remote servers:\n%s", mcpFile)
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
