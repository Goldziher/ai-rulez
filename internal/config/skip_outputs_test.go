package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSkipOutputs(t *testing.T) {
	tests := []struct {
		name    string
		skip    []string
		wantErr string
	}{
		{name: "empty is valid"},
		{name: "every documented value", skip: SkipOutputValues()},
		{name: "unknown value names the valid ones", skip: []string{"instructions", "bogus"}, wantErr: "bogus"},
		{name: "case matters", skip: []string{"Instructions"}, wantErr: "Instructions"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{SkipOutputs: tc.skip}

			// Act
			err := cfg.validateSkipOutputs()

			// Assert
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestSkipsAgentsMD(t *testing.T) {
	tests := []struct {
		skip []string
		want bool
	}{
		{nil, false},
		{[]string{SkipClaudeMD}, false},
		{[]string{SkipAgentsMD}, true},
		{[]string{SkipInstructions}, true},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, (&Config{SkipOutputs: tc.skip}).SkipsAgentsMD(), "%v", tc.skip)
	}
	var nilCfg *Config
	assert.False(t, nilCfg.SkipsAgentsMD())
}
