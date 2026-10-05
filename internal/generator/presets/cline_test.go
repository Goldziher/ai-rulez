package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestClinePresetGenerator_GetName(t *testing.T) {
	g := &ClinePresetGenerator{}
	if got := g.GetName(); got != "cline" {
		t.Errorf("GetName() = %q, want %q", got, "cline")
	}
}

func TestClinePresetGenerator_Generate_WithSkills(t *testing.T) {
	g := &ClinePresetGenerator{}
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "Rule content"},
		},
		Skills: []config.ContentFile{
			{
				Name:    "deploy",
				Content: "Deploy instructions",
				Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .clinerules, .cline, .cline/skills = 3 dirs
	// Rule: 1 file
	// Skill: dir + SKILL.md = 2
	// Agents dir = 1
	if len(outputs) != 7 {
		t.Errorf("Generate() got %d outputs, want 7", len(outputs))
	}

	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".cline/skills/deploy/SKILL.md") {
			foundSkill = true
			if !strings.Contains(o.Content, "name: deploy") {
				t.Error("SKILL.md should contain skill name")
			}
		}
	}
	if !foundSkill {
		t.Error("Expected .cline/skills/deploy/SKILL.md in outputs")
	}
}

func TestClinePresetGenerator_renderClineAgentFile(t *testing.T) {
	g := &ClinePresetGenerator{}

	agent := config.ContentFile{
		Name:    "test-agent",
		Content: "You are a test agent.",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"description": "A test agent",
				"model":       "anthropic/claude-sonnet-4",
			},
		},
	}

	content, err := g.renderClineAgentFile(agent, nil)
	require.NoError(t, err)
	assert.Contains(t, content, "---")
	assert.Contains(t, content, "name: test-agent")
	assert.Contains(t, content, "description: A test agent")
	assert.Contains(t, content, "modelId: anthropic/claude-sonnet-4")
	assert.Contains(t, content, "You are a test agent.")
}

func TestClinePresetGenerator_Generate_WithContext(t *testing.T) {
	g := &ClinePresetGenerator{}

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "Rule content"},
		},
		Context: []config.ContentFile{
			{Name: "project-arch", Content: "Uses hexagonal architecture"},
		},
		Skills:   []config.ContentFile{},
		Agents:   []config.ContentFile{},
		Commands: []config.ContentFile{},
		Domains:  map[string]*config.Domain{},
	}
	cfg := &config.Config{Name: "test"}

	outputs, err := g.Generate(content, "/tmp/test", cfg)
	require.NoError(t, err)

	// Check that context is rendered as a file in .clinerules
	found := false
	for _, o := range outputs {
		if strings.Contains(o.Content, "hexagonal architecture") {
			found = true
			break
		}
	}
	assert.True(t, found, "Context should be rendered in output")
}

func clineOutputs(t *testing.T, content *config.ContentTree) map[string]string {
	t.Helper()
	g := &ClinePresetGenerator{}
	outputs, err := g.Generate(content, "/tmp/test", &config.Config{Name: "test"})
	require.NoError(t, err)

	byPath := map[string]string{}
	for _, o := range outputs {
		if !o.IsDir {
			byPath[o.Path] = o.Content
		}
	}
	return byPath
}

