package importer

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

func TestSkillDirectories(t *testing.T) {
	tests := []struct {
		name    string
		presets []string
		want    []string
		absent  []string
	}{
		{name: "claude", presets: []string{"claude"}, want: []string{".claude/skills"}, absent: []string{".agents/skills"}},
		{name: "codex shares the agents directory", presets: []string{"codex"}, want: []string{".agents/skills"}, absent: []string{".claude/skills"}},
		{name: "several presets", presets: []string{"claude", "codex"}, want: []string{".agents/skills", ".claude/skills"}},
		{name: "unknown preset", presets: []string{"nope"}},
		{name: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SkillDirectories(tt.presets)

			for _, dir := range tt.want {
				assert.Contains(t, got, dir)
			}
			for _, dir := range tt.absent {
				assert.NotContains(t, got, dir)
			}
			if tt.want == nil {
				assert.Empty(t, got)
			}
			assert.True(t, slices.IsSorted(got), "the list is sorted")
		})
	}
}

// The directories an installed skill source is probed in must include the skill
// directories the presets themselves write, so the probe list cannot drift from
// the layouts. (.codex, .cursor and .gemini are read by their harnesses but not
// written by a preset, which uses .agents/skills.)
func TestConventionalSkillDirsCoverPresetLayouts(t *testing.T) {
	// Arrange
	probed := includes.ConventionalSkillDirs()
	known := map[string]bool{}
	for _, src := range nativeSources() {
		if src.Kind == KindSkill {
			known[src.Path] = true
		}
	}

	for _, dir := range []string{".claude/skills", ".agents/skills", ".github/skills", ".opencode/skills"} {
		t.Run(dir, func(t *testing.T) {
			// Assert
			assert.True(t, known[dir], "%s is no longer a preset skills directory", dir)
			assert.True(t, slices.Contains(probed, dir), "%s is a preset skills directory but is not probed", dir)
		})
	}
}

func TestKindDirectories(t *testing.T) {
	tests := []struct {
		name    string
		presets []string
		kind    Kind
		want    []string
		absent  []string
	}{
		{name: "claude rules", presets: []string{"claude"}, kind: KindRule, want: []string{".claude/rules"}, absent: []string{".cursor/rules"}},
		{name: "claude agents", presets: []string{"claude"}, kind: KindAgent, want: []string{".claude/agents"}},
		{name: "claude commands", presets: []string{"claude"}, kind: KindCommand, want: []string{".claude/commands"}},
		{name: "cursor rules", presets: []string{"cursor"}, kind: KindRule, want: []string{".cursor/rules"}, absent: []string{".claude/rules"}},
		{name: "skills match SkillDirectories", presets: []string{"claude", "codex"}, kind: KindSkill, want: SkillDirectories([]string{"claude", "codex"})},
		{name: "unknown preset", presets: []string{"nope"}, kind: KindRule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KindDirectories(tt.presets, tt.kind)

			for _, dir := range tt.want {
				assert.Contains(t, got, dir)
			}
			for _, dir := range tt.absent {
				assert.NotContains(t, got, dir)
			}
			if tt.want == nil {
				assert.Empty(t, got)
			}
			assert.True(t, slices.IsSorted(got), "the list is sorted")
		})
	}
}
