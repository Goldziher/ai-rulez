package presets

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
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
			baseDir: "/test",
			// No MCP servers in cfg, so no .agents/settings.json: .agents,
			// .agents/skills, .agents/agents, GEMINI.md
			wantOutputs: 4,
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
			wantOutputs: 6, // 4 base + skill dir + SKILL.md
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
			wantOutputs: 5, // 4 base + agent .md
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
			wantOutputs: 7, // 4 base + skill dir + SKILL.md + agent .md
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &AntigravityPresetGenerator{}
			cfg := &config.Config{Name: "test-project", Rules: &config.RulesConfig{Mode: config.RulesModeInline}}

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
		filepath.Join("/base", ".agents", "rules"),
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
	cfg := &config.Config{
		Name: "test", Rules: &config.RulesConfig{Mode: config.RulesModeInline},
		MCPServers: map[string]*config.MCPServer{"configured": {Command: "npx"}},
	}

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
		"/test/.agents/mcp_config.json",
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

	fm := g.buildAgentFrontmatter(agent, &config.Config{})

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

	rendered, err := g.renderSettingsJSON("", cfg)
	if err != nil {
		t.Fatalf("renderSettingsJSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(rendered.Body), &parsed); err != nil {
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

func antigravityRuleCfg(mode string, explicit bool, presets ...string) *config.Config {
	cfg := &config.Config{Name: "proj", ConfigDir: "/test/.ai-rulez", ConfigDirName: ".ai-rulez"}
	for _, p := range presets {
		cfg.Presets = append(cfg.Presets, config.Preset{BuiltIn: p})
	}
	if explicit {
		cfg.Rules = &config.RulesConfig{ModeByPreset: map[string]string{"antigravity": mode}}
	} else if mode != "" {
		cfg.Rules = &config.RulesConfig{Mode: mode}
	}
	return cfg
}

func antigravityOutputMap(t *testing.T, content *config.ContentTree, cfg *config.Config) map[string]string {
	t.Helper()
	outs, err := (&AntigravityPresetGenerator{}).Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	m := map[string]string{}
	for _, o := range outs {
		if !o.IsDir {
			m[filepath.ToSlash(o.Path)] = o.Content
		}
	}
	return m
}

func antigravityRuleFiles(m map[string]string) []string {
	var names []string
	for p := range m {
		if strings.HasPrefix(p, "/test/.agents/rules/") {
			names = append(names, strings.TrimPrefix(p, "/test/.agents/rules/"))
		}
	}
	return names
}

func TestAntigravity_RulesSplit(t *testing.T) {
	tests := []struct {
		name     string
		rule     config.ContentFile
		wantFile string
		wantAll  []string
	}{
		{
			name:     "unscoped rule is always_on",
			rule:     config.ContentFile{Name: "style", Content: "Be tidy."},
			wantFile: "style.md",
			wantAll:  []string{"trigger: always_on", "# style", "Be tidy."},
		},
		{
			name: "scoped rule uses glob trigger",
			rule: config.ContentFile{
				Name: "go-style", Content: "Use gofmt.",
				Metadata: &config.Metadata{Paths: []string{"**/*.go", "cmd/**"}},
			},
			wantFile: "go-style.md",
			wantAll:  []string{"trigger: glob", "globs: '**/*.go,cmd/**'"},
		},
		{
			name: "auto rule uses model_decision",
			rule: config.ContentFile{
				Name: "api", Content: "Design APIs.",
				Metadata: &config.Metadata{Activation: "auto", Extra: map[string]string{"description": "API work"}},
			},
			wantFile: "api.md",
			wantAll:  []string{"trigger: model_decision", "description: API work"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			content := &config.ContentTree{Rules: []config.ContentFile{tt.rule}}
			cfg := antigravityRuleCfg("split", false)

			// Act
			m := antigravityOutputMap(t, content, cfg)

			// Assert
			got, ok := m["/test/.agents/rules/"+tt.wantFile]
			if !ok {
				t.Fatalf("missing %s in %v", tt.wantFile, antigravityRuleFiles(m))
			}
			for _, want := range tt.wantAll {
				if !strings.Contains(got, want) {
					t.Errorf("rule file missing %q:\n%s", want, got)
				}
			}
			if strings.Contains(m["/test/GEMINI.md"], "## Rules") {
				t.Errorf("GEMINI.md should not inline split rules:\n%s", m["/test/GEMINI.md"])
			}
		})
	}
}

func TestAntigravity_InlineMode(t *testing.T) {
	// Arrange
	content := &config.ContentTree{Rules: []config.ContentFile{
		{Name: "plain", Content: "Plain body."},
		{Name: "scoped", Content: "Scoped body.", Metadata: &config.Metadata{Paths: []string{"src/**"}}},
	}}

	// Act
	m := antigravityOutputMap(t, content, antigravityRuleCfg("inline", false))

	// Assert
	files := antigravityRuleFiles(m)
	if len(files) != 1 || files[0] != "scoped.md" {
		t.Fatalf("rule files = %v, want [scoped.md]", files)
	}
	gemini := m["/test/GEMINI.md"]
	if !strings.Contains(gemini, "Plain body.") {
		t.Errorf("GEMINI.md should inline the unscoped rule:\n%s", gemini)
	}
	if strings.Contains(gemini, "Scoped body.") {
		t.Errorf("GEMINI.md should not inline the scoped rule:\n%s", gemini)
	}
}

func TestAntigravityGemini_SharedRootDemotion(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Config
		wantFiles int
		wantWarn  string
	}{
		{"gemini enabled, mode not explicit", antigravityRuleCfg("split", false, "antigravity", "gemini"), 0, ""},
		{"gemini enabled, explicit split", antigravityRuleCfg("split", true, "antigravity", "gemini"), 1, "loaded twice"},
		{"gemini absent", antigravityRuleCfg("split", false, "antigravity"), 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var warned []string
			routing, _ := antigravityRouting(tt.cfg, func(msg string, _ ...any) { warned = append(warned, msg) })
			content := &config.ContentTree{Rules: []config.ContentFile{{Name: "style", Content: "Be tidy."}}}

			// Act
			m := antigravityOutputMap(t, content, tt.cfg)

			// Assert
			if got := len(antigravityRuleFiles(m)); got != tt.wantFiles {
				t.Errorf("rule files = %d, want %d", got, tt.wantFiles)
			}
			inlined := strings.Contains(m["/test/GEMINI.md"], "Be tidy.")
			if inlined != (tt.wantFiles == 0) {
				t.Errorf("GEMINI.md inlined = %v with %d rule files", inlined, tt.wantFiles)
			}
			if (routing == rulefiles.RoutingNone) != (tt.wantFiles == 0) {
				t.Errorf("routing = %v with %d rule files", routing, tt.wantFiles)
			}
			if tt.wantWarn == "" && len(warned) != 0 {
				t.Errorf("unexpected warnings %v", warned)
			}
			if tt.wantWarn != "" && (len(warned) != 1 || !strings.Contains(warned[0], tt.wantWarn)) {
				t.Errorf("warnings = %v, want one containing %q", warned, tt.wantWarn)
			}
		})
	}
}

