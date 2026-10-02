package commands

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalFlagsAreRegistered(t *testing.T) {
	assert.NotNil(t, GenerateCmd.Flags().Lookup("no-local"))
	assert.NotNil(t, ValidateCmd.Flags().Lookup("no-local"))
	assert.NotNil(t, TokensCmd.Flags().Lookup("no-local"))
	assert.Nil(t, VerifyCmd.Flags().Lookup("no-local"), "verify always checks the shared view")
	assert.NotNil(t, GenerateCmd.Flags().Lookup("allow-local-drift"))
}

func TestNoLocalSkipsTheOverlayWhenLoading(t *testing.T) {
	// Arrange
	dir := localProject(t)
	writeFile(t, filepath.Join(dir, "config.local.toml"), "name = \"mine\"\n")
	t.Cleanup(func() { noLocal = false })

	// Act
	merged, err := loadConfigForCommand(context.Background(), nil)
	noLocal = true
	shared, sharedErr := loadConfigForCommand(context.Background(), nil)

	// Assert
	require.NoError(t, err)
	require.NoError(t, sharedErr)
	assert.Equal(t, "mine", merged.Name)
	assert.Equal(t, "shared-name", shared.Name)
	assert.Nil(t, shared.LocalOverlay)
}
