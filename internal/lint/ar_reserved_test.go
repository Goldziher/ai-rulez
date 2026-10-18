package lint

import (
	"strings"
	"testing"
)

func reservedSkill(name string) string {
	return "---\nname: " + name + "\ndescription: Use when testing reserved names of the lint rules.\n---\nbody\n"
}

// lintDetected lints the files the way `validate` does: the content tree plus
// the hand-written files a convention names (.claude/skills, ...).
func lintDetected(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	gitAdd(t, root)
	got, _ := lintWithTargets(t, root, TargetSpec{Detect: true})
	return got
}

func TestReservedNameAR808(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		config string
		want   []string // paths expected to carry AR808
		msg    string
	}{
		{
			name:  "authored skill shadows a built-in",
			files: map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			want:  []string{"skills/init/SKILL.md"},
			msg:   `skill "init" shadows the Claude Code built-in /init (preset claude)`,
		},
		{
			name:  "hand-written claude skills",
			files: map[string]string{".claude/skills/review/SKILL.md": reservedSkill("review"), ".claude/skills/security-review/SKILL.md": reservedSkill("security-review")},
			want:  []string{"skills/review/SKILL.md", "skills/security-review/SKILL.md"},
		},
		{
			name:  "command shadows a built-in",
			files: map[string]string{".ai-rulez/commands/clear.md": "---\ndescription: Clear things up for the test.\n---\nbody\n"},
			want:  []string{"commands/clear.md"},
			msg:   `command "clear" shadows the Claude Code built-in /clear (preset claude)`,
		},
		{
			name:  "name is normalized",
			files: map[string]string{".ai-rulez/skills/x/SKILL.md": reservedSkill(" Add_Dir ")},
			want:  []string{"skills/x/SKILL.md"},
		},
		{
			name:  "ordinary name is fine",
			files: map[string]string{".ai-rulez/skills/deploy-helper/SKILL.md": reservedSkill("deploy-helper")},
		},
		{
			name:   "allowed by name",
			files:  map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nallow_overrides = [\"init\"]\n",
		},
		{
			name:   "allowed by preset and name",
			files:  map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nallow_overrides = [\"claude:Init\"]\n",
		},
		{
			name:   "another preset entry does not allow it",
			files:  map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			config: "\n[lint]\nallow_overrides = [\"codex:init\"]\n",
			want:   []string{"skills/init/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{".ai-rulez/config.toml": baseConfig + tt.config}
			for k, v := range tt.files {
				files[k] = v
			}
			got := lintDetected(t, files)
			n := 0
			for _, f := range got {
				if f.Code != "AR808" {
					continue
				}
				n++
				if f.Severity != SeverityError {
					t.Errorf("severity = %s, want error", f.Severity)
				}
				if tt.msg != "" && !strings.Contains(f.Message, tt.msg) {
					t.Errorf("message %q lacks %q", f.Message, tt.msg)
				}
			}
			if n != len(tt.want) {
				t.Fatalf("AR808 count = %d, want %d:\n%s", n, len(tt.want), dump(got))
			}
			for _, w := range tt.want {
				if !has(got, "AR808", w, 0) {
					t.Errorf("missing AR808 on %s:\n%s", w, dump(got))
				}
			}
		})
	}
}

func TestReservedNameConfigless(t *testing.T) {
	files := map[string]string{
		"ai-rulez.toml":                           "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n",
		".claude/skills/init/SKILL.md":            reservedSkill("init"),
		".claude/skills/review/SKILL.md":          reservedSkill("review"),
		".claude/skills/security-review/SKILL.md": reservedSkill("security-review"),
	}
	got := lintDetected(t, files)
	if n := countCode(got, "AR808"); n != 3 {
		t.Fatalf("AR808 count = %d, want 3:\n%s", n, dump(got))
	}
}

func TestReservedNameOtherPresetOnly(t *testing.T) {
	files := map[string]string{
		".ai-rulez/config.toml":          strings.Replace(baseConfig, `presets = ["claude"]`, `presets = ["cursor"]`, 1),
		".ai-rulez/skills/init/SKILL.md": reservedSkill("init"),
	}
	if got := lintDetected(t, files); countCode(got, "AR808") != 0 {
		t.Fatalf("AR808 reported without the claude preset:\n%s", dump(got))
	}
}
