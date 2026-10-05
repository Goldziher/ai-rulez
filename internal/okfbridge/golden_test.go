package okfbridge_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regenerate with: UPDATE_GOLDEN=1 go test ./internal/okfbridge -run Golden
func TestExportGolden(t *testing.T) {
	res := exportProject(t, sampleProject(t))
	golden := filepath.Join("testdata", "golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.RemoveAll(golden))
		for _, f := range res.Files {
			p := filepath.Join(golden, filepath.FromSlash(f.Path))
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
			require.NoError(t, os.WriteFile(p, f.Data, 0o644))
		}
	}
	var want []string
	require.NoError(t, filepath.WalkDir(golden, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(golden, p)
		want = append(want, filepath.ToSlash(rel))
		return relErr
	}))
	sort.Strings(want)
	var got []string
	for _, f := range res.Files {
		got = append(got, f.Path)
		data, err := os.ReadFile(filepath.Join(golden, filepath.FromSlash(f.Path)))
		require.NoError(t, err, f.Path)
		assert.Equal(t, string(data), string(f.Data), f.Path)
	}
	assert.Equal(t, want, got, "the golden bundle and the export list the same files")
}
