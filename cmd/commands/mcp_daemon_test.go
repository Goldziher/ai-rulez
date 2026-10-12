package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `mcp --serve-skills --daemon` sets the daemon mode and an identity derived
// from the command line, so two entries that differ in a flag never share one.
func TestServeSetupFromFlags_Daemon(t *testing.T) {
	// Arrange
	require.NoError(t, MCPCmd.Flags().Parse([]string{"--serve-skills", "--daemon"}))
	t.Cleanup(func() {
		_ = MCPCmd.Flags().Set(flagServeDaemon, "false")
		_ = MCPCmd.Flags().Set("serve-skills", "false")
	})

	// Act
	setup, err := serveSetupFromFlags(MCPCmd)

	// Assert
	require.NoError(t, err)
	assert.True(t, setup.Daemon)
	assert.Equal(t, daemonIdentity(), setup.DaemonIdentity)
	assert.NotEmpty(t, setup.DaemonIdentity)
}

// The daemon flag belongs to the serve-skills mode, so a bare `mcp --daemon` is
// rejected like the other serve-only flags.
func TestMCPCmd_DaemonIsAServeOnlyFlag(t *testing.T) {
	assert.Contains(t, dynamicServeFlagNames, flagServeDaemon)
	assert.NotNil(t, MCPCmd.Flags().Lookup(flagServeDaemon))
}

// The daemon identity is stable for one command line and names the version.
func TestDaemonIdentity_IsStable(t *testing.T) {
	assert.Equal(t, daemonIdentity(), daemonIdentity())
	assert.Len(t, daemonIdentity(), 64)
}
