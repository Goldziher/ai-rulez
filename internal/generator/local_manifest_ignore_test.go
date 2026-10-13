package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_LocalManifestIgnoredOnFirstRun(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["vibe", "claude"]`)

	// Act
	generateRepo(t, root)
	first, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	generateRepo(t, root)
	second, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)

	// Assert
	assert.Contains(t, string(first), ".ai-rulez/.generated-manifest.local.json")
	assert.Equal(t, string(first), string(second), "run 1 and run 2 write the same .gitignore")
}

func TestGenerate_LocalManifestIgnoredWithoutManagedIgnores(t *testing.T) {
	tests := []struct {
		name    string
		presets string
	}{
		{name: "digest-only manifest", presets: `["claude"]`},
		{name: "several presets", presets: `["claude", "cursor", "codex"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := newSecretMCPRepo(t, tt.presets)
			cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
			data, err := os.ReadFile(cfgPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(cfgPath, []byte(strings.Replace(string(data), "gitignore = true\n", "", 1)), 0o644))

			// Act
			generateRepo(t, root)
			first, err := os.ReadFile(filepath.Join(root, ".gitignore"))
			require.NoError(t, err)
			generateRepo(t, root)
			second, err := os.ReadFile(filepath.Join(root, ".gitignore"))
			require.NoError(t, err)

			// Assert
			assert.Contains(t, string(first), ".ai-rulez/.generated-manifest.local.json")
			assert.NotContains(t, string(first), ".ai-rulez/local/", "the manifest does not imply the local content tree")
			assert.NotContains(t, string(first), ".ai-rulez/.generated-manifest.json\n", "the shared manifest stays committed")
			assert.Equal(t, string(first), string(second), "run 1 and run 2 write the same .gitignore")
		})
	}
}
