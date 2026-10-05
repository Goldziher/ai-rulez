package config

import (
	"bytes"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// gitDirName is the name of a git metadata directory.
const gitDirName = ".git"

// DefaultBundleExcludes are the names that are never bundled with a skill or
// command, whatever the project's .gitignore says: virtualenvs, bytecode caches,
// dependency trees and VCS metadata are build artifacts, not skill content.
// Projects add to the list with the top-level bundle_exclude config key; an entry
// prefixed with "!" re-includes a path a default excluded (e.g. "!references/venv").
var DefaultBundleExcludes = []string{gitDirName, ".venv*", "venv", "__pycache__", "*.pyc", "node_modules"}

// bundleFilter decides which files below a skill or command root are bundled.
type bundleFilter struct {
	patterns []string
	// visible is the set of root-relative slash paths git considers part of the
	// project (tracked, or untracked and not ignored). Nil disables the check:
	// the root is not in a git work tree, git is missing, or the root itself is
	// ignored (an author who gitignores the whole content tree still wants it).
	visible map[string]bool
}

// newBundleFilter builds the filter for one skill or command root. marker is the
// item's entry file (SKILL.md, COMMAND.md).
func newBundleFilter(root, marker string, extra []string) *bundleFilter {
	patterns := make([]string, 0, len(DefaultBundleExcludes)+len(extra))
	patterns = append(patterns, DefaultBundleExcludes...)
	for _, p := range extra {
		if p = strings.TrimSpace(p); p != "" {
			patterns = append(patterns, p)
		}
	}
	return &bundleFilter{patterns: patterns, visible: gitVisibleFiles(root, marker)}
}

// gitVisibleFiles lists the files under root that git would not ignore, or nil
// when that cannot be determined.
func gitVisibleFiles(root, marker string) map[string]bool {
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	// Exit 0: the entry file is ignored, so the whole item is; 128: not a repo.
	if err := gitutil.CommandNoContext(root, "check-ignore", "-q", "--no-index", "--", marker).Run(); err == nil {
		return nil
	}
	out, err := gitutil.CommandNoContext(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".").Output()
	if err != nil {
		logger.Debug("git ls-files unavailable, bundling without .gitignore", "path", root, "error", err)
		return nil
	}
	visible := make(map[string]bool)
	for _, name := range bytes.Split(out, []byte{0}) {
		if len(name) > 0 {
			visible[filepath.ToSlash(string(name))] = true
		}
	}
	if len(visible) == 0 {
		return nil
	}
	return visible
}

// excluded reports whether the root-relative slash path rel (a file, or a
// directory when isDir) is left out of the bundle by a pattern. A pattern
// without a slash matches any path segment; one with a slash matches the whole
// relative path or any leading directory of it.
func (f *bundleFilter) excluded(rel string) bool {
	segments := strings.Split(rel, "/")
	skip := false
	matched := ""
	// Later patterns win, so a "!pattern" entry re-includes what a default or an
	// earlier pattern excluded.
	for _, pattern := range f.patterns {
		negate := strings.HasPrefix(pattern, "!")
		raw := strings.TrimPrefix(pattern, "!")
		if matchesPattern(raw, segments) {
			skip = !negate
			matched = pattern
		}
	}
	if skip {
		logger.Debug("Skipped bundle path matching an exclude pattern", "path", rel, "pattern", matched)
	}
	return skip
}

// matchesPattern reports whether pattern matches a segment (slash-less pattern)
// or the whole path / a leading directory of it (pattern with a slash).
func matchesPattern(pattern string, segments []string) bool {
	pattern = strings.Trim(pattern, "/")
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		return slices.ContainsFunc(segments, func(seg string) bool { return globMatch(pattern, seg) })
	}
	for i := range segments {
		if globMatch(pattern, strings.Join(segments[:i+1], "/")) {
			return true
		}
	}
	return false
}

// keepFile reports whether the file at root-relative slash path rel is bundled.
func (f *bundleFilter) keepFile(rel string) bool {
	if f.excluded(rel) {
		return false
	}
	return f.visible == nil || f.visible[rel]
}

// rel is the root-relative slash path of p.
func (f *bundleFilter) rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

// globMatch reports whether name matches the glob; a malformed pattern matches nothing.
func globMatch(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}
