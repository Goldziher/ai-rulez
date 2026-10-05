// Package testutil provides portable test fixtures.
package testutil

import (
	"os"
	"path/filepath"
	"strings"
)

// AbsolutePath roots a synthetic slash path on the temporary directory's volume.
// It does not create the path; tests needing files should use testing.T.TempDir.
func AbsolutePath(path string) string {
	root := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)
	return filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, "/")))
}
