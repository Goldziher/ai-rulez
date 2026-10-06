package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeSetupFromFlags_MaxCloneBytes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int64
	}{
		{"unset leaves the environment variable and the default in charge", nil, 0},
		{"the flag sets the limit", []string{"--max-clone-bytes", "1048576"}, 1048576},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			require.NoError(t, MCPCmd.Flags().Parse(tt.args))
			t.Cleanup(func() { _ = MCPCmd.Flags().Set(flagServeMaxClone, "0") })

			// Act
			setup, err := serveSetupFromFlags(MCPCmd)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, setup.MaxCloneBytes)
		})
	}
}

func TestMCPCmd_ServeOnlyFlagsIncludeMaxCloneBytes(t *testing.T) {
	assert.Contains(t, dynamicServeFlagNames, flagServeMaxClone)
	assert.NotNil(t, MCPCmd.Flags().Lookup(flagServeMaxClone))
}
