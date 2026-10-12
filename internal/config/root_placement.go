package config

import (
	"os"
	"path/filepath"
)

// rootRelInRepo locates baseDir below the root of the git work tree holding it,
// in forward slashes. It is empty for the work tree's own root and when baseDir
// is outside any work tree. Generated headers use it to name a nested root's
// own config by its path from the repository root.
//
// It looks for a .git entry (a directory, or a file in a linked worktree)
// instead of running git, so a load stays free of a subprocess and the answer
// is the same for `generate`, `generate --recursive` and `generate --check`.
func rootRelInRepo(baseDir string) string {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return ""
	}
	for dir := abs; ; {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			rel, err := filepath.Rel(dir, abs)
			if err != nil || rel == "." {
				return ""
			}
			return filepath.ToSlash(rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
