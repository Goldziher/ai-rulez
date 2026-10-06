package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImprove_SiblingNativeFlag(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func()
		want    string
		wantErr string
	}{
		{"off by default", func() {}, "", ""},
		{"on with the default runs", func() { improveFlags.siblingNative = true }, "siblings:    native guard on: 3 run(s) per trigger prompt", ""},
		{"on with chosen runs", func() { improveFlags.siblingNative, improveFlags.siblingRuns = true, 5 }, "native guard on: 5 run(s)", ""},
		{"runs without the guard", func() { improveFlags.siblingRuns = 4 }, "", "--sibling-runs needs --sibling-native"},
		{"runs must be positive", func() { improveFlags.siblingNative, improveFlags.siblingRuns = true, -1 }, "", "--sibling-runs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			setupImproveRun(t)
			improveFlags.dryRun, improveFlags.format = true, formatText
			improveFlags.with = "/nonexistent/optimizer"
			tt.mutate()
			var out bytes.Buffer
			improveRunCmd.SetOut(&out)
			improveRunCmd.SetErr(&bytes.Buffer{})

			// Act
			_, err := runImprove(improveRunCmd, "deploy")

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.want == "" {
				assert.NotContains(t, out.String(), "native guard")
				return
			}
			assert.Contains(t, out.String(), tt.want)
		})
	}
}
