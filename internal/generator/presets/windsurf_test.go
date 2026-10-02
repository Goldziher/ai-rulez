package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestWindsurfPresetGenerator_Generate(t *testing.T) {
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
						Name:    "Test Rule",
						Content: "Rule content here",
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 5, // 4 directories (.windsurf, .windsurf/rules, .windsurf/skills, .windsurf/agents) + 1 file
			wantErr:     false,
		},
		{
			name: "generates multiple rule files",
			content: &config.ContentTree{
				Rules: []config.ContentFile{
					{
						Name:    "Rule One",
						Content: "First rule",
					},
					{
						Name:    "Rule Two",
						Content: "Second rule",
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 6, // 4 directories + 2 files
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &WindsurfPresetGenerator{}
			cfg := &config.Config{
				Name: "Test Project",
			}

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

func TestWindsurfPresetGenerator_TriggerFrontmatter(t *testing.T) {
	tests := []struct {
		name             string
		rule             config.ContentFile
		shouldContain    string
		shouldNotContain string
	}{
		{
			name: "no trigger - always_on frontmatter",
			rule: config.ContentFile{
				Name:    "Test Rule",
				Content: "Rule content",
			},
			shouldContain:    "---\ntrigger: always_on\n---\n",
			shouldNotContain: "trigger: manual",
		},
		{
			name: "manual mode - explicit manual frontmatter",
			rule: config.ContentFile{
				Name:    "Manual Rule",
				Content: "Content",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"trigger": "manual",
					},
				},
			},
			shouldContain:    "---\ntrigger: manual\n---\n",
			shouldNotContain: "always_on",
		},
		{
			name: "always_on mode - generates frontmatter",
			rule: config.ContentFile{
				Name:    "Always Rule",
				Content: "Content",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"trigger": "always_on",
					},
				},
			},
			shouldContain:    "---\ntrigger: always_on",
			shouldNotContain: "",
		},
		{
			name: "model_decision mode with description - generates frontmatter",
			rule: config.ContentFile{
				Name:    "Smart Rule",
				Content: "Content",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"trigger":     "model_decision",
						"description": "Apply when working with APIs",
					},
				},
			},
			shouldContain:    "description: Apply when working with APIs",
			shouldNotContain: "",
		},
		{
			name: "glob mode with pattern - generates frontmatter",
			rule: config.ContentFile{
				Name:    "Glob Rule",
				Content: "Content",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"trigger": "glob",
						"glob":    "**/*.ts",
					},
				},
			},
			shouldContain:    "globs: '**/*.ts'\ntrigger: glob",
			shouldNotContain: "",
		},
		{
			name: "description with special characters is quoted",
			rule: config.ContentFile{
				Name:    "Special Rule",
				Content: "Content",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"trigger":     "model_decision",
						"description": "Apply for JSON: API #critical\nand docs",
					},
				},
			},
			shouldContain:    "description: |-\n    Apply for JSON: API #critical\n    and docs",
			shouldNotContain: "",
		},
		{
			name: "model_decision mode - generates frontmatter",
			rule: config.ContentFile{
				Name:    "Auto Rule",
				Content: "Content",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"trigger": "model_decision",
					},
				},
			},
			shouldContain:    "trigger: model_decision",
			shouldNotContain: "",
		},
		{
			name: "invalid mode - falls back to always_on",
			rule: config.ContentFile{
				Name:    "Invalid Rule",
				Content: "Content",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"trigger": "invalid_mode",
					},
				},
			},
			shouldContain:    "trigger: always_on",
			shouldNotContain: "invalid_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &WindsurfPresetGenerator{}
			cfg := &config.Config{
				Name: "Test Project",
			}

			result := windsurfRuleFile(t, g, cfg, &config.ContentTree{Rules: []config.ContentFile{tt.rule}}, tt.rule.Name)

			if tt.shouldContain != "" && !contains(result, tt.shouldContain) {
				t.Errorf("Expected output to contain %q, but it didn't", tt.shouldContain)
			}

			if tt.shouldNotContain != "" && contains(result, tt.shouldNotContain) {
				t.Errorf("Expected output NOT to contain %q, but it did", tt.shouldNotContain)
			}
		})
	}
}

func TestWindsurfPresetGenerator_GetName(t *testing.T) {
	g := &WindsurfPresetGenerator{}
	if got := g.GetName(); got != "windsurf" {
		t.Errorf("GetName() = %v, want %v", got, "windsurf")
	}
}

