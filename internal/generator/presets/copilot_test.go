package presets

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopilotPresetGenerator_GetName(t *testing.T) {
	g := &CopilotPresetGenerator{}
	if got := g.GetName(); got != "copilot" {
		t.Errorf("GetName() = %q, want %q", got, "copilot")
	}
}

func TestCopilotPresetGenerator_Generate_WithSkillsAndAgents(t *testing.T) {
	g := &CopilotPresetGenerator{}
	cfg := &config.Config{Name: "test", Rules: &config.RulesConfig{Mode: config.RulesModeInline}}

	content := &config.ContentTree{
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
					Extra: map[string]string{
						"description": "Reviews code changes",
						"model":       "gpt-5",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .github, .github/skills, .github/agents, copilot-instructions.md = 4
	// Skill: dir + SKILL.md = 2
	// Agent: .agent.md = 1
	// Commands dir = 1
	if len(outputs) != 8 {
		t.Errorf("Generate() got %d outputs, want 8", len(outputs))
	}

	// Verify agent uses .agent.md extension
	var foundAgent bool
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, ".agent.md") {
			foundAgent = true
			if !strings.Contains(o.Content, "name: reviewer") {
				t.Error("Agent file should contain name")
			}
			if !strings.Contains(o.Content, "description: Reviews code changes") {
				t.Error("Agent file should contain description")
			}
			if !strings.Contains(o.Content, "model: gpt-5") {
				t.Error("Agent file should contain model")
			}
		}
	}
	if !foundAgent {
		t.Error("Expected .agent.md file in outputs")
	}

	// Verify skill in .github/skills/
	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".github/skills/deploy/SKILL.md") {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Error("Expected .github/skills/deploy/SKILL.md in outputs")
	}
}

func TestCopilotPresetGenerator_GetOutputPaths(t *testing.T) {
	g := &CopilotPresetGenerator{}
	paths := g.GetOutputPaths("/base")

	wantPaths := []string{
		filepath.Join("/base", ".github"),
		filepath.Join("/base", ".github", "copilot-instructions.md"),
		filepath.Join("/base", ".github", "instructions"),
		filepath.Join("/base", ".github", "skills"),
		filepath.Join("/base", ".github", "agents"),
		filepath.Join("/base", ".github", "prompts"),
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

func TestCopilotPresetGenerator_shouldIncludeCommand(t *testing.T) {
	g := &CopilotPresetGenerator{}

	tests := []struct {
		name     string
		command  config.ContentFile
		expected bool
	}{
		{
			name:     "no metadata includes",
			command:  config.ContentFile{Name: "cmd"},
			expected: true,
		},
		{
			name: "no targets includes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.Metadata{},
			},
			expected: true,
		},
		{
			name: "matching target includes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.Metadata{Targets: []string{"copilot"}},
			},
			expected: true,
		},
		{
			name: "non-matching target excludes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.Metadata{Targets: []string{"claude", "cursor"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, g.shouldIncludeCommand(tt.command))
		})
	}
}

func TestCopilotPresetGenerator_renderMCPJSON(t *testing.T) {
	g := &CopilotPresetGenerator{}

	cfg := &config.Config{
		MCPServers: map[string]*config.MCPServer{
			"test-server": {
				Command: "npx",
				Args:    []string{"-y", "test-mcp"},
				Env:     map[string]string{"API_KEY": "key"},
			},
		},
	}

	rendered, err := g.renderMCPJSON("", cfg)
	require.NoError(t, err)
	assert.Contains(t, rendered.Body, "test-server")
	assert.Contains(t, rendered.Body, "npx")
	assert.Contains(t, rendered.Body, "mcpServers")
	assert.False(t, rendered.PartiallyOwned, "a document ai-rulez created holds only the owned key")

	// Verify valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(rendered.Body), &parsed)
	require.NoError(t, err)
	servers := parsed["mcpServers"].(map[string]interface{})
	assert.Len(t, servers, 1)

	stdio := servers["test-server"].(map[string]interface{})
	assert.Equal(t, "npx", stdio["command"])
	assert.NotContains(t, stdio, "type")
	assert.NotContains(t, stdio, "transport")
}

