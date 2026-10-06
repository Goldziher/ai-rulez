package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPolicyWarnModeReportsWithoutFailing(t *testing.T) {
	violation := PolicyViolation{Code: "AR740", File: ".ai-rulez/config.toml", Line: 4, Message: "below the floor"}
	tests := []struct {
		name         string
		warn         bool
		wantErr      bool
		wantWarnings []string
	}{
		{"enforce fails", false, true, nil},
		{"warn mode returns the lines instead", true, false, []string{"AR740 .ai-rulez/config.toml:4  below the floor"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{PolicyOutcome: &PolicyOutcome{Warn: tt.warn, Violations: []PolicyViolation{violation}}}
			// Act
			err := CheckPolicy(cfg)
			warnings := PolicyWarnings(cfg)
			// Assert
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantWarnings, warnings)
		})
	}
}

func TestPolicyWarningsAreEmptyWithoutAPolicy(t *testing.T) {
	assert.Nil(t, PolicyWarnings(nil))
	assert.Nil(t, PolicyWarnings(&Config{}))
}