func TestWindsurfPresetGenerator_GetOutputPaths(t *testing.T) {
	g := &WindsurfPresetGenerator{}
	baseDir := "/test/base"
	paths := g.GetOutputPaths(baseDir)
	if len(paths) != 4 {
		t.Errorf("GetOutputPaths() returned %d paths, want 4", len(paths))
	}
	expectedPaths := []string{
		filepath.Join(baseDir, ".windsurf"),
		filepath.Join(baseDir, ".windsurf", "rules"),
		filepath.Join(baseDir, ".windsurf", "skills"),
		filepath.Join(baseDir, ".windsurf", "agents"),
	}
	for i, want := range expectedPaths {
		if i < len(paths) && paths[i] != want {
			t.Errorf("GetOutputPaths()[%d] = %v, want %v", i, paths[i], want)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestWindsurfPresetGenerator_buildWindsurfAgentFrontmatter_Effort(t *testing.T) {
	g := &WindsurfPresetGenerator{}

	t.Run("agent effort emitted", func(t *testing.T) {
		fm := g.buildWindsurfAgentFrontmatter(
			config.ContentFile{Name: "x", Metadata: &config.Metadata{Effort: "high"}},
			&config.Config{},
		)
		if fm["reasoning_effort"] != "high" {
			t.Errorf("want reasoning_effort=high, got %v", fm["reasoning_effort"])
		}
	})

	t.Run("max maps to high", func(t *testing.T) {
		fm := g.buildWindsurfAgentFrontmatter(
			config.ContentFile{Name: "x", Metadata: &config.Metadata{Effort: "max"}},
			&config.Config{},
		)
		if fm["reasoning_effort"] != "high" {
			t.Errorf("want reasoning_effort=high, got %v", fm["reasoning_effort"])
		}
	})

	t.Run("inherit dropped", func(t *testing.T) {
		fm := g.buildWindsurfAgentFrontmatter(
			config.ContentFile{Name: "x", Metadata: &config.Metadata{Effort: "inherit"}},
			&config.Config{},
		)
		if _, ok := fm["reasoning_effort"]; ok {
			t.Errorf("inherit should not emit reasoning_effort, got %v", fm["reasoning_effort"])
		}
	})

	t.Run("per-preset override applied when no agent metadata", func(t *testing.T) {
		fm := g.buildWindsurfAgentFrontmatter(
			config.ContentFile{Name: "x"},
			&config.Config{Defaults: &config.DefaultsConfig{
				Effort:         "low",
				EffortByPreset: map[string]string{"windsurf": "xhigh"},
			}},
		)
		if fm["reasoning_effort"] != "xhigh" {
			t.Errorf("want xhigh from per-preset override, got %v", fm["reasoning_effort"])
		}
	})

	t.Run("no effort when nothing set", func(t *testing.T) {
		fm := g.buildWindsurfAgentFrontmatter(
			config.ContentFile{Name: "x"},
			&config.Config{},
		)
		if _, ok := fm["reasoning_effort"]; ok {
			t.Errorf("expected no reasoning_effort, got %v", fm["reasoning_effort"])
		}
	})
}

// windsurfRuleFile generates the preset and returns the content of the rules-folder file for name.
func windsurfRuleFile(t *testing.T, g *WindsurfPresetGenerator, cfg *config.Config, content *config.ContentTree, fileName string) string {
	t.Helper()
	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := filepath.Join("/test", ".windsurf", "rules", sanitizeName(fileName)+".md")
	for _, o := range outputs {
		if o.Path == want {
			return o.Content
		}
	}
	t.Fatalf("no output for %s", want)
	return ""
}

func TestWindsurf_PathsGlobsHonored(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		paths []string
		globs []string
		want  string
	}{
		{name: "paths", paths: []string{"src/**"}, want: "globs: src/**\ntrigger: glob\n"},
		{name: "globs", globs: []string{"*.ts", "*.tsx"}, want: "globs: '*.ts,*.tsx'\ntrigger: glob\n"},
		{name: "legacy singular glob", extra: map[string]string{"trigger": "glob", "glob": "**/*.go"},
			want: "globs: '**/*.go'\ntrigger: glob\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rule := config.ContentFile{
				Name: "Scoped", Content: "body",
				Metadata: &config.Metadata{Paths: tt.paths, Globs: tt.globs, Extra: tt.extra},
			}
			// Act
			out := windsurfRuleFile(t, &WindsurfPresetGenerator{}, &config.Config{},
				&config.ContentTree{Rules: []config.ContentFile{rule}}, "Scoped")
			// Assert
			if !strings.Contains(out, tt.want) {
				t.Errorf("want %q in:\n%s", tt.want, out)
			}
			if strings.Contains(out, "\nglob:") {
				t.Errorf("singular glob: emitted:\n%s", out)
			}
		})
	}
}

func TestWindsurf_ContextFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		ctx  config.ContentFile
		want string
	}{
		{name: "unscoped is always_on", ctx: config.ContentFile{Name: "Overview", Content: "c"},
			want: "---\ntrigger: always_on\n---\n"},
		{name: "scoped is glob", ctx: config.ContentFile{Name: "Overview", Content: "c",
			Metadata: &config.Metadata{Paths: []string{"docs/**"}}}, want: "globs: docs/**\ntrigger: glob\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			content := &config.ContentTree{Context: []config.ContentFile{tt.ctx}}
			// Act
			out := windsurfRuleFile(t, &WindsurfPresetGenerator{}, &config.Config{}, content, "context-Overview")
			// Assert
			if !strings.HasPrefix(out, "---\n") || !strings.Contains(out, tt.want) {
				t.Errorf("want frontmatter %q in:\n%s", tt.want, out)
			}
			if !strings.Contains(out, "# Overview") {
				t.Errorf("missing heading in:\n%s", out)
			}
		})
	}
}

func TestWindsurf_MaxCharsWarning(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		wantNote bool
	}{
		{name: "under limit", size: 100},
		{name: "over limit", size: 13000, wantNote: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rules := []config.ContentFile{{Name: "Big", Content: strings.Repeat("x", tt.size)}}
			items, _, err := windsurfRuleItems(rules, nil)
			if err != nil || len(items) != 1 {
				t.Fatalf("items=%d err=%v", len(items), err)
			}
			// Act
			_, notes, err := rulefiles.Render(windsurfRulesTarget, items[0], &config.Config{})
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if (len(notes) > 0) != tt.wantNote {
				t.Errorf("notes = %v, wantNote %v", notes, tt.wantNote)
			}
		})
	}
}
