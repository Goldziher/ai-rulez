package walkutil

import "testing"

func TestInBackupDir(t *testing.T) {
	tests := []struct {
		rel  string
		want bool
	}{
		{".ai-rulez.bak-okf-20261027-123700/skills/demo/SKILL.md", true},
		{".ai-rulez.bak-20261027-123700/rules/a.md", true},
		{"sub/.ai-rulez.bak-okf-1/skills/demo/SKILL.md", true},
		{".ai-rulez/skills/demo/SKILL.md", false},
		{"docs/backup-notes/SKILL.md", false},
		{"bak-okf/SKILL.md", false},
		{".bak-/SKILL.md", false},
		{"SKILL.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			if got := InBackupDir(tt.rel); got != tt.want {
				t.Fatalf("InBackupDir(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}
