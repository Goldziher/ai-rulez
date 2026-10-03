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
						"model":       "anthropic/claude-sonnet-4",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .opencode, .opencode/skills, .opencode/agents, AGENTS.md, opencode.json = 5
	// Skill: dir + SKILL.md = 2
	// Agent: .md = 1
	if len(outputs) != 8 {
		t.Errorf("Generate() got %d outputs, want 8", len(outputs))
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
			if !strings.Contains(o.Content, "model: anthropic/claude-sonnet-4") {
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
	if doc["$schema"] != opencodeSchemaURL {
		t.Errorf("$schema = %v, want %s", doc["$schema"], opencodeSchemaURL)
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

// TestOpencodePresetGenerator_FullyOwnedWhenOnlyAiRulezKeys pins that owning
// $schema keeps a generated opencode.json in the gitignore/manifest set: a fresh
// document, or one carrying only ai-rulez's own keys, must not be reported as
// partially owned (#185).
func TestOpencodePresetGenerator_FullyOwnedWhenOnlyAiRulezKeys(t *testing.T) {
	dir := t.TempDir()
	g := &OpencodePresetGenerator{}
	cfg := &config.Config{
		Name:       "test",
		MCPServers: map[string]*config.MCPServer{"ai-rulez": {Name: "ai-rulez", Command: "npx"}},
	}
	path := filepath.Join(dir, "opencode.json")

	// Fresh: nothing on disk.
	fresh, err := g.renderMCPConfig(path, cfg)
	if err != nil {
		t.Fatalf("renderMCPConfig(fresh): %v", err)
	}
	if fresh.PartiallyOwned {
		t.Error("a freshly generated opencode.json must be fully owned")
	}
	if !strings.Contains(fresh.Body, `"$schema": "`+opencodeSchemaURL+`"`) {
		t.Errorf("fresh document must carry $schema, got:\n%s", fresh.Body)
	}

	// A file whose only keys are ai-rulez's is still fully owned.
	seed := `{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "servers": {
      "old": { "type": "local", "command": ["x"] }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	merged, err := g.renderMCPConfig(path, cfg)
	if err != nil {
		t.Fatalf("renderMCPConfig(merge): %v", err)
	}
	if merged.PartiallyOwned {
		t.Error("a document holding only ai-rulez's keys must be fully owned")
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
	if !strings.Contains(result, "model: anthropic/claude-sonnet-4-5\n") || !strings.Contains(result, "variant: high") {
		t.Errorf("expected plain model plus a separate variant, got:\n%s", result)
	}
	if strings.Contains(result, "claude-sonnet-4-5#") {
		t.Errorf("markdown agents must not carry model#variant, got:\n%s", result)
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

func TestOpencodePresetGenerator_AgentEmitsTopLevelNumericSampling(t *testing.T) {
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
	if !strings.Contains(result, "\ntemperature: 0.2\n") || !strings.Contains(result, "\ntop_p: 0.9\n") {
		t.Errorf("expected top-level numeric temperature/top_p, got:\n%s", result)
	}
	if strings.Contains(result, "request:") {
		t.Errorf("did not expect request.body, got:\n%s", result)
	}
}

func TestOpencodePresetGenerator_AgentDefaultsToModeAll(t *testing.T) {
	g := &OpencodePresetGenerator{}
	agent := config.ContentFile{Name: "spawnable", Content: "x"}
	result, err := g.renderOpencodeAgentFile(agent, &config.Config{})
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	if !strings.Contains(result, "mode: all") {
		t.Errorf("a generated agent must default to mode: all so it is spawnable as a subagent, got:\n%s", result)
	}
}

func TestOpencodePresetGenerator_OmitsConfiguredAgentFields(t *testing.T) {
	g := &OpencodePresetGenerator{}
	cfg := &config.Config{
		Defaults: &config.DefaultsConfig{
			Effort:          "high",
			OmitAgentFields: []string{"model", "effort", "description"},
		},
	}
	agent := config.ContentFile{
		Name:    "bare",
		Content: "x",
		Metadata: &config.Metadata{
			Extra: map[string]string{"description": "d", "model": "anthropic/claude-sonnet-4-5"},
		},
	}
	result, err := g.renderOpencodeAgentFile(agent, cfg)
	if err != nil {
		t.Fatalf("renderOpencodeAgentFile() error: %v", err)
	}
	for _, unwanted := range []string{"model:", "variant:", "description:"} {
		if strings.Contains(result, unwanted) {
			t.Errorf("omit_agent_fields should suppress %q, got:\n%s", unwanted, result)
		}
	}
}

func TestIsProviderQualifiedModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  bool
	}{
		{"anthropic/claude-sonnet-4-5", true},
		{"openrouter/anthropic/claude-sonnet-4", true},
		{"anthropic/claude-sonnet-4-5#high", true},
		{"sonnet", false},
		{"opus", false},
		{"claude-sonnet-4", false},
		{"/claude", false},
		{"anthropic/", false},
		{"anthropic/claude#", false},
		{"a#b/c", false},
		{"p//m", false},
		{"anthropic/claude/", false},
		{" anthropic/claude", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			if got := IsProviderQualifiedModel(tt.model); got != tt.want {
				t.Errorf("IsProviderQualifiedModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestOpencodeAgentFrontmatter_ModelValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		extra     map[string]string
		defaults  *config.DefaultsConfig
		wantModel string
		wantNone  bool
		wantVar   string
	}{
		{name: "unqualified alias is omitted", extra: map[string]string{"model": "sonnet"}, wantNone: true},
		{
			name:     "unqualified alias keeps effort as a bare variant",
			extra:    map[string]string{"model": "opus"},
			defaults: &config.DefaultsConfig{Effort: "high"},
			wantNone: true,
			wantVar:  "high",
		},
		{
			name:      "provider-qualified model passes through",
			extra:     map[string]string{"model": "anthropic/claude-sonnet-4-5"},
			wantModel: "anthropic/claude-sonnet-4-5",
		},
		{
			name:      "opencode_model wins over an unqualified legacy model",
			extra:     map[string]string{"model": "sonnet", "opencode_model": "opencode-go/deepseek-v4.1-flash"},
			wantModel: "opencode-go/deepseek-v4.1-flash",
		},
		{
			name:      "defaults.model_by_preset wins over an unqualified legacy model",
			extra:     map[string]string{"model": "sonnet"},
			defaults:  &config.DefaultsConfig{ModelByPreset: map[string]string{"opencode": "anthropic/claude-opus-4-1"}},
			wantModel: "anthropic/claude-opus-4-1",
		},
		{
			name:      "a source model variant is split off and wins over effort",
			extra:     map[string]string{"model": "anthropic/claude-sonnet-4-5#low"},
			defaults:  &config.DefaultsConfig{Effort: "high"},
			wantModel: "anthropic/claude-sonnet-4-5",
			wantVar:   "low",
		},
		{
			name:      "effort becomes a separate variant key",
			extra:     map[string]string{"model": "anthropic/claude-sonnet-4-5"},
			defaults:  &config.DefaultsConfig{Effort: "high"},
			wantModel: "anthropic/claude-sonnet-4-5",
			wantVar:   "high",
		},
		{name: "empty provider segment is omitted", extra: map[string]string{"model": "p//m"}, wantNone: true},
		{name: "uppercase alias is omitted", extra: map[string]string{"model": "Sonnet"}, wantNone: true},
		{
			name:      "surrounding whitespace is trimmed",
			extra:     map[string]string{"model": "  anthropic/claude-sonnet-4-5 \t"},
			wantModel: "anthropic/claude-sonnet-4-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := &OpencodePresetGenerator{}
			agent := config.ContentFile{Name: "a", Metadata: &config.Metadata{Extra: tt.extra}}
			fm := g.buildOpencodeAgentFrontmatter(agent, &config.Config{Defaults: tt.defaults})

			model, hasModel := fm["model"]
			if tt.wantNone && hasModel {
				t.Errorf("model = %v, want it omitted", model)
			}
			if tt.wantModel != "" && model != tt.wantModel {
				t.Errorf("model = %v, want %q", model, tt.wantModel)
			}
			if got, _ := fm["variant"].(string); got != tt.wantVar {
				t.Errorf("variant = %q, want %q", got, tt.wantVar)
			}
		})
	}
}

func TestOpencodeAgentFrontmatter_TypedScalars(t *testing.T) {
	t.Parallel()

	g := &OpencodePresetGenerator{}
	agent := config.ContentFile{Name: "a", Metadata: &config.Metadata{Extra: map[string]string{
		"hidden": "true", "temperature": "0.2", "top_p": "0.9",
	}}}
	fm := g.buildOpencodeAgentFrontmatter(agent, &config.Config{})

	if fm["hidden"] != true {
		t.Errorf("hidden = %#v, want boolean true", fm["hidden"])
	}
	if fm["temperature"] != 0.2 || fm["top_p"] != 0.9 {
		t.Errorf("temperature/top_p = %#v/%#v, want top-level numbers", fm["temperature"], fm["top_p"])
	}

	bad := config.ContentFile{Name: "b", Metadata: &config.Metadata{Extra: map[string]string{
		"hidden": "yes please", "temperature": "warm",
	}}}
	fm = g.buildOpencodeAgentFrontmatter(bad, &config.Config{})
	if _, ok := fm["hidden"]; ok {
		t.Errorf("non-boolean hidden should be omitted, got %#v", fm["hidden"])
	}
	if _, ok := fm["temperature"]; ok {
		t.Errorf("non-numeric temperature should be omitted, got %#v", fm["temperature"])
	}
}

// opencode.json lists AGENTS.local.md in instructions whenever the preset is on:
// OpenCode never reads it otherwise, and a missing listed file is skipped silently.
func TestOpencodePresetGenerator_InstructionsListLocalFile(t *testing.T) {
	tests := []struct {
		name        string
		existing    string
		servers     bool
		want        []any
		wantPartial bool
	}{
		{name: "fresh document", want: []any{"AGENTS.local.md"}},
		{name: "fresh document with servers", servers: true, want: []any{"AGENTS.local.md"}},
		{
			name:     "user entries are kept and ours is added once",
			existing: `{"instructions":["CONTRIBUTING.md","docs/*.md"]}`,
			want:     []any{"CONTRIBUTING.md", "docs/*.md", "AGENTS.local.md"}, wantPartial: true,
		},
		{
			name:     "already listed stays single",
			existing: `{"instructions":["AGENTS.local.md","CONTRIBUTING.md"]}`,
			want:     []any{"AGENTS.local.md", "CONTRIBUTING.md"}, wantPartial: true,
		},
		{
			name:     "only our entry is fully owned",
			existing: `{"$schema":"https://opencode.ai/config.json","instructions":["AGENTS.local.md"]}`,
			want:     []any{"AGENTS.local.md"},
		},
		{
			name:     "other user keys survive",
			existing: `{"model":"anthropic/claude-sonnet-4-5"}`,
			want:     []any{"AGENTS.local.md"}, wantPartial: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			path := filepath.Join(dir, "opencode.json")
			if tt.existing != "" {
				if err := os.WriteFile(path, []byte(tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.Config{Name: "test"}
			if tt.servers {
				cfg.MCPServers = map[string]*config.MCPServer{"s": {Name: "s", Command: "npx"}}
			}

			// Act
			result, err := (&OpencodePresetGenerator{}).renderMCPConfig(path, cfg)

			// Assert
			if err != nil {
				t.Fatalf("renderMCPConfig: %v", err)
			}
			var doc map[string]any
			if err := json.Unmarshal([]byte(result.Body), &doc); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if got := doc["instructions"]; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("instructions = %v, want %v", got, tt.want)
			}
			if result.PartiallyOwned != tt.wantPartial {
				t.Errorf("PartiallyOwned = %v, want %v", result.PartiallyOwned, tt.wantPartial)
			}
		})
	}
}

func TestOpencodePresetGenerator_WritesConfigWithoutMCPServers(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Config
		wantWrite bool
	}{
		{name: "project root", cfg: &config.Config{Name: "test"}, wantWrite: true},
		{
			name: "monorepo scope without servers",
			cfg:  &config.Config{Name: "test", Run: &config.RunState{Scope: &config.ScopeRun{}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			outputs, err := (&OpencodePresetGenerator{}).Generate(&config.ContentTree{}, "/test", tt.cfg)

			// Assert
			if err != nil {
				t.Fatalf("Generate() error: %v", err)
			}
			found := false
			for _, o := range outputs {
				found = found || filepath.ToSlash(o.Path) == "/test/opencode.json"
			}
			if found != tt.wantWrite {
				t.Errorf("opencode.json written = %v, want %v", found, tt.wantWrite)
			}
		})
	}
}
