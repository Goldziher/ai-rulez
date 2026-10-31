package commands

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

// TestGenerateRecordsOKFIndexesInManifest is the regression test for #533: the
// index.md files generate refreshes in an OKF configuration directory were
// emitted but absent from .generated-manifest.json, so ownership detection
// treated them as authored sources.
func TestGenerateRecordsOKFIndexesInManifest(t *testing.T) {
	// Arrange: an OKF bundle with a nested skill tree.
	root := t.TempDir()
	configDir := filepath.Join(root, ".ai-rulez")
	writeFile(t, filepath.Join(configDir, "config.toml"),
		"version = \"5.0\"\nname = \"okf-ownership\"\npresets = [\"claude\"]\ngitignore = false\n")
	writeFile(t, filepath.Join(configDir, "skills", "demo", "SKILL.md"),
		"---\nname: demo\ndescription: Demonstrate emitted index ownership.\n---\n\nUse the demo.\n")
	_, err := okfbridge.MigrateDir(t.Context(), configDir, okfbridge.MigrateOptions{Write: true})
	require.NoError(t, err)

	t.Cleanup(func() { pluginMode, dryRun, profile = false, false, "" })
	pluginMode, dryRun, profile = false, false, ""
	progress.SetQuiet(true)
	t.Cleanup(func() { progress.SetQuiet(false) })

	// Act: twice, because the second run reads the manifest the first wrote.
	counter := progress.NewFileCounter(1, "Processing configurations")
	_, err = processConfigFile(filepath.Join(configDir, "config.toml"), counter, nil, nil)
	require.NoError(t, err)
	counter.Finish()
	counter = progress.NewFileCounter(1, "Processing configurations")
	_, err = processConfigFile(filepath.Join(configDir, "config.toml"), counter, nil, nil)
	require.NoError(t, err)
	counter.Finish()

	// Assert: every index generate emits is owned by the manifest.
	data, err := os.ReadFile(filepath.Join(configDir, ".generated-manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal(data, &manifest))
	owned := map[string]bool{}
	for _, f := range manifest.Files {
		owned[f] = true
	}
	for _, want := range []string{
		".ai-rulez/index.md",
		".ai-rulez/skills/index.md",
		".ai-rulez/skills/demo/index.md",
	} {
		assert.True(t, owned[want], "%s must be recorded as generated (manifest: %v)", want, manifest.Files)
		assert.FileExists(t, filepath.Join(root, filepath.FromSlash(want)))
	}
	// Every index the run emitted, not only the ones this fixture names, is owned.
	require.NoError(t, filepath.WalkDir(configDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "index.md" {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		require.NoError(t, relErr)
		assert.True(t, owned[filepath.ToSlash(rel)], "%s is emitted but not in the manifest", rel)
		return nil
	}))
}

// TestRefreshIndexesFilesReportsOwnedIndexes keeps the okfbridge half honest: it
// returns every index it wrote or found current, including a nested one.
func TestRefreshIndexesFilesReportsOwnedIndexes(t *testing.T) {
	configDir := t.TempDir()
	writeFile(t, filepath.Join(configDir, "skills", "demo", "SKILL.md"),
		"---\nname: demo\ndescription: A skill.\n---\nbody\n")
	_, err := okfbridge.MigrateDir(t.Context(), configDir, okfbridge.MigrateOptions{Write: true})
	require.NoError(t, err)

	got, err := okfbridge.RefreshIndexesFilesFrom(t.Context(), configDir, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"index.md", "skills/demo/index.md", "skills/index.md"}, got)
}
