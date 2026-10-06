package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogHTMLCheck(t *testing.T) {
	// Arrange
	hostileCatalogProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir = dir
	require.NoError(t, runCatalog(&bytes.Buffer{}))
	before := treeOf(t, dir)

	tests := []struct {
		name      string
		tamper    func(t *testing.T)
		wantDrift bool
		wantLines []string
	}{
		{"fresh site passes", func(*testing.T) {}, false, []string{"is up to date"}},
		{"edited page", func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "about.html"), []byte("tampered"), 0o644))
		}, true, []string{"changed  about.html"}},
		{"deleted page", func(t *testing.T) {
			require.NoError(t, os.Remove(filepath.Join(dir, "lock.html")))
		}, true, []string{"missing  lock.html"}},
		{"unexpected file", func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "evil.js"), []byte("x"), 0o644))
		}, true, []string{"unexpected  assets/evil.js"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetDir(t, dir, before)
			tt.tamper(t)
			catalogCheck = true
			t.Cleanup(func() { catalogCheck = false })
			var out bytes.Buffer

			// Act
			err := runCatalog(&out)

			// Assert
			if tt.wantDrift {
				require.ErrorIs(t, err, errCatalogDrift)
			} else {
				require.NoError(t, err)
			}
			for _, line := range tt.wantLines {
				assert.Contains(t, out.String(), line)
			}
		})
	}
}

// resetDir restores dir to the files in tree, removing anything else.
func resetDir(t *testing.T, dir string, tree map[string]string) {
	t.Helper()
	require.NoError(t, os.RemoveAll(dir))
	for name, content := range tree {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
}

func TestCatalogHTMLCheckWritesNothing(t *testing.T) {
	// Arrange
	hostileCatalogProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir, catalogCheck = dir, true
	var out bytes.Buffer

	// Act
	err := runCatalog(&out)

	// Assert
	require.ErrorIs(t, err, errCatalogDrift)
	assert.NoDirExists(t, dir, "a missing site is drift, and --check never creates it")
	assert.Contains(t, out.String(), "missing  index.html")
}

func TestCatalogCheckFlagValidation(t *testing.T) {
	tests := []struct {
		name string
		set  func()
		want string
	}{
		{"without html", func() { catalogCheck = true }, "--check applies to --html only"},
		{"with clean", func() { catalogCheck, catalogHTMLDir, catalogClean = true, "x", true }, "drop --clean"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetCatalogFlags(t)
			tt.set()

			// Act
			err := checkCatalogFlags()

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
