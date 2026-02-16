package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestWindsurfPresetGenerator_Generate(t *testing.T) {
	tests := []struct {
		name        string
		content     *config.ContentTreeV3
		baseDir     string
		wantOutputs int
		wantErr     bool
	}{
		{
			name: "generates rule files",
			content: &config.ContentTreeV3{
				Rules: []config.ContentFile{
					{
						Name:    "Test Rule",
						Content: "Rule content here",
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 2, // 1 directory + 1 file
			wantErr:     false,
		},
		{
			name: "generates multiple rule files",
			content: &config.ContentTreeV3{
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
			wantOutputs: 3, // 1 directory + 2 files
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &WindsurfPresetGenerator{}
			cfg := &config.ConfigV3{
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
			name: "no trigger - no frontmatter",
			rule: config.ContentFile{
				Name:    "Test Rule",
				Content: "Rule content",
			},
			shouldContain:    "# Test Rule",
			shouldNotContain: "---\ntrigger:",
		},
		{
			name: "manual mode - no frontmatter (default)",
			rule: config.ContentFile{
				Name:    "Manual Rule",
				Content: "Content",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"trigger": "manual",
					},
				},
			},
			shouldContain:    "# Manual Rule",
			shouldNotContain: "---\ntrigger: manual",
		},
		{
			name: "always_on mode - generates frontmatter",
			rule: config.ContentFile{
				Name:    "Always Rule",
				Content: "Content",
				Metadata: &config.MetadataV3{
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
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"trigger":     "model_decision",
						"description": "Apply when working with APIs",
					},
				},
			},
			shouldContain:    "description: \"Apply when working with APIs\"",
			shouldNotContain: "",
		},
		{
			name: "glob mode with pattern - generates frontmatter",
			rule: config.ContentFile{
				Name:    "Glob Rule",
				Content: "Content",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"trigger": "glob",
						"glob":    "**/*.ts",
					},
				},
			},
			shouldContain:    "glob: \"**/*.ts\"",
			shouldNotContain: "",
		},
		{
			name: "description with special characters is quoted",
			rule: config.ContentFile{
				Name:    "Special Rule",
				Content: "Content",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"trigger":     "model_decision",
						"description": "Apply for JSON: API #critical\nand docs",
					},
				},
			},
			shouldContain:    "description: \"Apply for JSON: API #critical\\nand docs\"",
			shouldNotContain: "",
		},
		{
			name: "model_decision mode - generates frontmatter",
			rule: config.ContentFile{
				Name:    "Auto Rule",
				Content: "Content",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"trigger": "model_decision",
					},
				},
			},
			shouldContain:    "---\ntrigger: model_decision",
			shouldNotContain: "",
		},
		{
			name: "invalid mode - falls back to manual, no frontmatter",
			rule: config.ContentFile{
				Name:    "Invalid Rule",
				Content: "Content",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"trigger": "invalid_mode",
					},
				},
			},
			shouldContain:    "# Invalid Rule",
			shouldNotContain: "---\ntrigger: invalid_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &WindsurfPresetGenerator{}
			cfg := &config.ConfigV3{
				Name: "Test Project",
			}

			result := g.renderRuleFile(tt.rule, cfg, "/test/.windsurf/rules/test.md", 1)

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
	paths := g.GetOutputPaths("/test/base")
	if len(paths) != 1 {
		t.Errorf("GetOutputPaths() returned %d paths, want 1", len(paths))
	}
	if paths[0] != "/test/base/.windsurf/rules" {
		t.Errorf("GetOutputPaths()[0] = %v, want %v", paths[0], "/test/base/.windsurf/rules")
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
