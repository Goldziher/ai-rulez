package crud

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestMarketplaceAndPluginLifecycle(t *testing.T) {
	// Arrange
	dir := setupTestProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Act: add a marketplace and install a plugin from it
	require.NoError(t, op.AddMarketplace(ctx, &config.InstalledMarketplaceConfig{Name: "acme", Source: "acme/plugins", Version: "^1.2"}))
	require.NoError(t, op.InstallPlugin(ctx, &config.InstalledPluginConfig{Name: "tools", Marketplace: "acme", EnableHooks: true, Exclude: []string{"mcp"}}))
	marketplaces, err := op.ListMarketplaces(ctx)
	require.NoError(t, err)
	plugins, err := op.ListPlugins(ctx)
	require.NoError(t, err)

	// Assert
	require.Len(t, marketplaces, 1)
	assert.Equal(t, "^1.2", marketplaces[0].Version)
	require.Len(t, plugins, 1)
	assert.True(t, plugins[0].EnableHooks)
	data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "[[installed_marketplaces]]")
	assert.Contains(t, string(data), "[[installed_plugins]]")

	// Act / Assert: a marketplace in use stays
	err = op.RemoveMarketplace(ctx, "acme")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "used by installed plugin")

	// Act: remove in order
	require.NoError(t, op.RemovePlugin(ctx, "tools"))
	require.NoError(t, op.RemoveMarketplace(ctx, "acme"))

	// Assert
	marketplaces, _ = op.ListMarketplaces(ctx) //nolint:errcheck // the project loads
	plugins, _ = op.ListPlugins(ctx)           //nolint:errcheck // the project loads
	assert.Empty(t, marketplaces)
	assert.Empty(t, plugins)
}

func TestMarketplaceAndPluginValidation(t *testing.T) {
	tests := []struct {
		name string
		do   func(op *OperatorImpl) error
		want string
	}{
		{"marketplace without a source", func(op *OperatorImpl) error {
			return op.AddMarketplace(context.Background(), &config.InstalledMarketplaceConfig{Name: "m"})
		}, "required"},
		{"marketplace with credentials", func(op *OperatorImpl) error {
			return op.AddMarketplace(context.Background(), &config.InstalledMarketplaceConfig{Name: "m", Source: "https://u:p@example.com/x.git"})
		}, "credential"},
		{"marketplace over http", func(op *OperatorImpl) error {
			return op.AddMarketplace(context.Background(), &config.InstalledMarketplaceConfig{Name: "m", Source: "http://example.com/x.git"})
		}, "http://"},
		{"plugin from an unknown marketplace", func(op *OperatorImpl) error {
			return op.InstallPlugin(context.Background(), &config.InstalledPluginConfig{Name: "p", Marketplace: "nope"})
		}, "not an installed_marketplaces"},
		{"plugin with credentials", func(op *OperatorImpl) error {
			return op.InstallPlugin(context.Background(), &config.InstalledPluginConfig{Name: "p", Source: "https://tok@example.com/x.git"})
		}, "credential"},
		{"plugin without a name", func(op *OperatorImpl) error {
			return op.InstallPlugin(context.Background(), &config.InstalledPluginConfig{Source: "a/b"})
		}, "name"},
		{"duplicate marketplace", func(op *OperatorImpl) error {
			if err := op.AddMarketplace(context.Background(), &config.InstalledMarketplaceConfig{Name: "m", Source: "a/b"}); err != nil {
				return err
			}
			return op.AddMarketplace(context.Background(), &config.InstalledMarketplaceConfig{Name: "m", Source: "c/d"})
		}, "duplicate"},
		{"remove an unknown plugin", func(op *OperatorImpl) error { return op.RemovePlugin(context.Background(), "nope") }, "not found"},
		{"remove an unknown marketplace", func(op *OperatorImpl) error { return op.RemoveMarketplace(context.Background(), "nope") }, "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := setupTestProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)
			before, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
			require.NoError(t, err)

			// Act
			err = tt.do(op)

			// Assert: refused, and a refused entry leaves nothing in config.toml
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			after, readErr := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
			require.NoError(t, readErr)
			if tt.name != "duplicate marketplace" {
				assert.Equal(t, string(before), string(after))
			}
		})
	}
}
