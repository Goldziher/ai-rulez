package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// targetsFixture holds one untargeted rule and one rule per target form.
func targetsFixture() *config.ContentTree {
	mk := func(name string, targets ...string) config.ContentFile {
		return config.ContentFile{
			Name: name, Content: strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_BODY",
			Path:     "/test/.ai-rulez/rules/" + name + ".md",
			Metadata: &config.Metadata{Targets: targets},
		}
	}
	return &config.ContentTree{Rules: []config.ContentFile{
		mk("free"),
		mk("to-claude-md", "CLAUDE.md"),
		mk("to-cursor", "cursor"),
		mk("to-cursor-dir", ".cursor/rules/"),
		mk("to-glob", "*.mdc"),
		mk("windows", `.\.cursor\rules\`),
		mk("skill-only", ".claude/skills/*/SKILL.md"),
	}, Context: []config.ContentFile{mk("ctx-free"), mk("ctx-cursor", "cursor")}}
}

func targetedRuleFiles(outputs []config.OutputFile, dir string) []string {
	var names []string
	for _, o := range outputs {
		if o.IsDir {
			continue
		}
		if d := filepath.ToSlash(filepath.Dir(o.Path)); strings.HasSuffix(d, "/"+dir) {
			names = append(names, filepath.Base(o.Path))
		}
	}
	return names
}

func TestTargets_RuleFilesPerPreset(t *testing.T) {
	split := &config.Config{Name: "test", Rules: &config.RulesConfig{Mode: config.RulesModeSplit}}
	tests := []struct {
		name string
		gen  interface {
			Generate(*config.ContentTree, string, *config.Config) ([]config.OutputFile, error)
		}
		cfg       *config.Config
		dir       string
		wantFiles []string
		root      string   // inline root file, "" when the preset has none
		wantRoot  []string // bodies expected in the root file
		notRoot   []string
	}{
		{
			name: "cursor", gen: &CursorPresetGenerator{}, cfg: &config.Config{Name: "test"}, dir: ".cursor/rules",
			wantFiles: []string{"free.mdc", "to-cursor.mdc", "to-cursor-dir.mdc", "to-glob.mdc", "windows.mdc", "context-ctx-free.mdc", "context-ctx-cursor.mdc"},
		},
		{
			name: "windsurf", gen: &WindsurfPresetGenerator{}, cfg: &config.Config{Name: "test"}, dir: ".windsurf/rules",
			wantFiles: []string{"free.md", "context-ctx-free.md"},
		},
		{
			name: "cline", gen: &ClinePresetGenerator{}, cfg: &config.Config{Name: "test"}, dir: ".clinerules",
			wantFiles: []string{"free.md", "context-ctx-free.md"},
		},
		{
			name: "continue-dev", gen: &ContinueDevPresetGenerator{}, cfg: &config.Config{Name: "test"}, dir: ".continue/rules",
			wantFiles: []string{"free.md"},
		},
		{
			name: "antigravity inline", gen: &AntigravityPresetGenerator{}, cfg: &config.Config{Name: "test"},
			dir: ".agents/rules", root: "GEMINI.md",
			wantRoot: []string{"FREE_BODY"},
			notRoot:  []string{"TO_CLAUDE_MD_BODY", "TO_CURSOR_BODY", "SKILL_ONLY_BODY"},
		},
		{
			name: "copilot split", gen: &CopilotPresetGenerator{}, cfg: split, dir: ".github/instructions",
			wantFiles: []string{"free.instructions.md"},
			root:      ".github/copilot-instructions.md",
			notRoot:   []string{"TO_CLAUDE_MD_BODY", "TO_CURSOR_BODY", "TO_CURSOR_DIR_BODY", "TO_GLOB_BODY"},
		},
		{
			name: "copilot inline", gen: &CopilotPresetGenerator{}, cfg: &config.Config{Name: "test"}, dir: ".github/instructions",
			root:     ".github/copilot-instructions.md",
			wantRoot: []string{"FREE_BODY"},
			notRoot:  []string{"TO_CLAUDE_MD_BODY", "TO_CURSOR_BODY", "TO_CURSOR_DIR_BODY", "TO_GLOB_BODY"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			outputs, err := tt.gen.Generate(targetsFixture(), "/test", tt.cfg)

			// Assert
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantFiles, targetedRuleFiles(outputs, tt.dir))
			if tt.root == "" {
				return
			}
			root, ok := copilotOutputByPath(outputs, tt.root)
			require.True(t, ok)
			for _, body := range tt.wantRoot {
				assert.Contains(t, root.Content, body)
			}
			for _, body := range tt.notRoot {
				assert.NotContains(t, root.Content, body)
			}
		})
	}
}

// The section matcher (skills, agents, targeted sections) and the rule-file
// matcher must agree on every path-style target.
func TestTargetMatchers_Agree(t *testing.T) {
	targets := []string{
		"CLAUDE.md", "claude.md", "./CLAUDE.md", "/CLAUDE.md", `.cursor\rules\x.mdc`, ".cursor/rules/x.mdc",
		".cursor/rules/", ".cursor/rules", `.cursor\rules\`, ".cursor/rules/*", ".cursor/**", ".cursor/*", ".windsurf/*",
		"*.mdc", "x.mdc", "*.md", "*", "**", "[x", ".claude/skills/*/SKILL.md", "GEMINI.md",
	}
	outputs := []string{".cursor/rules/x.mdc", ".cursor/rules/sub/x.mdc", "CLAUDE.md", ".claude/skills/a/SKILL.md"}
	for _, target := range targets {
		for _, out := range outputs {
			t.Run(target+" vs "+out, func(t *testing.T) {
				tg := rulefiles.Target{Preset: "none"}
				if out == "CLAUDE.md" {
					tg.RootFile = out
				}
				rel := out
				if out == "CLAUDE.md" {
					rel = ""
				}

				got := rulefiles.TargetsAllow([]string{target}, tg, rel)
				want := targetMatchesOutput(target, buildOutputPathCandidates(out, ""))

				assert.Equal(t, want, got)
			})
		}
	}
}
