package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginRuntimesProject is a plugin authoring project whose bundle covers the
// claude, opencode and cursor runtimes, generated with generate --plugin.
func pluginRuntimesProject(t *testing.T) string {
	t.Helper()
	dir := newDomainsProject(t, "")
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	updated := strings.Replace(string(body), `runtimes = ["claude"]`, `runtimes = ["claude", "opencode", "cursor"]`, 1)
	require.NoError(t, os.WriteFile(cfgPath, []byte(updated), 0o600))
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	return dir
}

// bundleFiles lists the files below dir, outside the config directory.
func bundleFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".ai-rulez" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	}))
	return files
}

func TestClean_RemovesWhatGeneratePluginWrote(t *testing.T) {
	tests := []struct {
		name     string
		edit     string
		wantKept []string
	}{
		{name: "an untouched bundle is removed whole"},
		{
			name: "an edited bundle file is kept", edit: ".opencode/ai-rulez-bundle.json",
			wantKept: []string{".opencode/ai-rulez-bundle.json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietWarnings(t)
			// Arrange
			dir := pluginRuntimesProject(t)
			before := bundleFiles(t, dir)
			require.Contains(t, before, ".opencode/ai-rulez-bundle.json")
			if tt.edit != "" {
				f, err := os.OpenFile(filepath.Join(dir, tt.edit), os.O_APPEND|os.O_WRONLY, 0o600)
				require.NoError(t, err)
				_, err = f.WriteString("\n")
				require.NoError(t, err)
				require.NoError(t, f.Close())
			}

			// Act
			_, err := loadDomainsProject(t, dir).Clean("", CleanOptions{})

			// Assert
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantKept, bundleFiles(t, dir))
		})
	}
}

func TestClean_RemovesTheEmptiedPluginSkillsDirectory(t *testing.T) {
	quietWarnings(t)
	dir := pluginRuntimesProject(t)
	require.DirExists(t, filepath.Join(dir, "skills"), "the bundle writes root skills/")

	_, err := loadDomainsProject(t, dir).Clean("", CleanOptions{})

	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(dir, "skills"))
}
