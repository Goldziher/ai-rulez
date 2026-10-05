package presets

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCursorPresetGenerator_Generate(t *testing.T) {
	tests := []struct {
		name        string
		content     *config.ContentTree
		baseDir     string
		wantOutputs int
		wantErr     bool
	}{
		{
			name: "generates rule files",
			content: &config.ContentTree{
				Rules: []config.ContentFile{
					{
						Name:    "rule1",
						Content: "Rule 1 content",
						Metadata: &config.Metadata{
							Priority: "high",
						},
					},
					{
						Name:    "rule2",
						Content: "Rule 2 content",
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 8, // .cursor, .cursor/rules, .cursor/commands, .agents, .agents/skills, .cursor/agents, 2 rule files
			wantErr:     false,
		},
		{
			name: "handles domains",
			content: &config.ContentTree{
				Rules: []config.ContentFile{
					{
						Name:    "root-rule",
						Content: "Root rule",
					},
				},
				Domains: map[string]*config.Domain{
					"backend": {
						Name: "backend",
						Rules: []config.ContentFile{
							{
								Name:    "backend-rule",
								Content: "Backend rule",
							},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 8, // .cursor, .cursor/rules, .cursor/commands, .agents, .agents/skills, .cursor/agents, 2 rule files
			wantErr:     false,
		},
		{
			name: "generates command files as rules",
			content: &config.ContentTree{
				Commands: []config.ContentFile{
					{
						Name:    "test command",
						Content: "Command content",
						Metadata: &config.Metadata{
							Usage:   "test-cmd",
							Aliases: []string{"tc"},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 7, // .cursor, .cursor/rules, .cursor/commands, .agents, .agents/skills, .cursor/agents, 1 command file
			wantErr:     false,
		},
		{
			name: "filters commands by target",
			content: &config.ContentTree{
				Commands: []config.ContentFile{
					{
						Name:    "cursor-command",
						Content: "Cursor only",
						Metadata: &config.Metadata{
							Usage:   "cmd1",
							Targets: []string{"cursor"},
						},
					},
					{
						Name:    "claude-command",
						Content: "Claude only",
						Metadata: &config.Metadata{
							Usage:   "cmd2",
							Targets: []string{"claude"},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 7, // .cursor, .cursor/rules, .cursor/commands, .agents, .agents/skills, .cursor/agents, 1 command file (only cursor-command)
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &CursorPresetGenerator{}
			cfg := &config.Config{
				Name: "test",
			}

			outputs, err := g.Generate(tt.content, tt.baseDir, cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(outputs) != tt.wantOutputs {
				t.Errorf("Generate() got %d outputs, want %d", len(outputs), tt.wantOutputs)
			}

			// Verify rule files have .mdc extension; command files use .md
			for _, output := range outputs {
				if output.IsDir {
					continue
				}
				pathSlash := filepath.ToSlash(output.Path)
				if strings.Contains(pathSlash, "/rules/") && !strings.HasSuffix(output.Path, ".mdc") {
					t.Errorf("Rule file expected .mdc extension, got %s", output.Path)
				}
				if strings.Contains(pathSlash, "/commands/") && !strings.HasSuffix(output.Path, ".md") {
					t.Errorf("Command file expected .md extension, got %s", output.Path)
				}
			}
		})
	}
}

func TestCursorPresetGenerator_RuleFileContent(t *testing.T) {
	// Arrange
	g := &CursorPresetGenerator{}
	content := &config.ContentTree{Rules: []config.ContentFile{{
		Name:     "test rule",
		Content:  "Test content",
		Metadata: &config.Metadata{Priority: "high"},
	}}}

	// Act
	result := cursorRuleFile(t, g, content, "test-rule.mdc")

	// Assert
	assert.Contains(t, result, "# test rule")
	assert.Contains(t, result, "**Priority:** high")
	assert.Contains(t, result, "Test content")
}

// cursorRuleFile generates the preset and returns the content of the named file
// under .cursor/rules.
func cursorRuleFile(t *testing.T, g *CursorPresetGenerator, content *config.ContentTree, name string) string {
	t.Helper()
	outputs, err := g.Generate(content, "/test", &config.Config{})
	require.NoError(t, err)
	want := filepath.Join("/test", ".cursor", "rules", name)
	for _, o := range outputs {
		if o.Path == want {
			return o.Content
		}
	}
	t.Fatalf("output %s not generated", want)
	return ""
}

func cursorFrontmatterOf(t *testing.T, text string) map[string]any {
	t.Helper()
	require.True(t, strings.HasPrefix(text, "---\n"), "must open with frontmatter:\n%s", text)
	end := strings.Index(text[4:], "\n---\n")
	require.GreaterOrEqual(t, end, 0)
	// Cursor's globs line is a bare comma list, not valid YAML when it starts
	// with "*": quote it for the parse.
	block := regexp.MustCompile(`(?m)^globs: [^'"].*$`).ReplaceAllStringFunc(text[4:4+end+1], func(line string) string {
		return "globs: '" + strings.TrimPrefix(line, "globs: ") + "'"
	})
	var fm map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(block), &fm))
	return fm
}

func TestCursor_ContextAlwaysApply(t *testing.T) {
	tests := []struct {
		name string
		meta *config.Metadata
		want map[string]any
	}{
		{"unscoped context applies always", nil, map[string]any{"alwaysApply": true}},
		{
			"scoped context attaches by glob",
			&config.Metadata{Globs: []string{"src/**"}},
			map[string]any{"alwaysApply": false, "globs": "src/**"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			content := &config.ContentTree{Context: []config.ContentFile{{Name: "Arch Notes", Content: "body", Metadata: tt.meta}}}

			// Act
			text := cursorRuleFile(t, &CursorPresetGenerator{}, content, "context-Arch-Notes.mdc")

			// Assert
			assert.Equal(t, tt.want, cursorFrontmatterOf(t, text))
			assert.Contains(t, text, "# Arch Notes")
		})
	}
}

func TestCursor_BraceGlobsExpanded(t *testing.T) {
	// Arrange
	content := &config.ContentTree{Rules: []config.ContentFile{{
		Name: "ts", Content: "x", Metadata: &config.Metadata{Globs: []string{"*.{ts,tsx}"}},
	}}}

	// Act
	text := cursorRuleFile(t, &CursorPresetGenerator{}, content, "ts.mdc")

	// Assert
	assert.Equal(t, map[string]any{"alwaysApply": false, "globs": "*.ts,*.tsx"}, cursorFrontmatterOf(t, text))
	assert.Contains(t, text, "\nglobs: *.ts,*.tsx\n", "Cursor writes globs as an unquoted comma list")
}

func TestCursor_ManualHasFrontmatter(t *testing.T) {
	// Arrange
	content := &config.ContentTree{Rules: []config.ContentFile{{
		Name: "manual", Content: "x",
		Metadata: &config.Metadata{Activation: "manual", Extra: map[string]string{"description": "ignored"}},
	}}}

	// Act
	text := cursorRuleFile(t, &CursorPresetGenerator{}, content, "manual.mdc")

	// Assert: an explicit alwaysApply: false and no description (which would make it agent-requested)
	assert.Equal(t, map[string]any{"alwaysApply": false}, cursorFrontmatterOf(t, text))
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "spaces to dashes",
			input:    "test rule",
			expected: "test-rule",
		},
		{
			name:     "special characters removed",
			input:    "test@rule!",
			expected: "testrule",
		},
		{
			name:     "underscores to dashes",
			input:    "test_rule",
			expected: "test-rule",
		},
		{
			name:     "mixed case preserved",
			input:    "TestRule",
			expected: "TestRule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCombineContentFiles(t *testing.T) {
	slice1 := []config.ContentFile{
		{Name: "file1"},
		{Name: "file2"},
	}
	slice2 := []config.ContentFile{
		{Name: "file3"},
	}

	result := combineContentFiles(slice1, slice2)

	if len(result) != 3 {
		t.Errorf("combineContentFiles() length = %d, want 3", len(result))
	}

	names := []string{result[0].Name, result[1].Name, result[2].Name}
	expected := []string{"file1", "file2", "file3"}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("combineContentFiles()[%d].Name = %q, want %q", i, name, expected[i])
		}
	}
}

func TestCursorPresetGenerator_shouldIncludeCommand(t *testing.T) {
	tests := []struct {
		name    string
		command config.ContentFile
		want    bool
	}{
		{
			name: "includes command with no metadata",
			command: config.ContentFile{
				Name:    "cmd1",
				Content: "Content",
			},
			want: true,
		},
		{
			name: "includes command with empty targets",
			command: config.ContentFile{
				Name:    "cmd2",
				Content: "Content",
				Metadata: &config.Metadata{
					Targets: []string{},
				},
			},
			want: true,
		},
		{
			name: "includes command with cursor target",
			command: config.ContentFile{
				Name:    "cmd3",
				Content: "Content",
				Metadata: &config.Metadata{
					Targets: []string{"cursor"},
				},
			},
			want: true,
		},
		{
			name: "includes command with multiple targets including cursor",
			command: config.ContentFile{
				Name:    "cmd4",
				Content: "Content",
				Metadata: &config.Metadata{
					Targets: []string{"claude", "cursor"},
				},
			},
			want: true,
		},
		{
			name: "excludes command with non-cursor target",
			command: config.ContentFile{
				Name:    "cmd5",
				Content: "Content",
				Metadata: &config.Metadata{
					Targets: []string{"claude"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &CursorPresetGenerator{}
			result := g.shouldIncludeCommand(tt.command)
			if result != tt.want {
				t.Errorf("shouldIncludeCommand() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestCursorPresetGenerator_Generate_WithAgents(t *testing.T) {
	g := &CursorPresetGenerator{}
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
		Agents: []config.ContentFile{
			{
				Name:    "verifier",
				Content: "You verify completed work.",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"description":   "Validates completed work",
						"model":         "fast",
						"is_background": "false",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Find the agent file
	var agentContent string
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, "verifier.md") {
			agentContent = o.Content
		}
	}

	if agentContent == "" {
		t.Fatal("Expected verifier.md agent file in outputs")
	}

	if !strings.HasPrefix(agentContent, "---\n") {
		t.Error("Expected YAML frontmatter start")
	}
	if !strings.Contains(agentContent, "name: verifier") {
		t.Error("Expected name in frontmatter")
	}
	if !strings.Contains(agentContent, "description: Validates completed work") {
		t.Error("Expected description in frontmatter")
	}
	if !strings.Contains(agentContent, "is_background: false\n") {
		t.Error("Expected is_background as a YAML boolean")
	}
	if !strings.Contains(agentContent, "model: fast") {
		t.Error("Expected model in frontmatter")
	}
	if !strings.Contains(agentContent, "You verify completed work.") {
		t.Error("Expected agent content")
	}
}

func TestCursorPresetGenerator_Generate_SkillsInAgentsDir(t *testing.T) {
	g := &CursorPresetGenerator{}
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
		Skills: []config.ContentFile{
			{
				Name:    "deploy",
				Content: "Deploy skill",
				Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Verify skills are in .agents/skills/ not .cursor/skills/
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".cursor/skills/") {
			t.Error("Skills should NOT be in .cursor/skills/, should be in .agents/skills/")
		}
	}

	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".agents/skills/deploy/SKILL.md") {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Error("Expected skill file at .agents/skills/deploy/SKILL.md")
	}
}

func TestCursorPresetGenerator_renderSkillFile_QuotesDescriptionFrontmatter(t *testing.T) {
	g := &CursorPresetGenerator{}
	cfg := &config.Config{Name: "test"}

	content := g.renderSkillFile(config.ContentFile{
		Name:    "binding-audit",
		Content: "# Binding Audit",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"description": "Audit bindings for coverage gaps. Covers the full audit flow: config review, attribute scan, and triage.",
			},
		},
	}, cfg)

	parts := strings.SplitN(content, "---", 3)
	if len(parts) != 3 {
		t.Fatalf("expected YAML frontmatter, got:\n%s", content)
	}

	var frontmatter map[string]string
	if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err != nil {
		t.Fatalf("frontmatter should parse as YAML: %v\n%s", err, parts[1])
	}

	if got := frontmatter["description"]; got != "Audit bindings for coverage gaps. Covers the full audit flow: config review, attribute scan, and triage." {
		t.Fatalf("description = %q", got)
	}
}

func TestCursorPresetGenerator_renderCommandFile(t *testing.T) {
	g := &CursorPresetGenerator{}

	command := config.ContentFile{
		Name:    "test-command",
		Content: "Command implementation details",
		Metadata: &config.Metadata{
			Usage:   "test-cmd [options]",
			Aliases: []string{"tc", "test"},
			Extra: map[string]string{
				"description": "A test command",
			},
		},
	}

	result := g.renderCommandFile(command)

	// Heading format is # /command-name
	if !strings.Contains(result, "# /test-command") {
		t.Error("Expected command name as heading with slash prefix")
	}
	if !strings.Contains(result, "**Description:** A test command") {
		t.Error("Expected description in output")
	}
	if !strings.Contains(result, "**Usage:** `test-cmd [options]`") {
		t.Error("Expected usage in output")
	}
	// Aliases are rendered as `/<alias>`
	if !strings.Contains(result, "**Aliases:**") {
		t.Error("Expected aliases section in output")
	}
	if !strings.Contains(result, "`/tc`") || !strings.Contains(result, "`/test`") {
		t.Error("Expected aliases tc and test in output")
	}
	if !strings.Contains(result, "Command implementation details") {
		t.Error("Expected command content in output")
	}
}

func TestCursorPresetGenerator_buildCursorAgentFrontmatter_DoesNotEmitEffort(t *testing.T) {
	g := &CursorPresetGenerator{}

	agent := config.ContentFile{
		Name: "reviewer",
		Metadata: &config.Metadata{
			Effort: "high",
			Extra: map[string]string{
				"description": "Reviews code",
			},
		},
	}

	fm := g.buildCursorAgentFrontmatter(agent, nil)

	if _, ok := fm["effort"]; ok {
		t.Errorf("effort field leaked into Cursor agent frontmatter; cursor does not natively support reasoning effort yet")
	}
	if fm["description"] != "Reviews code" {
		t.Errorf("expected description to be passed through; got %v", fm["description"])
	}
}

func TestCursorPresetGenerator_renderMCPJSON_Transports(t *testing.T) {
	g := &CursorPresetGenerator{}

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

	rendered, err := g.renderMCPJSON("", cfg)
	if err != nil {
		t.Fatalf("renderMCPJSON: %v", err)
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
		if entry["url"] != wantURL {
			t.Errorf("%s url = %v, want %v", name, entry["url"], wantURL)
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
		// The root .mcp.json is shared with the mcp preset, which keys remote
		// transport on `type`; the two must render the same bytes.
		wantType := strings.TrimSuffix(name, "-server")
		if entry["type"] != wantType {
			t.Errorf("%s type = %v, want %s", name, entry["type"], wantType)
		}
	}
}

func TestCursorPresetGenerator_RuleFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		rule config.ContentFile
		want map[string]any
	}{
		{
			"unscoped rule is always applied",
			config.ContentFile{Name: "plain", Content: "Use named exports."},
			map[string]any{"alwaysApply": true},
		},
		{
			"globbed rule carries globs, alwaysApply false and its description",
			config.ContentFile{Name: "plain", Content: "x", Metadata: &config.Metadata{
				Globs: []string{"**/*.tsx"}, Extra: map[string]string{"description": "tsx rules"},
			}},
			map[string]any{"alwaysApply": false, "globs": "**/*.tsx", "description": "tsx rules"},
		},
		{
			"always rule keeps its description",
			config.ContentFile{Name: "plain", Content: "x", Metadata: &config.Metadata{
				Extra: map[string]string{"description": "style"},
			}},
			map[string]any{"alwaysApply": true, "description": "style"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			content := &config.ContentTree{Rules: []config.ContentFile{tt.rule}}

			// Act
			text := cursorRuleFile(t, &CursorPresetGenerator{}, content, "plain.mdc")

			// Assert
			assert.Equal(t, tt.want, cursorFrontmatterOf(t, text))
		})
	}
}

func TestCursorAgentFrontmatter_BooleanFlags(t *testing.T) {
	t.Parallel()

	g := &CursorPresetGenerator{}
	tests := []struct {
		name      string
		extra     map[string]string
		wantRead  interface{}
		wantBackg interface{}
	}{
		{"booleans are typed", map[string]string{"readonly": "true", "is_background": "false"}, true, false},
		{"garbage is omitted", map[string]string{"readonly": "maybe", "is_background": "yes please"}, nil, nil},
		{"unset stays absent", map[string]string{"description": "d"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			agent := config.ContentFile{Name: "a", Metadata: &config.Metadata{Extra: tt.extra}}
			fm := g.buildCursorAgentFrontmatter(agent, &config.Config{})
			if fm["readonly"] != tt.wantRead || fm["is_background"] != tt.wantBackg {
				t.Errorf("readonly/is_background = %#v/%#v, want %#v/%#v",
					fm["readonly"], fm["is_background"], tt.wantRead, tt.wantBackg)
			}
		})
	}
}
