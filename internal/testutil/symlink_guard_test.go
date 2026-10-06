package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDirectSymlinkInTests fails when a _test.go file calls the os symlink function
// directly. Tests must go through SymlinkOrSkip or RequireSymlink so platforms
// that refuse symlinks (Windows without the privilege) skip instead of failing.
func TestNoDirectSymlinkInTests(t *testing.T) {
	// Arrange
	root := filepath.Join("..", "..")
	var offenders []string

	// Act
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "site":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // repo test files
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "os"+".Symlink(") {
			offenders = append(offenders, filepath.ToSlash(path))
		}
		return nil
	})

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range offenders {
		t.Errorf("%s calls the os symlink function directly; use testutil.SymlinkOrSkip", o)
	}
}