func TestCline_PathsFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		metadata    *config.Metadata
		wantPrefix  string
		wantNoFront bool
	}{
		{
			name:       "paths scoped rule gets paths frontmatter first",
			metadata:   &config.Metadata{Paths: []string{"src/**/*.ts", "lib/**"}},
			wantPrefix: "---\npaths:\n    - src/**/*.ts\n    - lib/**\n---\n",
		},
		{
			name:       "globs spelling maps to paths",
			metadata:   &config.Metadata{Globs: []string{"*.go"}},
			wantPrefix: "---\npaths:\n    - '*.go'\n---\n",
		},
		{
			name:        "always-on rule has no frontmatter",
			metadata:    nil,
			wantNoFront: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			content := &config.ContentTree{
				Rules:   []config.ContentFile{{Name: "ts rules", Content: "Body", Metadata: tt.metadata}},
				Context: []config.ContentFile{{Name: "arch", Content: "Context body", Metadata: tt.metadata}},
				Domains: map[string]*config.Domain{},
			}

			// Act
			files := clineOutputs(t, content)

			// Assert: rules and context share the same frontmatter handling, banner after it
			for _, p := range []string{
				filepath.Join("/tmp/test", ".clinerules", "ts-rules.md"),
				filepath.Join("/tmp/test", ".clinerules", "context-arch.md"),
			} {
				got, ok := files[p]
				require.True(t, ok, "missing %s", p)
				if tt.wantNoFront {
					assert.False(t, strings.HasPrefix(got, "---\n"), "unexpected frontmatter in %s", p)
					continue
				}
				require.True(t, strings.HasPrefix(got, tt.wantPrefix), "%s: %q", p, got)
				assert.Contains(t, got[len(tt.wantPrefix):], "# ")
			}
		})
	}
}

func TestClinePresetGenerator_AgentFiles(t *testing.T) {
	tests := []struct {
		name        string
		agent       config.ContentFile
		wantHas     []string
		wantMissing []string
	}{
		{"bare alias is dropped", config.ContentFile{
			Name: "a", Content: "Body", Metadata: &config.Metadata{Extra: map[string]string{"description": "d", "model": "sonnet"}},
		}, []string{"description: d"}, []string{"modelId", "model:"}},
		{"description falls back", config.ContentFile{Name: "a", Content: "Body"},
			[]string{"description: a subagent"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			outputs, err := (&ClinePresetGenerator{}).Generate(&config.ContentTree{Agents: []config.ContentFile{tt.agent}}, "/p", &config.Config{})

			// Assert
			require.NoError(t, err)
			var got string
			for _, o := range outputs {
				if filepath.ToSlash(o.Path) == "/p/.cline/agents/a.yaml" {
					got = o.Content
				}
				assert.NotEqual(t, "/p/.cline/agents/a.md", filepath.ToSlash(o.Path), "Cline reads agents as .yaml")
			}
			require.NotEmpty(t, got)
			for _, want := range tt.wantHas {
				assert.Contains(t, got, want)
			}
			for _, bad := range tt.wantMissing {
				assert.NotContains(t, got, bad)
			}
		})
	}
}

func TestClinePresetGenerator_AgentToolsUseClineVocabulary(t *testing.T) {
	tests := []struct {
		name  string
		tools []string
		want  []any
	}{
		{"claude names map to cline tools", []string{"Read", "Edit", "Bash", "Grep", "Glob", "WebFetch"},
			[]any{"read_file", "replace_in_file", "execute_command", "search_files", "list_files", "web_fetch"}},
		{"native names pass through and duplicates collapse", []string{"read_file", "Read", "Write"},
			[]any{"read_file", "write_to_file"}},
		{"unknown tools are dropped because cline refuses the whole agent", []string{"mcp__x__y", "Read"}, []any{"read_file"}},
		{"nothing mappable emits no tools key", []string{"mcp__x__y"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			agent := config.ContentFile{Name: "a", Content: "Body.", Metadata: &config.Metadata{Tools: tt.tools}}

			// Act
			fm := (&ClinePresetGenerator{}).buildClineAgentFrontmatter(agent, &config.Config{})

			// Assert
			if tt.want == nil {
				assert.NotContains(t, fm, "tools")
				return
			}
			got, _ := fm["tools"].([]string)
			var gotAny []any
			for _, g := range got {
				gotAny = append(gotAny, g)
			}
			assert.Equal(t, tt.want, gotAny)
		})
	}
}

func TestClinePresetGenerator_SkillNameIsYAMLSafe(t *testing.T) {
	skill := config.ContentFile{Name: "ship: it #1", Content: "Body.", Metadata: &config.Metadata{Extra: map[string]string{"description": "d"}}}

	out := (&ClinePresetGenerator{}).renderSkillFile(skill)

	_, after, _ := strings.Cut(out, "---\n")
	front, _, _ := strings.Cut(after, "---\n")
	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(front), &parsed))
	assert.Equal(t, "ship: it #1", parsed["name"])
}
