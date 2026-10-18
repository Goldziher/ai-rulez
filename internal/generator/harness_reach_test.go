package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

const reachConfig = `version = "5.0"
name = "reach"
presets = ["claude", "cursor"]
`

func writeReachItem(t *testing.T, dir, rel, body string) {
	t.Helper()
	abs := filepath.Join(dir, ".ai-rulez", filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
}

// reachHas reports whether the targets hold the preset reading dir, as a writer or not.
func reachHas(targets []lint.ReachTarget, preset, dir string, writer bool) bool {
	for _, t := range targets {
		if t.Preset == preset && t.Dir == dir && t.Writer == writer {
			return true
		}
	}
	return false
}

func TestHarnessReach(t *testing.T) {
	dir := planProject(t, reachConfig)
	writeReachItem(t, dir, "skills/help/SKILL.md", "---\nname: help\ndescription: Use when testing the reach of an output.\n---\nbody\n")
	writeReachItem(t, dir, "skills/plug/SKILL.md", "---\nname: plug\ndescription: Use when testing a plugin-only output.\nplacement: plugin\n---\nbody\n")
	writeReachItem(t, dir, "agents/Explore.md", "---\nname: Explore\ndescription: Use when testing an agent output.\n---\nbody\n")
	cfg := loadPlanConfig(t, dir, nil)
	reach := NewHarnessReach(cfg, NewPlannedFiles(cfg))

	tests := []struct {
		name    string
		kind    string
		id      string
		writers map[string]string // preset -> directory it writes to
		readers []string          // presets that only read the directory
		absent  map[string]string // preset -> directory it must not be reached through
		none    bool
	}{
		{
			name: "a skill reaches the presets that write it and every reader of the shared directory", kind: "skill", id: "help",
			writers: map[string]string{"claude": ".claude/skills", "cursor": ".agents/skills"},
			readers: []string{"gemini", "codex", "opencode"},
		},
		{
			name: "an agent output file with a folded name is found", kind: "agent", id: "Explore",
			writers: map[string]string{"claude": ".claude/agents", "cursor": ".cursor/agents"},
		},
		{
			name: "a plugin-only skill is not written to .claude/skills, so Claude Code does not load it from there", kind: "skill", id: "plug",
			writers: map[string]string{"cursor": ".agents/skills"}, absent: map[string]string{"claude": ".claude/skills"},
		},
		{name: "an item the plan does not write reaches nobody", kind: "skill", id: "missing", none: true},
		{name: "an unknown kind reaches nobody", kind: "rule", id: "help", none: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := reach.Reach(tt.kind, tt.id)
			// Assert
			if tt.none {
				assert.Empty(t, got)
				return
			}
			for preset, wantDir := range tt.writers {
				assert.True(t, reachHas(got, preset, wantDir, true), "%s writes %s: %+v", preset, wantDir, got)
			}
			for preset, dir := range tt.absent {
				assert.False(t, reachHas(got, preset, dir, true), "%s must not write %s: %+v", preset, dir, got)
			}
			for _, preset := range tt.readers {
				assert.True(t, reachHas(got, preset, ".agents/skills", false), "%s reads the shared directory: %+v", preset, got)
			}
		})
	}
}

func TestHarnessReachWithoutAPlanCountsEveryEnabledPresetAsAWriter(t *testing.T) {
	// Arrange: a plan that writes nothing (no plan could be rendered)
	dir := planProject(t, reachConfig)
	cfg := loadPlanConfig(t, dir, nil)
	reach := NewHarnessReach(cfg, emptyPlan{})
	// Act
	got := reach.Reach("skill", "anything")
	// Assert
	assert.True(t, reachHas(got, "claude", ".claude/skills", true), "%+v", got)
	assert.True(t, reachHas(got, "cursor", ".agents/skills", true), "%+v", got)
}

type emptyPlan struct{}

func (emptyPlan) Planned(string) ([]byte, bool) { return nil, false }
func (emptyPlan) PlannedPaths() []string        { return nil }
