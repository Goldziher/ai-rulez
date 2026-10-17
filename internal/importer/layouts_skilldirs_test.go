package importer

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

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
