package lint

import (
	"strings"
	"testing"
)

func TestDiscoveredSkillCollapsedAR703(t *testing.T) {
	repoSkill := "---\nname: testing-conventions\ndescription: Use when writing tests in this project only.\n---\nmine\n"
	tests := []struct {
		name   string
		files  map[string]string
		config string
		want   bool
		msg    []string
	}{
		{
			name:   "repo skill beside a builtin of the same name",
			files:  map[string]string{".claude/skills/testing-conventions/SKILL.md": repoSkill},
			config: "builtins = [\"testing\"]\n",
			want:   true,
			msg:    []string{"builtin://", ".claude/skills/testing-conventions/SKILL.md (kept)"},
		},
		{
			name:   "allowed by name",
			files:  map[string]string{".claude/skills/testing-conventions/SKILL.md": repoSkill},
			config: "builtins = [\"testing\"]\n[lint]\nallow_overrides = [\"testing-conventions\"]\n",
		},
		{
			name:  "repo skill beside an authored skill",
			files: map[string]string{".claude/skills/testing-conventions/SKILL.md": repoSkill, ".ai-rulez/skills/testing-conventions/SKILL.md": repoSkill},
			want:  true,
			msg:   []string{".ai-rulez/skills/testing-conventions/SKILL.md (kept)"},
		},
		{
			name:  "unrelated repo skill",
			files: map[string]string{".claude/skills/other/SKILL.md": strings.ReplaceAll(repoSkill, "testing-conventions", "other")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{".ai-rulez/config.toml": baseConfig + tt.config}
			for k, v := range tt.files {
				files[k] = v
			}
			var got []Finding
			for _, f := range lintDetected(t, files) {
				if f.Code == CodeDuplicateCollapsed {
					got = append(got, f)
				}
			}
			if (len(got) > 0) != tt.want {
				t.Fatalf("AR703 findings = %d, want reported = %v:\n%s", len(got), tt.want, dump(got))
			}
			for _, m := range tt.msg {
				if len(got) == 0 || !strings.Contains(got[0].Message, m) {
					t.Errorf("message lacks %q: %v", m, got)
				}
			}
		})
	}
}