func TestCopilotPresetGenerator_renderMCPJSON_RemoteTransports(t *testing.T) {
	g := &CopilotPresetGenerator{}

	cfg := &config.Config{
		MCPServers: map[string]*config.MCPServer{
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

	rendered, err := g.renderMCPJSON("", cfg)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(rendered.Body), &parsed))
	servers := parsed["mcpServers"].(map[string]interface{})

	httpServer := servers["http-server"].(map[string]interface{})
	assert.Equal(t, "http", httpServer["type"])
	assert.Equal(t, "https://example.com/mcp", httpServer["url"])
	assert.NotContains(t, httpServer, "command")
	assert.NotContains(t, httpServer, "args")
	assert.NotContains(t, httpServer, "transport")

	sseServer := servers["sse-server"].(map[string]interface{})
	assert.Equal(t, "sse", sseServer["type"])
	assert.Equal(t, "https://example.com/sse", sseServer["url"])
	assert.NotContains(t, sseServer, "command")
	assert.NotContains(t, sseServer, "transport")
}

func copilotRuleFixture(name, body string, md *config.Metadata) config.ContentFile {
	return config.ContentFile{Name: name, Content: body, Path: "/test/.ai-rulez/rules/" + name + ".md", Metadata: md}
}

func copilotOutputByPath(outputs []config.OutputFile, suffix string) (config.OutputFile, bool) {
	for _, o := range outputs {
		if strings.HasSuffix(filepath.ToSlash(o.Path), suffix) {
			return o, true
		}
	}
	return config.OutputFile{}, false
}

func splitCopilotConfig() *config.Config {
	return &config.Config{Name: "test", Rules: &config.RulesConfig{Mode: config.RulesModeSplit}}
}

func TestCopilot_InstructionsFiles(t *testing.T) {
	tests := []struct {
		name       string
		md         *config.Metadata
		wantFM     []string
		wantAbsent []string
		wantInline bool // no applyTo possible: the rule stays in copilot-instructions.md
	}{
		{name: "always", md: nil, wantFM: []string{"applyTo: '**'"}},
		{
			name:   "glob comma-joined with brace expansion",
			md:     &config.Metadata{Globs: []string{"src/**/*.{ts,tsx}", "docs/**"}},
			wantFM: []string{"applyTo: src/**/*.ts,src/**/*.tsx,docs/**"},
		},
		{
			name:       "auto stays inline",
			md:         &config.Metadata{Activation: "auto", Extra: map[string]string{"description": "Use for API work"}},
			wantInline: true,
		},
		{
			name:       "manual stays inline",
			md:         &config.Metadata{Activation: "manual"},
			wantInline: true,
		},
		{
			name:       "negated glob dropped from applyTo",
			md:         &config.Metadata{Globs: []string{"src/**", "!src/gen/**"}},
			wantFM:     []string{"applyTo: src/**"},
			wantAbsent: []string{"!src"},
		},
		{
			name:       "only negated globs stay inline",
			md:         &config.Metadata{Globs: []string{"!src/gen/**"}},
			wantInline: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g := &CopilotPresetGenerator{}
			content := &config.ContentTree{Rules: []config.ContentFile{copilotRuleFixture("my-rule", "Body text.", tt.md)}}

			// Act
			outputs, err := g.Generate(content, "/test", splitCopilotConfig())

			// Assert
			require.NoError(t, err)
			file, ok := copilotOutputByPath(outputs, ".github/instructions/my-rule.instructions.md")
			if tt.wantInline {
				assert.False(t, ok, "rule without applyTo must not become an instructions file")
				root, found := copilotOutputByPath(outputs, ".github/copilot-instructions.md")
				require.True(t, found)
				assert.Contains(t, root.Content, "Body text.")
				return
			}
			require.True(t, ok, "instructions file missing")
			for _, want := range tt.wantFM {
				assert.Contains(t, file.Content, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, file.Content, absent)
			}
			assert.Contains(t, file.Content, "Body text.")
			_, hasDir := copilotOutputByPath(outputs, ".github/instructions")
			assert.True(t, hasDir, "instructions dir output missing")
		})
	}
}

func TestCopilot_InlineMode(t *testing.T) {
	// Arrange
	g := &CopilotPresetGenerator{}
	cfg := &config.Config{Name: "test", Rules: &config.RulesConfig{Mode: config.RulesModeInline}}
	content := &config.ContentTree{Rules: []config.ContentFile{
		copilotRuleFixture("global-rule", "Global body.", nil),
		copilotRuleFixture("scoped-rule", "Scoped body.", &config.Metadata{Globs: []string{"src/**"}}),
	}}

	// Act
	outputs, err := g.Generate(content, "/test", cfg)

	// Assert
	require.NoError(t, err)
	root, ok := copilotOutputByPath(outputs, ".github/copilot-instructions.md")
	require.True(t, ok)
	assert.Contains(t, root.Content, "## Rules")
	assert.Contains(t, root.Content, "Global body.")
	assert.NotContains(t, root.Content, "Scoped body.")

	_, ok = copilotOutputByPath(outputs, ".github/instructions/global-rule.instructions.md")
	assert.False(t, ok, "unscoped rule must stay inline")
	scoped, ok := copilotOutputByPath(outputs, ".github/instructions/scoped-rule.instructions.md")
	require.True(t, ok)
	assert.Contains(t, scoped.Content, "applyTo: src/**")
}

