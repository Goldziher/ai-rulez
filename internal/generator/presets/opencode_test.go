package presets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestOpencodePresetGenerator_GetName(t *testing.T) {
	g := &OpencodePresetGenerator{}
	if got := g.GetName(); got != "opencode" {
		t.Errorf("GetName() = %q, want %q", got, "opencode")
	}
}

func TestOpencodePresetGenerator_Generate_WithSkillsAndAgents(t *testing.T) {
	g := &OpencodePresetGenerator{}
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
		Skills: []config.ContentFile{
			{
				Name:    "deploy",
				Content: "Deploy skill",
				Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
			},
		},
		Agents: []config.ContentFile{
			{
				Name:    "reviewer",
				Content: "Review code changes.",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"description": "Reviews code",
						"mode":        "subagent",
						"model":       "claude-sonnet-4",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .opencode, .opencode/skills, .opencode/agents, AGENTS.md = 4
	// Skill: dir + SKILL.md = 2
	// Agent: .md = 1
	if len(outputs) != 7 {
		t.Errorf("Generate() got %d outputs, want 7", len(outputs))
	}

	// Verify skill
	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".opencode/skills/deploy/SKILL.md") {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Error("Expected .opencode/skills/deploy/SKILL.md")
	}

	// Verify agent
	var foundAgent bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".opencode/agents/reviewer.md") {
			foundAgent = true
			if strings.Contains(o.Content, "name: reviewer") {
				t.Error("Agent frontmatter must not carry a name key; the filename is the ID")
			}
			if !strings.Contains(o.Content, "description: Reviews code") {
				t.Error("Agent file should contain description")
			}
			if !strings.Contains(o.Content, "mode: subagent") {
				t.Error("Agent file should contain mode")
			}
			if !strings.Contains(o.Content, "model: claude-sonnet-4") {
				t.Error("Agent file should contain model")
			}
		}
	}
	if !foundAgent {
		t.Error("Expected .opencode/agents/reviewer.md")
	}
}

func TestOpencodePresetGenerator_GeneratesV2MCPConfig(t *testing.T) {
	g := &OpencodePresetGenerator{}
	enabled := true
	cfg := &config.Config{
		Name: "test",
		MCPServers: map[string]*config.MCPServer{
			"local-server": {
				Name:    "local-server",
				Command: "npx",
				Args:    []string{"-y", "example"},
				Env:     map[string]string{"TOKEN": "abc"},
				Enabled: &enabled,
			},
			"remote-server": {
				Name:      "remote-server",
				Transport: config.TransportHTTP,
				URL:       "https://mcp.example.com/mcp",
			},
		},
	}

	outputs, err := g.Generate(&config.ContentTree{}, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var mcpOutput *config.OutputFile
	for i := range outputs {
		if filepath.ToSlash(outputs[i].Path) == "/test/opencode.json" {
			mcpOutput = &outputs[i]
		}
	}
	if mcpOutput == nil {
		t.Fatal("expected opencode.json MCP output")
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(mcpOutput.Content), &doc); err != nil {
		t.Fatalf("opencode.json is not valid JSON: %v", err)
	}
	mcp, ok := doc["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested mcp object, got: %v", doc)
	}
	servers, ok := mcp["servers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp.servers object, got: %v", mcp)
	}

	local := servers["local-server"].(map[string]any)
	if local["type"] != "local" {
		t.Errorf("local server type = %v, want local", local["type"])
	}
	if local["disabled"] != false {
		t.Errorf("local server disabled = %v, want false", local["disabled"])
	}
	cmd, ok := local["command"].([]any)
	if !ok || len(cmd) != 3 || cmd[0] != "npx" {
		t.Errorf("local command = %v, want [npx -y example]", local["command"])
	}
	if local["environment"].(map[string]any)["TOKEN"] != "abc" {
		t.Errorf("local environment = %v", local["environment"])
	}

	remote := servers["remote-server"].(map[string]any)
	if remote["type"] != "remote" {
		t.Errorf("remote server type = %v, want remote", remote["type"])
	}
	if remote["url"] != "https://mcp.example.com/mcp" {
		t.Errorf("remote url = %v", remote["url"])
	}
}

