package testutil

import (
	"os"
	"path/filepath"
	"strings"
)

// CeilGit keeps git, in the test binary and in every process it starts, from
// finding a repository above the directory temporary test directories are made
// in. Without it a test that expects "not a git repository" (or reads the ignore
// rules of the project it just wrote) is decided by whatever repository happens
// to enclose the machine's temporary directory, such as a checkout whose
// TMPDIR is inside it. Call it from TestMain.
func CeilGit() {
	tmp := os.TempDir()
	ceilings := []string{tmp}
	if real, err := filepath.EvalSymlinks(tmp); err == nil && real != tmp {
		ceilings = append(ceilings, real)
	}
	if existing := os.Getenv("GIT_CEILING_DIRECTORIES"); existing != "" {
		ceilings = append(ceilings, strings.Split(existing, string(os.PathListSeparator))...)
	}
	_ = os.Setenv("GIT_CEILING_DIRECTORIES", strings.Join(ceilings, string(os.PathListSeparator))) //nolint:errcheck // best effort: the tests then run as they did
}
