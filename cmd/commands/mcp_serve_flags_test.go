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

// `mcp --serve-skills` now registers the authoring surface too, so the setup
// carries the authoring options; --read-only narrows them.
func TestServeSetupFromFlags_MergesTheAuthoringSurface(t *testing.T) {
	// Arrange
	require.NoError(t, MCPCmd.Flags().Parse(nil))
	t.Cleanup(func() { _ = MCPCmd.Flags().Set(flagMCPReadOnly, "false") })

	// Act
	setup, err := serveSetupFromFlags(MCPCmd)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, setup.Authoring, "mcp --serve-skills registers the authoring tools too")
	plain := len(setup.Authoring)

	// Act: --read-only adds the narrowing option.
	require.NoError(t, MCPCmd.Flags().Set(flagMCPReadOnly, "true"))
	setup, err = serveSetupFromFlags(MCPCmd)

	// Assert
	require.NoError(t, err)
	assert.Len(t, setup.Authoring, plain+1, "--read-only adds WithReadOnly")
}
