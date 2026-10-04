package config

import (
	"bytes"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

// DefaultBundleExcludes are the names that are never bundled with a skill or
// command, whatever the project's .gitignore says: virtualenvs, bytecode caches,
// dependency trees and VCS metadata are build artifacts, not skill content.
// Projects add to the list with the top-level bundle_exclude config key.
var DefaultBundleExcludes = []string{".git", ".venv*", "venv", "__pycache__", "*.pyc", "node_modules"}

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
	git, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	// Exit 0: the entry file is ignored, so the whole item is; 128: not a repo.
	//nolint:gosec // git is resolved by LookPath; root and marker are paths of the project being generated.
	if err := exec.Command(git, "-C", root, "check-ignore", "-q", "--no-index", "--", marker).Run(); err == nil {
		return nil
	}
	//nolint:gosec // same arguments as above, no shell involved.
	out, err := exec.Command(git, "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".").Output()
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
	for _, pattern := range f.patterns {
		pattern = strings.Trim(pattern, "/")
		if pattern == "" {
			continue
		}
		if !strings.Contains(pattern, "/") {
			for _, seg := range segments {
				if globMatch(pattern, seg) {
					return true
				}
			}
			continue
		}
		for i := range segments {
			if globMatch(pattern, strings.Join(segments[:i+1], "/")) {
				return true
			}
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
