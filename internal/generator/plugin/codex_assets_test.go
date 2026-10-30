package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// codexAssetManifest lays out an output root (sourceDir) that also holds the
// plugin config dir, like plugin/ with plugin/.ai-rulez.
func codexAssetManifest(t *testing.T, outputAsset *string, sourceAsset string) (*Manifest, string) {
	t.Helper()
	sourceDir := t.TempDir()
	configDir := filepath.Join(sourceDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "assets", "icon.svg"), []byte(sourceAsset), 0o644))
	if outputAsset != nil {
		require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "assets"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "assets", "icon.svg"), []byte(*outputAsset), 0o644))
	}
	cfg := &config.Config{BaseDir: sourceDir, ConfigDir: configDir}
	return &Manifest{
		Config:    cfg,
		SourceDir: sourceDir,
		Interface: &config.PluginInterface{ComposerIcon: "./assets/icon.svg"},
	}, filepath.Join(sourceDir, "out")
}

func TestBundleCodexAssetsResolveAgainstContentRoot(t *testing.T) {
	stale := "stale"
	tests := []struct {
		name        string
		outputAsset *string
	}{
		{name: "stale output copy is ignored", outputAsset: &stale},
		{name: "missing output assets dir does not error", outputAsset: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, baseDir := codexAssetManifest(t, tt.outputAsset, "fresh")

			outputs, err := bundleCodexAssets(m, baseDir)

			require.NoError(t, err)
			require.Len(t, outputs, 1)
			assert.Equal(t, filepath.Join(baseDir, "assets", "icon.svg"), outputs[0].Path)
			assert.Equal(t, "fresh", string(outputs[0].RawContent))
		})
	}
}

func TestBundleCodexAssetsFallBackToSourceDir(t *testing.T) {
	sourceDir := t.TempDir()
	configDir := filepath.Join(sourceDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "assets", "icon.svg"), []byte("beside"), 0o644))
	m := &Manifest{
		Config:    &config.Config{BaseDir: sourceDir, ConfigDir: configDir},
		SourceDir: sourceDir,
		Interface: &config.PluginInterface{ComposerIcon: "./assets/icon.svg"},
	}

	outputs, err := bundleCodexAssets(m, sourceDir)

	require.NoError(t, err)
	require.Len(t, outputs, 1)
	assert.Equal(t, "beside", string(outputs[0].RawContent))
}
