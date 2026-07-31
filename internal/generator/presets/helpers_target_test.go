package presets

import "testing"

// TestTargetMatchesOutput covers the target→output matching used by the
// targeted_rules / targeted_context sections. Regression coverage for #156:
// a rule targeting the "claude" preset name must route only to CLAUDE.md, not
// to every nested .claude/ per-item output (skills, agents, commands).
func TestTargetMatchesOutput(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		candidates []string
		want       bool
	}{
		{
			name:       "claude preset target matches CLAUDE.md",
			target:     "claude",
			candidates: []string{"CLAUDE.md"},
			want:       true,
		},
		{
			name:       "claude preset target does NOT match nested skill file (#156)",
			target:     "claude",
			candidates: []string{".claude/skills/owasp-quick-reference/SKILL.md"},
			want:       false,
		},
		{
			name:       "claude preset target does NOT match nested agent file (#156)",
			target:     "claude",
			candidates: []string{".claude/agents/code-reviewer.md"},
			want:       false,
		},
		{
			name:       "explicit skill path target still matches that skill file",
			target:     ".claude/skills/foo/SKILL.md",
			candidates: []string{".claude/skills/foo/SKILL.md"},
			want:       true,
		},
		{
			name:       "explicit .claude/ directory target matches descendants",
			target:     ".claude/skills/",
			candidates: []string{".claude/skills/foo/SKILL.md"},
			want:       true,
		},
		{
			name:       "glob target still matches",
			target:     ".claude/skills/*/SKILL.md",
			candidates: []string{".claude/skills/bar/SKILL.md"},
			want:       true,
		},
		{
			name:       "exact file target matches",
			target:     "GEMINI.md",
			candidates: []string{"GEMINI.md"},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := targetMatchesOutput(tt.target, tt.candidates)
			if got != tt.want {
				t.Errorf("targetMatchesOutput(%q, %v) = %v, want %v", tt.target, tt.candidates, got, tt.want)
			}
		})
	}
}
