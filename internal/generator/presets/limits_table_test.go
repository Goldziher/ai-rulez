package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

func TestRuleFileLimitsComeFromTheLimitsTable(t *testing.T) {
	// Arrange
	tests := []struct {
		name string
		got  int
		id   string
	}{
		{"devin", devinRulesTarget.MaxChars, "devin.rule_file_chars"},
		{"antigravity", antigravityRulesTarget.MaxChars, "antigravity.rule_file_bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act / Assert
			if want := harnesslimits.MustValue(tt.id); tt.got != want {
				t.Errorf("MaxChars = %d, limits table %s = %d", tt.got, tt.id, want)
			}
		})
	}
}
