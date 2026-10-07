package catalogsite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestCheck(t *testing.T) {
	want := site(map[string]string{"index.html": "<h1>x</h1>", "items/a.html": "a", "assets/catalog.css": "css"})
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  CheckResult
	}{
		{"up to date", func(*testing.T, string) {}, CheckResult{}},
		{"changed file", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "items", "a.html"), []byte("tampered"), 0o644))
		}, CheckResult{Changed: []string{"items/a.html"}}},
		{"missing file", func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, "assets", "catalog.css")))
		}, CheckResult{Missing: []string{"assets/catalog.css"}}},
		{"extra file", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "evil.js"), []byte("alert(1)"), 0o644))
		}, CheckResult{Extra: []string{"assets/evil.js"}}},
		{"missing marker", func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, MarkerFile)))
		}, CheckResult{Missing: []string{MarkerFile}}},
		{"everything at once", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644))
			require.NoError(t, os.Remove(filepath.Join(dir, "items", "a.html")))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "z.txt"), []byte("x"), 0o644))
		}, CheckResult{Missing: []string{"items/a.html"}, Changed: []string{"index.html"}, Extra: []string{"z.txt"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := filepath.Join(t.TempDir(), "site")
			_, err := Write(nil, dir, want, false)
			require.NoError(t, err)
			tt.setup(t, dir)

			// Act
			got, err := Check(dir, want)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want.Missing, got.Missing)
			assert.Equal(t, tt.want.Changed, got.Changed)
			assert.Equal(t, tt.want.Extra, got.Extra)
			assert.Equal(t, tt.want.Missing != nil || tt.want.Changed != nil || tt.want.Extra != nil, got.Drift())
		})
	}
}

func TestCheck_MissingDirectoryIsDriftAndWritesNothing(t *testing.T) {
	// Arrange
	dir := filepath.Join(t.TempDir(), "never-generated")

	// Act
	got, err := Check(dir, site(map[string]string{"index.html": "x"}))

	// Assert
	require.NoError(t, err)
	assert.True(t, got.Drift())
	assert.Equal(t, []string{MarkerFile, "index.html"}, got.Missing)
	assert.NoDirExists(t, dir)
}

func TestCheck_AFileIsNotADirectory(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	// Act
	_, err := Check(path, site(map[string]string{"index.html": "x"}))

	// Assert
	require.Error(t, err)
}

func TestCheck_SymlinksAreReportedNotFollowed(t *testing.T) {
	// Arrange
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside.html")
	require.NoError(t, os.WriteFile(outside, []byte("<h1>x</h1>"), 0o644))
	want := site(map[string]string{"index.html": "<h1>x</h1>"})
	dir := filepath.Join(parent, "site")
	_, err := Write(nil, dir, want, false)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(dir, "index.html")))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, "index.html"))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, "extra-link.html"))

	// Act
	got, err := Check(dir, want)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"index.html"}, got.Changed, "identical bytes behind a symlink still differ")
	assert.Equal(t, []string{"extra-link.html"}, got.Extra)
}