func TestOpencodePresetGenerator_MCPDisabledIsInverted(t *testing.T) {
	g := &OpencodePresetGenerator{}
	disabled := false
	cfg := &config.Config{
		Name: "test",
		MCPServers: map[string]*config.MCPServer{
			"off": {Name: "off", Command: "x", Enabled: &disabled},
		},
	}

	outputs, err := g.Generate(&config.ContentTree{}, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	var body string
	for _, o := range outputs {
		if filepath.ToSlash(o.Path) == "/test/opencode.json" {
			body = o.Content
		}
	}
	if !strings.Contains(body, `"disabled": true`) {
		t.Errorf("an explicitly disabled server must render disabled:true, got:\n%s", body)
	}
}

func TestOpencodePresetGenerator_MCPMergePreservesUserKeys(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, "opencode.json")
	existing := `{
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-sonnet-4-5",
  "mcp": {
    "timeout": {
      "catalog": 30000
    },
    "servers": {
      "user-server": {
        "type": "local",
        "command": ["user-cmd"]
      }
    }
  }
}
`
	if err := os.WriteFile(mcpPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	g := &OpencodePresetGenerator{}
	cfg := &config.Config{
		Name: "test",
		MCPServers: map[string]*config.MCPServer{
			"ai-rulez": {Name: "ai-rulez", Command: "npx", Args: []string{"-y", "ai-rulez", "mcp"}},
		},
	}

	result, err := g.renderMCPConfig(mcpPath, cfg)
	if err != nil {
		t.Fatalf("renderMCPConfig: %v", err)
	}
	if !result.PartiallyOwned {
		t.Error("a document with the consumer's own keys must be PartiallyOwned")
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(result.Body), &doc); err != nil {
		t.Fatalf("merged document is not valid JSON: %v", err)
	}
	if doc["model"] != "anthropic/claude-sonnet-4-5" {
		t.Errorf("top-level model key was dropped: %v", doc)
	}
	mcp := doc["mcp"].(map[string]any)
	if _, ok := mcp["timeout"]; !ok {
		t.Errorf("sibling mcp.timeout was dropped: %v", mcp)
	}
	servers := mcp["servers"].(map[string]any)
	if _, ok := servers["ai-rulez"]; !ok {
		t.Errorf("owned server missing: %v", servers)
	}
	if _, ok := servers["user-server"]; ok {
		t.Errorf("owned mcp.servers must replace the whole map, found user-server: %v", servers)
	}
}

func TestOpencodePresetGenerator_GetOutputPaths(t *testing.T) {
	g := &OpencodePresetGenerator{}
	paths := g.GetOutputPaths("/base")

	wantPaths := []string{
		filepath.Join("/base", "AGENTS.md"),
		filepath.Join("/base", ".opencode"),
		filepath.Join("/base", ".opencode", "skills"),
		filepath.Join("/base", ".opencode", "agents"),
	}

	if len(paths) != len(wantPaths) {
		t.Fatalf("GetOutputPaths() returned %d paths, want %d", len(paths), len(wantPaths))
	}

	for i, want := range wantPaths {
		if paths[i] != want {
			t.Errorf("GetOutputPaths()[%d] = %q, want %q", i, paths[i], want)
		}
	}
}

func TestOpencodePresetGenerator_AgentEmitsPerAgentVariant(t *testing.T) {
	g := &OpencodePresetGenerator{}
	cfg := &config.Config{
		Name:     "test",
		Defaults: &config.DefaultsConfig{Effort: "medium"},
	}

	agent := config.ContentFile{
		Name:    "deep-thinker",
		Content: "Think hard.",
		Metadata: &config.Metadata{
			Effort: "high",
			Extra:  map[string]string{"description": "Long-horizon reasoning"},
		},
	}

	result, err := g.renderOpencodeAgentFile(agent, cfg)
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	if !strings.Contains(result, "variant: high") {
		t.Errorf("expected variant: high in frontmatter, got:\n%s", result)
	}
	if strings.Contains(result, "reasoningEffort") {
		t.Errorf("legacy reasoningEffort must not be emitted, got:\n%s", result)
	}
}

func TestOpencodePresetGenerator_AgentInheritsGlobalVariant(t *testing.T) {
	g := &OpencodePresetGenerator{}
	cfg := &config.Config{
		Name:     "test",
		Defaults: &config.DefaultsConfig{Effort: "medium"},
	}

	agent := config.ContentFile{Name: "default-agent", Content: "default"}
	result, err := g.renderOpencodeAgentFile(agent, cfg)
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	if !strings.Contains(result, "variant: medium") {
		t.Errorf("expected variant: medium in frontmatter, got:\n%s", result)
	}
}

func TestOpencodePresetGenerator_AgentJoinsModelAndVariant(t *testing.T) {
	g := &OpencodePresetGenerator{}
	cfg := &config.Config{
		Name:     "test",
		Defaults: &config.DefaultsConfig{Effort: "high"},
	}

	agent := config.ContentFile{
		Name:    "modeled",
		Content: "x",
		Metadata: &config.Metadata{
			Extra: map[string]string{"model": "anthropic/claude-sonnet-4-5"},
		},
	}
	result, err := g.renderOpencodeAgentFile(agent, cfg)
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	if !strings.Contains(result, "model: anthropic/claude-sonnet-4-5#high") {
		t.Errorf("expected model#variant in frontmatter, got:\n%s", result)
	}
}

func TestOpencodePresetGenerator_AgentOmitsVariantWhenNoneResolved(t *testing.T) {
	g := &OpencodePresetGenerator{}
	agent := config.ContentFile{Name: "plain", Content: "no effort"}
	result, err := g.renderOpencodeAgentFile(agent, &config.Config{})
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	if strings.Contains(result, "variant") || strings.Contains(result, "reasoningEffort") {
		t.Errorf("did not expect an effort field in frontmatter, got:\n%s", result)
	}
}

func TestOpencodePresetGenerator_AgentEffortMaxMapsToHigh(t *testing.T) {
	g := &OpencodePresetGenerator{}
	cfg := &config.Config{
		Name:     "test",
		Defaults: &config.DefaultsConfig{Effort: "max"},
	}

	agent := config.ContentFile{Name: "tier-test", Content: "x"}
	result, err := g.renderOpencodeAgentFile(agent, cfg)
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	if !strings.Contains(result, "variant: high") {
		t.Errorf("expected max → high, got:\n%s", result)
	}
}

func TestOpencodePresetGenerator_AgentMovesSamplingUnderRequestBody(t *testing.T) {
	g := &OpencodePresetGenerator{}
	agent := config.ContentFile{
		Name:    "sampled",
		Content: "x",
		Metadata: &config.Metadata{
			Extra: map[string]string{"temperature": "0.2", "top_p": "0.9"},
		},
	}
	result, err := g.renderOpencodeAgentFile(agent, &config.Config{})
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	if strings.Contains(result, "\ntemperature:") || strings.Contains(result, "\ntop_p:") {
		t.Errorf("temperature/top_p must not be top-level in v2, got:\n%s", result)
	}
	if !strings.Contains(result, "request:") || !strings.Contains(result, "body:") {
		t.Errorf("expected request.body in frontmatter, got:\n%s", result)
	}
}
