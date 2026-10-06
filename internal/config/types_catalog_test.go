package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_CatalogTable(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	configDir := filepath.Join(dir, aiRulezDirName)
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	body := `version = "4.0"
name = "catalog-test"
presets = ["claude"]

[catalog]
title = "Acme catalog"
include_excerpt = false
exclude_owners = true
indexable = true
max_items_per_page = 50
render_markdown = true
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, configTOMLFilename), []byte(body), 0o644))

	// Act
	cfg, err := LoadConfig(context.Background(), dir)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, cfg.Catalog)
	assert.Equal(t, "Acme catalog", cfg.Catalog.Title)
	require.NotNil(t, cfg.Catalog.IncludeExcerpt)
	assert.False(t, *cfg.Catalog.IncludeExcerpt)
	assert.True(t, cfg.Catalog.ExcludeOwners)
	assert.True(t, cfg.Catalog.Indexable)
	assert.Equal(t, 50, cfg.Catalog.MaxItemsPerPage)
	assert.True(t, cfg.Catalog.RenderMarkdown)
}

func TestValidateCatalog(t *testing.T) {
	tests := []struct {
		name    string
		cat     *CatalogConfig
		wantErr string
	}{
		{"absent", nil, ""},
		{"defaults", &CatalogConfig{}, ""},
		{"page size", &CatalogConfig{MaxItemsPerPage: 25}, ""},
		{"negative page size", &CatalogConfig{MaxItemsPerPage: -1}, "max_items_per_page"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := (&Config{Catalog: tt.cat}).validateCatalog()

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMergeConfigDocs_LocalOverlayMergesCatalogKeys(t *testing.T) {
	// Arrange
	shared := map[string]any{"name": "p", "catalog": map[string]any{"title": "Shared", "max_items_per_page": int64(100)}}
	local := map[string]any{"catalog": map[string]any{"title": "Mine"}}

	// Act
	merged, _, err := MergeConfigDocs(shared, local)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"title": "Mine", "max_items_per_page": int64(100)}, merged["catalog"])
}
