package presets

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ruleFolderPreset struct {
	name string
	gen  interface {
		Generate(*config.ContentTree, string, *config.Config) ([]config.OutputFile, error)
	}
	dir          string
	ext          string
	contextFiles bool // unscoped context items become rule files too
}

func ruleFolderPresets() []ruleFolderPreset {
	return []ruleFolderPreset{
		{"cline", &ClinePresetGenerator{}, ".clinerules/", ".md", true},
		{"devin", &DevinPresetGenerator{}, ".devin/rules/", ".md", true},
		{"cursor", &CursorPresetGenerator{}, ".cursor/rules/", ".mdc", true},
		{"copilot", &CopilotPresetGenerator{}, ".github/instructions/", ".instructions.md", false},
		{"antigravity", &AntigravityPresetGenerator{}, ".agents/rules/", ".md", false},
	}
}

func splitRulesConfig() *config.Config {
	return &config.Config{Name: "p", Rules: &config.RulesConfig{Mode: config.RulesModeSplit}}
}

func ruleFolderFiles(outputs []config.OutputFile, dir string) []string {
	var names []string
	for _, o := range outputs {
		p := filepath.ToSlash(o.Path)
		if idx := strings.Index(p, "/"+dir); idx >= 0 && !o.IsDir {
			names = append(names, p[idx+1+len(dir):])
		}
	}
	sort.Strings(names)
	return names
}

func TestRulesFolders_FileNames(t *testing.T) {
	// Spaces, underscores, case, dots and a name without ASCII letters or
	// digits (stable hash fallback).
	rules := []config.ContentFile{
		{Name: "My Rule", Content: "a"},
		{Name: "snake_case_rule", Content: "b"},
		{Name: "UPPER", Content: "c"},
		{Name: "v1.2 notes", Content: "d"},
		{Name: "日本語", Content: "e"},
	}
	for _, p := range ruleFolderPresets() {
		t.Run(p.name, func(t *testing.T) {
			// Arrange
			want := []string{
				"My-Rule" + p.ext, "UPPER" + p.ext, "rule-c12140a0" + p.ext,
				"snake-case-rule" + p.ext, "v12-notes" + p.ext,
			}

			// Act
			outputs, err := p.gen.Generate(&config.ContentTree{Rules: rules}, "/test", splitRulesConfig())

			// Assert
			require.NoError(t, err)
			assert.Equal(t, want, ruleFolderFiles(outputs, p.dir))
		})
	}
}

func TestRulesFolders_NameCollisionsAreDisambiguated(t *testing.T) {
	tests := []struct {
		name        string
		rules       []config.ContentFile
		context     []config.ContentFile
		needContext bool // only fails in presets that write context files
	}{
		{"case-only rule names", []config.ContentFile{{Name: "Foo", Content: "a"}, {Name: "foo", Content: "b"}}, nil, false},
		{"same after sanitizing", []config.ContentFile{{Name: "a b", Content: "a"}, {Name: "a_b", Content: "b"}}, nil, false},
		{"rule context-x vs context x", []config.ContentFile{{Name: "context-x", Content: "a"}},
			[]config.ContentFile{{Name: "x", Content: "b"}}, true},
		{"case-only context names", nil,
			[]config.ContentFile{{Name: "Ctx", Content: "a"}, {Name: "ctx", Content: "b"}}, true},
	}
	for _, p := range ruleFolderPresets() {
		for _, tt := range tests {
			t.Run(p.name+"/"+tt.name, func(t *testing.T) {
				// Arrange
				content := &config.ContentTree{Rules: tt.rules, Context: tt.context}

				// Act
				outputs, err := p.gen.Generate(content, "/test", splitRulesConfig())

				// Assert
				require.NoError(t, err)
				names := ruleFolderFiles(outputs, p.dir)
				if tt.needContext && !p.contextFiles {
					return
				}
				assert.Len(t, names, len(tt.rules)+len(tt.context), "every item keeps its own file: %v", names)
			})
		}
	}
}
