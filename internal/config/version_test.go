package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		version string
		wantErr string
	}{
		{version: "5.0"},
		{version: "4.0", wantErr: "ai-rulez migrate v5"},
		{version: "4.1", wantErr: "ai-rulez migrate v5"},
		{version: "3.0", wantErr: "install ai-rulez 4.x"},
		{version: "2.0", wantErr: "install ai-rulez 4.x"},
		{version: "6.0", wantErr: "invalid version"},
		{version: "", wantErr: "missing required key: version"},
		{version: "dev", wantErr: "invalid version"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			err := CheckVersion(tt.version)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestIsLegacyVersion(t *testing.T) {
	assert.True(t, IsLegacyVersion("3.0"))
	assert.True(t, IsLegacyVersion("4.0"))
	assert.True(t, IsLegacyVersion("4"))
	assert.False(t, IsLegacyVersion("5.0"))
	assert.False(t, IsLegacyVersion(""))
	assert.False(t, IsLegacyVersion("abc"))
}
