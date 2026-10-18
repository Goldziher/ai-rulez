package presets

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// recordingLogger collects Warn messages.
type recordingLogger struct{ warns []string }

func (*recordingLogger) Debug(string, ...any) {}
func (*recordingLogger) Info(string, ...any)  {}
func (*recordingLogger) Error(string, ...any) {}
func (r *recordingLogger) Warn(msg string, _ ...any) {
	r.warns = append(r.warns, msg)
}

func TestWarnDuplicateContent_honoursAllowOverrides(t *testing.T) {
	t.Parallel()

	configDir := filepath.Join(t.TempDir(), ".ai-rulez")
	localSkill := filepath.Join(configDir, "skills", "testing-conventions", "SKILL.md")
	domainSkill := filepath.Join(configDir, "domains", "backend", "skills", "testing-conventions", "SKILL.md")
	builtinSkill := "builtin://universal/testing/skills/testing-conventions/SKILL.md"

	tests := []struct {
		name      string
		allow     []string
		root      string
		wantWarns int
	}{
		{"no allow list warns", nil, localSkill, 1},
		{"bare name silences", []string{"testing-conventions"}, localSkill, 0},
		{"bare name is case-insensitive", []string{" Testing-Conventions "}, localSkill, 0},
		{"other name still warns", []string{"other"}, localSkill, 1},
		{"scoped to builtin silences", []string{"builtin/testing-conventions"}, localSkill, 0},
		{"scoped to unrelated domain warns", []string{"backend/testing-conventions"}, localSkill, 1},
		{"scoped to the domain copy silences", []string{"backend/testing-conventions"}, domainSkill, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			tree := &config.ContentTree{
				Skills: []config.ContentFile{{Name: "testing-conventions", Path: tc.root}},
				Domains: map[string]*config.Domain{
					"testing": {Name: "testing", Builtin: true, Skills: []config.ContentFile{{Name: "testing-conventions", Path: builtinSkill}}},
				},
			}
			cfg := &config.Config{ConfigDir: configDir}
			if tc.allow != nil {
				cfg.Lint = &config.LintConfig{AllowOverrides: tc.allow}
			}
			if tc.root == domainSkill {
				tree.Skills = nil
				tree.Domains["backend"] = &config.Domain{Name: "backend", Skills: []config.ContentFile{{Name: "testing-conventions", Path: tc.root}}}
			}
			log := &recordingLogger{}

			// Act
			WarnDuplicateContent(log, tree, cfg)

			// Assert
			assert.Len(t, log.warns, tc.wantWarns)
		})
	}
}
