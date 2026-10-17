package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShowAllowed_SkipOutputs(t *testing.T) {
	assert.True(t, showAllowed(strings.Split("skip_outputs", ".")), "a list of output kinds holds no secret")
	kind, ok := showKind([]string{"skip_outputs"})
	require.True(t, ok)
	assert.Equal(t, kindStringList, kind)
}

func TestDescribeLocalOverlay_ShowsSkipOutputs(t *testing.T) {
	tests := []struct {
		name    string
		local   string
		wantRed bool
	}{
		{name: "a list of kinds is shown", local: "skip_outputs = [\"instructions\", \"skills\"]\n"},
		{name: "a value of another type stays withheld", local: "skip_outputs = \"SECRETTYPE\"\n", wantRed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, configDir := overlayProject(t, "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n")
			writeProjectFile(t, configDir, "config.local.toml", tt.local)

			// Act
			_, changes, err := DescribeLocalOverlay(base)

			// Assert
			require.NoError(t, err)
			var found *OverlayChange
			for i := range changes {
				if changes[i].Path == "skip_outputs" {
					found = &changes[i]
				}
			}
			require.NotNil(t, found)
			assert.Equal(t, tt.wantRed, found.Redacted)
		})
	}
}
