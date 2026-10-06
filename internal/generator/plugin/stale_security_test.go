package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// writeSidecar records outputs (relative path -> content) the way AddProvenance
// does, without writing the files.
func writeSidecar(t *testing.T, dir string, outputs map[string]string) {
	t.Helper()
	doc := provenanceDocument{SchemaVersion: provenanceSchema, Outputs: map[string]provenanceOutput{}}
	for rel, content := range outputs {
		doc.Outputs[rel] = provenanceOutput{ContentHash: hashBytes([]byte(content))}
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, provenanceFileName), data, 0o600))
}

func TestRemoveGeneratedPluginDir_NeverFollowsSymlinks(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir, outside string) (victim string)
	}{
		{"symlinked subdirectory", func(t *testing.T, dir, outside string) string {
			victim := filepath.Join(outside, "victim.txt")
			require.NoError(t, os.WriteFile(victim, []byte("precious"), 0o600))
			testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, "link"))
			writeSidecar(t, dir, map[string]string{"link/victim.txt": "precious"})
			return victim
		}},
		{"file replaced by a link", func(t *testing.T, dir, outside string) string {
			victim := filepath.Join(outside, "victim.txt")
			require.NoError(t, os.WriteFile(victim, []byte("precious"), 0o600))
			testutil.SymlinkOrSkip(t, victim, filepath.Join(dir, "plugin.json"))
			writeSidecar(t, dir, map[string]string{"plugin.json": "precious"})
			return victim
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := filepath.Join(t.TempDir(), "plugins", "old")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			outside := t.TempDir()
			victim := tt.setup(t, dir, outside)

			// Act
			_, _ = RemoveGeneratedPluginDir(dir) //nolint:errcheck // the outside file is what is asserted

			// Assert
			got, err := os.ReadFile(victim)
			require.NoError(t, err, "a file outside the plugin directory was deleted")
			assert.Equal(t, "precious", string(got))
		})
	}
}

func TestRemoveGeneratedPluginDir_KeepsEditedFiles(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte("edited by hand"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte("{}"), 0o600))
	writeSidecar(t, dir, map[string]string{"a.json": "{}", "b.json": "{}"})

	// Act
	kept, err := RemoveGeneratedPluginDir(dir)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "a.json")}, kept)
	assert.NoFileExists(t, filepath.Join(dir, "b.json"))
}

func TestRemoveObsolete_RefusesSymlinkedParent(t *testing.T) {
	// Arrange
	bundle := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("precious"), 0o600))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(bundle, "link"))

	// Act
	err := RemoveObsolete(bundle, filepath.Join(bundle, "link", "victim.txt"))

	// Assert
	require.Error(t, err)
	assert.FileExists(t, victim)
}

func TestStalePluginDirs_IgnoresASymlinkedPluginsDir(t *testing.T) {
	// Arrange
	root := t.TempDir()
	outside := t.TempDir()
	stale := filepath.Join(outside, "old")
	require.NoError(t, os.MkdirAll(stale, 0o755))
	writeSidecar(t, stale, map[string]string{})
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, DomainPluginsDir))

	// Act
	dirs, err := StalePluginDirs(root, map[string]bool{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, dirs)
}

func TestObsoleteFiles_MarksFilesBehindALinkedParentEdited(t *testing.T) {
	// Arrange
	bundle := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "victim.txt"), []byte("precious"), 0o600))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(bundle, "link"))
	previous := provenanceDocument{SchemaVersion: provenanceSchema, Outputs: map[string]provenanceOutput{
		"link/victim.txt": {ContentHash: hashBytes([]byte("precious"))},
	}}
	prev, err := json.Marshal(previous)
	require.NoError(t, err)
	planned, err := json.Marshal(provenanceDocument{SchemaVersion: provenanceSchema, Outputs: map[string]provenanceOutput{}})
	require.NoError(t, err)

	// Act
	got, err := ObsoleteFiles(bundle, prev, planned)

	// Assert
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].Edited, "a file behind a link must be kept")
}
