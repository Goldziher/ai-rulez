package config

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// SearchStopDir is the last directory a walk-up from cwd searches: the git
// work tree root, else the file system root (""), and never a directory above
// the home directory.
func SearchStopDir(ctx context.Context, cwd string) string {
	stop := (gitutil.Git{}).TopLevelContext(ctx, cwd)
	home, err := ambient.OrOS(nil).UserHomeDir()
	if err != nil || home == "" || !withinDir(cwd, home) {
		return stop
	}
	if stop == "" || (resolvedDir(stop) != resolvedDir(home) && withinDir(home, stop)) {
		return home
	}
	return stop
}

// DiscoverProjectRoot returns the nearest directory at or above start that
// holds a configuration in any layout, stopping at SearchStopDir. A missing
// configuration is reported as ErrNoConfig.
func DiscoverProjectRoot(ctx context.Context, start string) (string, error) {
	return FindProjectRoot(start, SearchStopDir(ctx, start))
}

// SameDir reports whether a and b are the same directory, resolving symlinks.
func SameDir(a, b string) bool { return resolvedDir(a) == resolvedDir(b) }

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
