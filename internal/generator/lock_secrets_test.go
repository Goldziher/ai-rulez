package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_DoesNotMutateAsWrittenMCPServers(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["claude"]`)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)

	// Act
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	// Assert: the lock digests MCPServersRaw, so it must still be as written.
	require.Len(t, cfg.MCPServersRaw, 1)
	assert.Equal(t, "${GITHUB_TOKEN}", cfg.MCPServersRaw[0].Env["GITHUB_TOKEN"])
	assert.Equal(t, []string{"-y", "gh-mcp", "${PROJECT_ROOT}"}, cfg.MCPServersRaw[0].Args)
	assert.Empty(t, cfg.MCPServersRaw[0].SecretEnvKeys)
}

func TestLockOutputs_NeverPinSecretBearingOutputs(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["claude", "vibe", "cursor"]`)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)

	// Act
	pins, err := NewGenerator(cfg).LockOutputs("default")

	// Assert
	require.NoError(t, err)
	for _, pin := range pins {
		assert.NotContains(t, string(pin.Data), leakToken, pin.Path)
		assert.False(t, strings.Contains(string(pin.Data), root), pin.Path)
	}
}
