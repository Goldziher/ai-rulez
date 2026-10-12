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
	abs, repo, ok := repoRootOf(baseDir)
	if !ok {
		return ""
	}
	rel, err := filepath.Rel(repo, abs)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

// nestedInRepo reports whether baseDir sits inside another ai-rulez root of the
// same git work tree: some directory between it and the work tree root (the root
// included) holds a config.
func nestedInRepo(baseDir string) bool {
	abs, repo, ok := repoRootOf(baseDir)
	if !ok {
		return false
	}
	for dir := abs; dir != repo; {
		dir = filepath.Dir(dir)
		if ResolveConfigDirName(dir) != "" {
			return true
		}
	}
	return false
}

// repoRootOf returns the absolute baseDir and the root of the git work tree
// that holds it; ok is false outside a work tree.
func repoRootOf(baseDir string) (abs, repo string, ok bool) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", false
	}
	for dir := abs; ; {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return abs, dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}