func TestAntigravity_ScopedRulesGoToRootFolder(t *testing.T) {
	// Arrange
	content := &config.ContentTree{Rules: []config.ContentFile{
		{Name: "plain", Content: "Plain body."},
		{Name: "scoped", Content: "Scoped body.", Metadata: &config.Metadata{Paths: []string{"src/**"}}},
	}}
	cfg := antigravityRuleCfg("split", true)
	cfg.Run = &config.RunState{Scope: &config.ScopeRun{Path: "services/api", Slug: "services-api", RootDir: "/test"}}

	// Act
	m := antigravityOutputMap(t, content, cfg)

	// Assert
	files := antigravityRuleFiles(m)
	sort.Strings(files)
	if want := []string{"services-api--plain.md", "services-api--scoped.md"}; !reflect.DeepEqual(files, want) {
		t.Errorf("rule files = %v, want %v", files, want)
	}
	if got := m["/test/.agents/rules/services-api--scoped.md"]; !strings.Contains(got, "services/api/src/**") {
		t.Errorf("scoped rule misses prefixed glob:\n%s", got)
	}
}

func TestAntigravity_MaxCharsWarns(t *testing.T) {
	// Arrange
	var msgs []string
	restore := rulefiles.SetWarnSink(func(msg string, _ ...any) { msgs = append(msgs, msg) })
	t.Cleanup(restore)
	big := strings.Repeat("x", antigravityRuleMaxChars+1)
	content := &config.ContentTree{Rules: []config.ContentFile{{Name: "big", Content: big}}}

	// Act
	m := antigravityOutputMap(t, content, antigravityRuleCfg("split", false))

	// Assert
	if len(msgs) != 1 || !strings.Contains(msgs[0], "limit") {
		t.Errorf("warnings = %v, want one limit warning", msgs)
	}
	if !strings.Contains(m["/test/.agents/rules/big.md"], big) {
		t.Error("oversized rule must not be truncated")
	}
}

func TestAntigravityAgentFrontmatter(t *testing.T) {
	tests := []struct {
		name  string
		agent config.Metadata
		want  map[string]any
		gone  []string
	}{
		{"alias maps by tier and gemini keys drop", config.Metadata{
			Tools: []string{"Read", "Bash", "WebFetch"},
			Extra: map[string]string{"description": "d", "model": "sonnet", "temperature": "0.2", "kind": "local", "max_turns": "3"},
		}, map[string]any{"description": "d", "model": "pro", "tools": []string{"view_file", "run_command"}},
			[]string{"temperature", "kind", "max_turns", "timeout_mins"}},
		{"haiku is flash, inherit stays", config.Metadata{Extra: map[string]string{"model": "haiku"}},
			map[string]any{"model": "flash"}, nil},
		{"unknown model drops", config.Metadata{Extra: map[string]string{"model": "gpt-5"}}, nil, []string{"model"}},
		{"no mappable tool writes none", config.Metadata{Tools: []string{"TodoWrite"}}, nil, []string{"tools"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			fm := (&AntigravityPresetGenerator{}).buildAgentFrontmatter(
				config.ContentFile{Name: "a", Metadata: &tt.agent}, &config.Config{})

			// Assert
			for k, v := range tt.want {
				if !reflect.DeepEqual(fm[k], v) {
					t.Errorf("%s = %v, want %v", k, fm[k], v)
				}
			}
			for _, k := range tt.gone {
				if _, ok := fm[k]; ok {
					t.Errorf("%s must be absent", k)
				}
			}
		})
	}
}

func TestAntigravityMCPServers_SelfServerIsOptIn(t *testing.T) {
	declared := map[string]*config.MCPServer{"x": {Command: "npx"}}
	off := antigravityMCPServers(&config.Config{MCPServers: declared})
	if _, ok := off["ai-rulez"]; ok {
		t.Errorf("the ai-rulez server must not be written unless [mcp] self_server is set")
	}
	on := antigravityMCPServers(&config.Config{MCPServers: declared, MCP: &config.MCPConfig{SelfServer: true}})
	self, ok := on["ai-rulez"].(map[string]any)
	if !ok || self["command"] != "npx" {
		t.Fatalf("self_server must add the ai-rulez entry, got %v", on["ai-rulez"])
	}
	if _, typed := self["type"]; typed {
		t.Errorf("Antigravity entries carry no type key")
	}
}
