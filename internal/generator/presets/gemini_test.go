package presets

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestGeminiPresetGenerator_GetName(t *testing.T) {
	g := &GeminiPresetGenerator{}
	if got := g.GetName(); got != "gemini" {
		t.Errorf("GetName() = %q, want %q", got, "gemini")
	}
}

func TestGeminiPresetGenerator_Generate(t *testing.T) {
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
			wantOutputs: 6, // .gemini, .agents, .agents/skills, .agents/agents, settings.json, GEMINI.md
			wantErr:     false,
		},
		{
			name: "generates with skills",
			content: &config.ContentTree{
				Skills: []config.ContentFile{
					{
						Name:    "deploy",
						Content: "Deploy instructions",
						Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 8, // 6 base + skill dir + SKILL.md
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
			wantOutputs: 7, // 6 base + agent .md
			wantErr:     false,
		},
		{
			name: "generates with skills and agents from domains",
			content: &config.ContentTree{
				Domains: map[string]*config.Domain{
					"backend": {
						Name: "backend",
						Skills: []config.ContentFile{
							{
								Name:    "api-deploy",
								Content: "API deploy skill",
								Path:    "/test/.ai-rulez/domains/backend/skills/api-deploy/SKILL.md",
							},
						},
						Agents: []config.ContentFile{
							{
								Name:    "db-expert",
								Content: "Database expert agent",
								Metadata: &config.Metadata{
									Extra: map[string]string{"description": "Database expertise"},
								},
							},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 9, // 6 base + skill dir + SKILL.md + agent .md
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GeminiPresetGenerator{}
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

func TestGeminiPresetGenerator_renderGeminiSkillFile(t *testing.T) {
	g := &GeminiPresetGenerator{}

	skill := config.ContentFile{
		Name:    "deploy",
		Content: "Deploy to production.",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"description": "Deploys the app",
			},
		},
	}

	result := g.renderGeminiSkillFile(skill)

	if !strings.HasPrefix(result, "---\n") {
		t.Error("Expected YAML frontmatter start")
	}
	if !strings.Contains(result, "name: deploy") {
		t.Error("Expected skill name in frontmatter")
	}
	if !strings.Contains(result, "description:") {
		t.Error("Expected description in frontmatter")
	}
	if !strings.Contains(result, "Deploy to production.") {
		t.Error("Expected skill content")
	}
}

func TestGeminiPresetGenerator_renderGeminiAgentFile(t *testing.T) {
	g := &GeminiPresetGenerator{}

	agent := config.ContentFile{
		Name:    "security-auditor",
		Content: "You are a security auditor.",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"description": "Audits code for vulnerabilities",
				"model":       "gemini-3-flash",
				"kind":        "local",
			},
		},
	}

	result, err := g.renderGeminiAgentFile(agent, nil)
	if err != nil {
		t.Fatalf("renderGeminiAgentFile() error: %v", err)
	}

	if !strings.HasPrefix(result, "---\n") {
		t.Error("Expected YAML frontmatter start")
	}
	if !strings.Contains(result, "name: security-auditor") {
		t.Error("Expected agent name")
	}
	if !strings.Contains(result, "description: Audits code for vulnerabilities") {
		t.Error("Expected description")
	}
	if !strings.Contains(result, "kind: local") {
		t.Error("Expected kind field")
	}
	if !strings.Contains(result, "model: gemini-3-flash") {
		t.Error("Expected model field")
	}
	if !strings.Contains(result, "You are a security auditor.") {
		t.Error("Expected agent content")
	}
}

func TestGeminiPresetGenerator_GetOutputPaths(t *testing.T) {
	g := &GeminiPresetGenerator{}
	paths := g.GetOutputPaths("/base")

	wantPaths := []string{
		filepath.Join("/base", ".gemini"),
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

func TestGeminiPresetGenerator_renderSettingsJSON_Transports(t *testing.T) {
	g := &GeminiPresetGenerator{}

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
	if _, ok := stdio["transport"]; ok {
		t.Error("stdio entry must not contain transport")
	}

	httpServer := servers["http-server"].(map[string]interface{})
	if httpServer["httpUrl"] != "https://example.com/mcp" {
		t.Errorf("http httpUrl = %v", httpServer["httpUrl"])
	}
	if _, ok := httpServer["command"]; ok {
		t.Error("http entry must not contain command")
	}
	if _, ok := httpServer["transport"]; ok {
		t.Error("http entry must not contain transport")
	}
	if _, ok := httpServer["url"]; ok {
		t.Error("http entry must use httpUrl, not url")
	}

	sseServer := servers["sse-server"].(map[string]interface{})
	if sseServer["url"] != "https://example.com/sse" {
		t.Errorf("sse url = %v", sseServer["url"])
	}
	if _, ok := sseServer["command"]; ok {
		t.Error("sse entry must not contain command")
	}
	if _, ok := sseServer["transport"]; ok {
		t.Error("sse entry must not contain transport")
	}
}