func TestCopilot_InlineModeNoScopedRulesWritesNoInstructionsDir(t *testing.T) {
	g := &CopilotPresetGenerator{}
	content := &config.ContentTree{Rules: []config.ContentFile{copilotRuleFixture("global-rule", "Body.", nil)}}

	outputs, err := g.Generate(content, "/test", &config.Config{Name: "test", Rules: &config.RulesConfig{Mode: config.RulesModeInline}})

	require.NoError(t, err)
	_, hasDir := copilotOutputByPath(outputs, ".github/instructions")
	assert.False(t, hasDir)
}

func TestCopilot_SplitMode(t *testing.T) {
	// Arrange
	g := &CopilotPresetGenerator{}
	content := &config.ContentTree{
		Rules: []config.ContentFile{
			copilotRuleFixture("global-rule", "Global body.", nil),
			copilotRuleFixture("scoped-rule", "Scoped body.", &config.Metadata{Globs: []string{"src/**"}}),
		},
		Context: []config.ContentFile{
			copilotRuleFixture("scoped-ctx", "Ctx body.", &config.Metadata{Globs: []string{"docs/**"}}),
			copilotRuleFixture("plain-ctx", "Plain ctx body.", nil),
		},
	}

	// Act
	outputs, err := g.Generate(content, "/test", splitCopilotConfig())

	// Assert
	require.NoError(t, err)
	root, ok := copilotOutputByPath(outputs, ".github/copilot-instructions.md")
	require.True(t, ok)
	assert.NotContains(t, root.Content, "## Rules")
	assert.NotContains(t, root.Content, "Global body.")
	assert.NotContains(t, root.Content, "Scoped body.")
	assert.Contains(t, root.Content, "## Context")
	assert.Contains(t, root.Content, "Plain ctx body.")

	for _, p := range []string{
		".github/instructions/global-rule.instructions.md",
		".github/instructions/scoped-rule.instructions.md",
		".github/instructions/context-scoped-ctx.instructions.md",
	} {
		_, ok := copilotOutputByPath(outputs, p)
		assert.True(t, ok, p)
	}
	_, ok = copilotOutputByPath(outputs, ".github/instructions/context-plain-ctx.instructions.md")
	assert.False(t, ok, "unscoped context stays inline")
}

func TestCopilot_InlineOnlyRulesDoNotCollide(t *testing.T) {
	// Arrange: manual rules stay inline, so names that would map to the same
	// file do not matter; the glob rule still gets its file.
	manual := &config.Metadata{Activation: "manual"}
	content := &config.ContentTree{Rules: []config.ContentFile{
		copilotRuleFixture("Foo", "Upper.", manual),
		copilotRuleFixture("foo", "Lower.", manual),
		copilotRuleFixture("a b", "Space.", manual),
		copilotRuleFixture("a_b", "Under.", manual),
		copilotRuleFixture("scoped", "Scoped.", &config.Metadata{Globs: []string{"src/**"}}),
	}}

	// Act
	outputs, err := (&CopilotPresetGenerator{}).Generate(content, "/test", splitCopilotConfig())

	// Assert
	require.NoError(t, err)
	root, ok := copilotOutputByPath(outputs, ".github/copilot-instructions.md")
	require.True(t, ok)
	for _, body := range []string{"Upper.", "Lower.", "Space.", "Under."} {
		assert.Contains(t, root.Content, body)
	}
	_, ok = copilotOutputByPath(outputs, ".github/instructions/scoped.instructions.md")
	assert.True(t, ok)
}

func TestCopilotPresetGenerator_renderCommandFile_PromptFile(t *testing.T) {
	g := &CopilotPresetGenerator{}
	tests := []struct {
		name    string
		command config.ContentFile
		want    string
	}{
		{"description, hint and input variable", config.ContentFile{
			Name: "ship", Content: "Ship $ARGUMENTS now.",
			Metadata: &config.Metadata{Extra: map[string]string{"description": "Ship it", "argument-hint": "<env>"}},
		}, "---\nargument-hint: <env>\ndescription: Ship it\n---\n\nShip ${input:args} now."},
		{"no metadata is the bare body", config.ContentFile{Name: "ship", Content: "Do it."}, "Do it."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, g.renderCommandFile(tt.command))
		})
	}
}
