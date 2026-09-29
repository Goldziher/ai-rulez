package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const demoSpec = `name = "demo"

[root]
file = "DEMO.md"
sections = ["title"]
`

func TestLoadProviderFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.toml")
	require.NoError(t, os.WriteFile(path, []byte(demoSpec), 0o644))

	gen, err := providers.LoadProviderFile(path)
	require.NoError(t, err)
	assert.Equal(t, "demo", gen.Spec.Name)
}

func TestProviderSpecFactory(t *testing.T) {
	require.NotNil(t, config.ProviderSpecGeneratorFactory, "providers init must register the factory")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "demo.toml"), []byte(demoSpec), 0o644))

	t.Run("resolves a matching provider", func(t *testing.T) {
		gen, err := config.ProviderSpecGeneratorFactory(config.Preset{Name: "demo", Provider: "demo.toml"}, dir)
		require.NoError(t, err)
		assert.Equal(t, "demo", gen.GetName())
	})

	t.Run("rejects a name mismatch", func(t *testing.T) {
		_, err := config.ProviderSpecGeneratorFactory(config.Preset{Name: "other", Provider: "demo.toml"}, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match preset name")
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		_, err := config.ProviderSpecGeneratorFactory(config.Preset{Name: "demo", Provider: "../demo.toml"}, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "escapes the project root")
	})
}

func TestGeneratePresets_ProviderBacked(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, ".ai-rulez", "providers"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(baseDir, ".ai-rulez", "providers", "demo.toml"),
		[]byte(demoSpec), 0o644,
	))

	cfg := &config.Config{
		Version: "4.0",
		Name:    "proj",
		BaseDir: baseDir,
		Content: &config.ContentTree{Rules: []config.ContentFile{{Name: "r", Content: "c"}}},
		Presets: []config.Preset{{Name: "demo", Provider: ".ai-rulez/providers/demo.toml"}},
	}

	results, err := config.GeneratePresets(cfg)
	require.NoError(t, err)
	outputs, ok := results["demo"]
	require.True(t, ok, "provider-backed preset results are keyed by preset name")

	var found bool
	for _, o := range outputs {
		if filepath.ToSlash(o.Path) == filepath.Join(baseDir, "DEMO.md") {
			found = true
		}
	}
	assert.True(t, found, "provider root file must be rendered")
}
