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
	// The tests serve include and skill-source repositories from temporary
	// directories through file:// URLs, which the project config otherwise keeps
	// inside the project.
	_ = os.Setenv("AI_RULEZ_ALLOW_FILE_URLS", "1") //nolint:errcheck // best effort
	tmp := os.TempDir()
	ceilings := []string{tmp}
	if resolved, err := filepath.EvalSymlinks(tmp); err == nil && resolved != tmp {
		ceilings = append(ceilings, resolved)
	}
	if existing := os.Getenv("GIT_CEILING_DIRECTORIES"); existing != "" {
		ceilings = append(ceilings, strings.Split(existing, string(os.PathListSeparator))...)
	}
	_ = os.Setenv("GIT_CEILING_DIRECTORIES", strings.Join(ceilings, string(os.PathListSeparator))) //nolint:errcheck // best effort: the tests then run as they did
}
