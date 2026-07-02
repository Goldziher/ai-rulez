package presets

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestAntigravityPresetGenerator_GetName(t *testing.T) {
	g := &AntigravityPresetGenerator{}
	if got := g.GetName(); got != "antigravity" {
		t.Errorf("GetName() = %q, want %q", got, "antigravity")
	}
}

func TestAntigravityPresetGenerator_Generate(t *testing.T) {
	tests := []struct {
		name        string
		content     *config.ContentTree
		baseDir     string
		wantOutputs int
		wantErr     bool
	}{
		{
			name: "generates basic structure",
			content: &config.ContentTree{
				Rules: []config.ContentFile{
					{Name: "rule1", Content: "Rule content"},
				},
			},
			baseDir:     "/test",
			wantOutputs: 5, // .agents, .agents/skills, .agents/agents, .agents/settings.json, GEMINI.md
			wantErr:     false,
		},
		{
			name: "generates with skills",
			content: &config.ContentTree{
				Skills: []config.ContentFile{
					{
						Name:    "my-skill",
						Content: "Skill instructions",
						Path:    "/test/.ai-rulez/skills/my-skill/SKILL.md",
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 7, // 5 base + skill dir + SKILL.md
			wantErr:     false,
		},
		{
			name: "generates with agents",
			content: &config.ContentTree{
				Agents: []config.ContentFile{
					{
						Name:    "security-auditor",
						Content: "You are a security auditor.",
						Metadata: &config.Metadata{
							Extra: map[string]string{
								"description": "Audits code for security issues",
							},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 6, // 5 base + agent .md
			wantErr:     false,
		},
		{
			name: "generates with skills and agents",
			content: &config.ContentTree{
				Skills: []config.ContentFile{
					{
						Name:    "deploy",
						Content: "Deploy instructions",
						Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
					},
				},
				Agents: []config.ContentFile{
					{
						Name:    "reviewer",
						Content: "You review code.",
						Metadata: &config.Metadata{
							Extra: map[string]string{"description": "Reviews code"},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 8, // 5 base + skill dir + SKILL.md + agent .md
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &AntigravityPresetGenerator{}
			cfg := &config.Config{Name: "test-project"}

			outputs, err := g.Generate(tt.content, tt.baseDir, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(outputs) != tt.wantOutputs {
				t.Errorf("Generate() got %d outputs, want %d", len(outputs), tt.wantOutputs)
			}
		})
	}
}

func TestAntigravityPresetGenerator_GetOutputPaths(t *testing.T) {
	g := &AntigravityPresetGenerator{}
	paths := g.GetOutputPaths("/base")

	wantPaths := []string{
		filepath.Join("/base", "GEMINI.md"),
		filepath.Join("/base", ".agents"),
		filepath.Join("/base", ".agents", "skills"),
		filepath.Join("/base", ".agents", "agents"),
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

func TestAntigravityPresetGenerator_outputStructure(t *testing.T) {
	g := &AntigravityPresetGenerator{}
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "Rule content"},
		},
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
				Content: "Review agent",
				Metadata: &config.Metadata{
					Extra: map[string]string{"description": "Reviews code"},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Verify key output paths exist
	pathSet := make(map[string]bool)
	for _, o := range outputs {
		pathSet[filepath.ToSlash(o.Path)] = true
	}

	expectedPaths := []string{
		"/test/GEMINI.md",
		"/test/.agents/settings.json",
		"/test/.agents/skills/deploy/SKILL.md",
		"/test/.agents/agents/reviewer.md",
	}

	for _, p := range expectedPaths {
		if !pathSet[p] {
			t.Errorf("Expected output path %q not found", p)
		}
	}

	// Verify GEMINI.md contains rules
	for _, o := range outputs {
		if filepath.Base(o.Path) == "GEMINI.md" {
			if !strings.Contains(o.Content, "## Rules") {
				t.Error("GEMINI.md should contain Rules section")
			}
			if !strings.Contains(o.Content, "rule1") {
				t.Error("GEMINI.md should contain rule1")
			}
		}
	}

	// Verify skill file has frontmatter
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, "deploy/SKILL.md") {
			if !strings.HasPrefix(o.Content, "---\n") {
				t.Error("SKILL.md should start with YAML frontmatter")
			}
			if !strings.Contains(o.Content, "name: deploy") {
				t.Error("SKILL.md should contain skill name")
			}
			if !strings.Contains(o.Content, "Deploy skill") {
				t.Error("SKILL.md should contain skill content")
			}
		}
	}

	// Verify agent file has frontmatter
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, "reviewer.md") {
			if !strings.HasPrefix(o.Content, "---\n") {
				t.Error("Agent file should start with YAML frontmatter")
			}
			if !strings.Contains(o.Content, "name: reviewer") {
				t.Error("Agent file should contain agent name")
			}
			if !strings.Contains(o.Content, "description: Reviews code") {
				t.Error("Agent file should contain description")
			}
			if !strings.Contains(o.Content, "Review agent") {
				t.Error("Agent file should contain agent content")
			}
		}
	}
}

func TestAntigravityPresetGenerator_buildAgentFrontmatter_DoesNotEmitEffort(t *testing.T) {
	g := &AntigravityPresetGenerator{}

	agent := config.ContentFile{
		Name: "reviewer",
		Metadata: &config.Metadata{
			Effort: "high",
			Extra: map[string]string{
				"description": "Reviews code",
			},
		},
	}

	fm := g.buildAgentFrontmatter(agent)

	// Antigravity's thinking control (thinkingLevel) is a model-variant selection
	// at the API level, not a frontmatter field the IDE reads from agent files.
	// We deliberately do not emit it here; revisit if Google publishes a frontmatter
	// schema that includes a thinking/reasoning field.
	if _, ok := fm["effort"]; ok {
		t.Errorf("effort field leaked into Antigravity agent frontmatter")
	}
	if _, ok := fm["thinking_level"]; ok {
		t.Errorf("thinking_level should not be emitted until Antigravity documents it as a frontmatter field")
	}
	if fm["description"] != "Reviews code" {
		t.Errorf("expected description to be passed through; got %v", fm["description"])
	}
}

func TestAntigravityPresetGenerator_renderSettingsJSON_Transports(t *testing.T) {
	g := &AntigravityPresetGenerator{}

	cfg := &config.Config{
		MCPServers: map[string]*config.MCPServer{
			"stdio-server": {
				Command: "npx",
				Args:    []string{"-y", "test-mcp"},
			},
			"http-server": {
				Transport: "http",
				URL:       "https://example.com/mcp",
			},
			"sse-server": {
				Transport: "sse",
				URL:       "https://example.com/sse",
			},
		},
	}

	content, err := g.renderSettingsJSON(cfg)
	if err != nil {
		t.Fatalf("renderSettingsJSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	servers := parsed["mcpServers"].(map[string]interface{})

	stdio := servers["stdio-server"].(map[string]interface{})
	if stdio["command"] != "npx" {
		t.Errorf("stdio command = %v, want npx", stdio["command"])
	}

	for _, name := range []string{"http-server", "sse-server"} {
		entry := servers[name].(map[string]interface{})
		wantURL := "https://example.com/mcp"
		if name == "sse-server" {
			wantURL = "https://example.com/sse"
		}
		if entry["serverUrl"] != wantURL {
			t.Errorf("%s serverUrl = %v, want %v", name, entry["serverUrl"], wantURL)
		}
		if _, ok := entry["command"]; ok {
			t.Errorf("%s must not contain command", name)
		}
		if _, ok := entry["args"]; ok {
			t.Errorf("%s must not contain args", name)
		}
		if _, ok := entry["transport"]; ok {
			t.Errorf("%s must not contain transport", name)
		}
		if _, ok := entry["url"]; ok {
			t.Errorf("%s must use serverUrl, not url", name)
		}
	}
}
