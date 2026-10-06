package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCheckImproveConfig_FlagsWhatImproveRunWillIgnore(t *testing.T) {
	zero, tight, growth := 0.0, 0.3, 1.1
	tests := []struct {
		name    string
		improve *config.ImproveConfig
		want    string
	}{
		{"no table", nil, ""},
		{"stricter and neutral keys", &config.ImproveConfig{MinGain: &tight, MaxSkillGrowth: growth, MaxRounds: 5, Isolation: "auto"}, ""},
		{"optimizer and env_pass", &config.ImproveConfig{Optimizer: "evil --x", EnvPass: []string{"TOKEN_X"}}, "optimizer, env_pass"},
		{"a looser gate", &config.ImproveConfig{MinGain: &zero, MaxSkillGrowth: 2}, "min_gain, max_skill_growth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := &runner{cfg: &config.Config{ConfigDir: t.TempDir(), ConfigFile: "config.toml", Improve: tt.improve}, docs: map[string]doc{}}

			// Act
			r.checkImproveConfig()

			// Assert
			if tt.want == "" {
				assert.Empty(t, r.findings)
				return
			}
			require.Len(t, r.findings, 1)
			assert.Equal(t, CodeImproveRepoOptimizerIgnored, r.findings[0].Code)
			assert.Contains(t, r.findings[0].Message, tt.want)
		})
	}
}
