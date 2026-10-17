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

// listFiles lists the files below root that are not ignored: git's own listing
// (tracked files and untracked files .gitignore does not exclude) in a
// repository, else a walk that skips .git and node_modules. Paths are slash
// separated and relative to root; a symlink may be among them, and the reader
// refuses it.
func listFiles(ctx context.Context, root string) []string {
	if files, ok, err := (gitutil.Git{}).ListFilesContext(ctx, root); ok && err == nil {
		return files
	}
	var files []string
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
		if rel, relErr := filepath.Rel(root, p); relErr == nil && entry.Type().IsRegular() {
			files = append(files, filepath.ToSlash(rel))
		}
		if len(files) >= maxWalk {
			return fs.SkipAll
		}
		return nil
	})
	return files
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
