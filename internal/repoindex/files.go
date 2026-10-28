package repoindex

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

const (
	dotGit      = ".git"
	nodeModules = "node_modules"
	configDir   = ".ai-rulez"

	// maxWalk bounds the files a walk of a directory that is not a git
	// repository lists.
	maxWalk = 200000
)

// ListFiles lists the files below root that are not ignored: git's own listing
// (tracked files and untracked files .gitignore does not exclude) in a
// repository, else a walk that skips .git and node_modules. Paths are slash
// separated and relative to root. A symlink may be among them in both modes
// (the walk keeps every symlink that does not resolve to a directory, dangling
// ones included), and the reader decides whether to follow it: ReadRegular
// refuses it.
func ListFiles(ctx context.Context, root string) []string {
	files, _ := ListFilesCapped(ctx, root)
	return files
}

// ListFilesCapped is ListFiles that also reports whether the walk outside a git
// repository stopped at its limit, so a caller that counts files can say the
// listing is incomplete.
func ListFilesCapped(ctx context.Context, root string) (files []string, capped bool) {
	if files, ok, err := (gitutil.Git{}).ListFilesContext(ctx, root); ok && err == nil {
		return files, false
	}
	return walkFiles(root, maxWalk)
}

// walkFiles lists up to limit files below root, skipping .git and node_modules.
func walkFiles(root string, limit int) (files []string, capped bool) {
	//nolint:errcheck // an unreadable entry is left out
	_ = filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is left out
		}
		if entry.IsDir() {
			if name := entry.Name(); name == dotGit || name == nodeModules {
				return fs.SkipDir
			}
			return nil
		}
		if len(files) >= limit {
			capped = true
			return fs.SkipAll
		}
		if rel, relErr := filepath.Rel(root, p); relErr == nil && listable(p, entry) {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files, capped
}

// listable reports whether a walked entry that is not a directory is listed:
// a regular file, or a symlink that does not resolve to a directory.
func listable(p string, entry fs.DirEntry) bool {
	switch {
	case entry.Type().IsRegular():
		return true
	case entry.Type()&fs.ModeSymlink == 0:
		return false
	}
	info, err := os.Stat(p)
	return err != nil || !info.IsDir()
}

// ReadRegular reads a regular file below root of at most limit bytes. ok is
// false, without an error, for a path that is gone, a symlink or another
// non-regular file: the index never follows a link. rel must come from the
// index, never from a caller's argument.
func ReadRegular(root, rel string, limit int64) (data []byte, ok bool, err error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err //nolint:wrapcheck // the caller names the path
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	if info.Size() > limit {
		return nil, false, ErrTooLarge
	}
	data, err = os.ReadFile(abs) //nolint:gosec // abs is below the project root and was just checked to be a regular file
	if err != nil {
		return nil, false, err //nolint:wrapcheck // the caller names the path
	}
	return data, true, nil
}
