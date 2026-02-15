package presets

import (
	"path/filepath"
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
			wantOutputs: 6, // .cursor, .cursor/rules, .cursor/skills, .cursor/commands, 2 rule files
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
			wantOutputs: 6, // .cursor, .cursor/rules, .cursor/skills, .cursor/commands, 2 rule files
			wantErr:     false,
		},
		{
			name: "generates command files as rules",
			content: &config.ContentTreeV3{
				Commands: []config.ContentFile{
					{
						Name:    "test command",
						Content: "Command content",
						Metadata: &config.MetadataV3{
							Usage:   "test-cmd",
							Aliases: []string{"tc"},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 5, // .cursor, .cursor/rules, .cursor/skills, .cursor/commands, 1 command file
			wantErr:     false,
		},
		{
			name: "filters commands by target",
			content: &config.ContentTreeV3{
				Commands: []config.ContentFile{
					{
						Name:    "cursor-command",
						Content: "Cursor only",
						Metadata: &config.MetadataV3{
							Usage:   "cmd1",
							Targets: []string{"cursor"},
						},
					},
					{
						Name:    "claude-command",
						Content: "Claude only",
						Metadata: &config.MetadataV3{
							Usage:   "cmd2",
							Targets: []string{"claude"},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 5, // .cursor, .cursor/rules, .cursor/skills, .cursor/commands, 1 command file (only cursor-command)
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
				Metadata: &config.MetadataV3{
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
				Metadata: &config.MetadataV3{
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
				Metadata: &config.MetadataV3{
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
				Metadata: &config.MetadataV3{
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

func TestCursorPresetGenerator_renderCommandFile(t *testing.T) {
	g := &CursorPresetGenerator{}

	command := config.ContentFile{
		Name:    "test-command",
		Content: "Command implementation details",
		Metadata: &config.MetadataV3{
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
