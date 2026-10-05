package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
)

// localOnDisk lays out a project whose overlay and local/ tree exist on disk,
// without loading them into the config (as plugin mode or --no-local would).
func localOnDisk(t *testing.T, withLocal bool) *config.Config {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	if withLocal {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte("name = \"x\"\n"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "local", "rules"), 0o755))
	}
	return &config.Config{BaseDir: base, ConfigDir: dir, ConfigDirName: ".ai-rulez"}
}

func TestCollectGitignorePaths_KeepsLocalPatternsWhenLocalWasNotLoaded(t *testing.T) {
	tests := []struct {
		name      string
		withLocal bool
		want      bool
	}{
		{"overlay and local tree on disk", true, true},
		{"nothing local on disk", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			gen := NewGenerator(localOnDisk(t, tt.withLocal))

			// Act
			patterns := gen.collectGitignorePaths(nil)

			// Assert
			assert.Equal(t, tt.want, patterns[".ai-rulez/config.local.*"])
			assert.Equal(t, tt.want, patterns[".ai-rulez/local/"])
		})
	}
}

func TestStripGitignoreManagedBlock_KeepsLocalPatterns(t *testing.T) {
	tests := []struct {
		name      string
		withLocal bool
		want      string
	}{
		{
			"local files exist",
			true,
			"node_modules/\n\n" + gitignore.BeginMarker + "\n.ai-rulez/config.local.*\n.ai-rulez/local/\n" + gitignore.EndMarker + "\n",
		},
		{"no local files", false, "node_modules/\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := localOnDisk(t, tt.withLocal)
			ignore := filepath.Join(cfg.BaseDir, ".gitignore")
			content := "node_modules/\n\n" + gitignore.BeginMarker + "\nAGENTS.md\n.ai-rulez/config.local.*\n" + gitignore.EndMarker + "\n"
			require.NoError(t, os.WriteFile(ignore, []byte(content), 0o600))

			// Act
			err := NewGenerator(cfg).stripGitignoreManagedBlock()

			// Assert
			require.NoError(t, err)
			got, err := os.ReadFile(ignore)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}
