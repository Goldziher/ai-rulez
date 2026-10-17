package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// discoverProjectRoot returns the directory to load the project from: the
// nearest directory at or above the working directory that holds a
// configuration in any layout. It is "." when that is the working directory.
// A legacy configuration is not a walk-up result: the loader of "." reports
// it. A missing configuration is returned as the search error, which names
// where it looked.
func discoverProjectRoot(ctx context.Context) (string, error) {
	cwd := workingDir()
	if cwd == "" {
		return ".", nil
	}
	root, err := config.FindProjectRoot(cwd, discoveryStopDir(ctx, cwd))
	if err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			return "", err //nolint:wrapcheck // already contextual
		}
		return ".", nil
	}
	if resolvedDir(root) == resolvedDir(cwd) {
		return ".", nil
	}
	return root, nil
}

// discoveryStopDir is the last directory the walk-up searches: the git work
// tree root, else the file system root, and never a directory above the home
// directory.
func discoveryStopDir(ctx context.Context, cwd string) string {
	stop := (gitutil.Git{}).TopLevelContext(ctx, cwd)
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !withinDir(cwd, home) {
		return stop
	}
	if stop == "" || (resolvedDir(stop) != resolvedDir(home) && withinDir(home, stop)) {
		return home
	}
	return stop
}

// withinDir reports whether path is dir or below it, resolving symlinks.
func withinDir(path, dir string) bool {
	rel, err := filepath.Rel(resolvedDir(dir), resolvedDir(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolvedDir(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}
