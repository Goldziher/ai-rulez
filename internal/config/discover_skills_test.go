package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiscoverSkills(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: DiscoverOff},
		{in: "off", want: DiscoverOff},
		{in: " Repo ", want: DiscoverRepo},
		{in: "ALL", want: DiscoverAll},
		{in: "everything", wantErr: true},
		{in: "true", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDiscoverSkills(tt.in)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "discover_skills")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidate_RejectsAnUnknownDiscoverSkillsMode(t *testing.T) {
	cfg := &Config{
		Version: "5.0", Name: "test", Presets: []Preset{{BuiltIn: "claude"}},
		MCP: &MCPConfig{DiscoverSkills: "some"},
	}

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "discover_skills")
}
