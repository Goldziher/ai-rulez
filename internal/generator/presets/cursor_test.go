package presets

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestCursorPresetGenerator_Generate(t *testing.T) {
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
						Name:    "rule1",
						Content: "Rule 1 content",
						Metadata: &config.MetadataV3{
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
			wantOutputs: 4, // .cursor dir, .cursor/rules dir, 2 rule files
			wantErr:     false,
		},
		{
			name: "handles domains",
			content: &config.ContentTreeV3{
				Rules: []config.ContentFile{
					{
						Name:    "root-rule",
						Content: "Root rule",
					},
				},
				Domains: map[string]*config.DomainV3{
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
			wantOutputs: 4, // .cursor dir, .cursor/rules dir, 2 rule files
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &CursorPresetGenerator{}
			cfg := &config.ConfigV3{
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

			// Verify rule files have .mdc extension
			for _, output := range outputs {
				if !output.IsDir && !strings.HasSuffix(output.Path, ".mdc") {
					t.Errorf("Expected .mdc extension, got %s", output.Path)
				}
			}
		})
	}
}

func TestCursorPresetGenerator_renderRuleFile(t *testing.T) {
	g := &CursorPresetGenerator{}

	rule := config.ContentFile{
		Name:    "test rule",
		Content: "Test content",
		Metadata: &config.MetadataV3{
			Priority: "high",
		},
	}

	result := g.renderRuleFile(rule)

	if !strings.Contains(result, "# test rule") {
		t.Error("Expected rule name as heading")
	}
	if !strings.Contains(result, "**Priority:** high") {
		t.Error("Expected priority in output")
	}
	if !strings.Contains(result, "Test content") {
		t.Error("Expected rule content in output")
	}
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
