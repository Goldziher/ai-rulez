package repoindex

import (
	"path"
	"strings"

	"github.com/samber/oops"
)

// maxPathBytes bounds a path argument.
const maxPathBytes = 4096

// CleanPath validates an untrusted project-relative path and returns it cleaned
// and slash separated ("." for the root). It is lexical: the path must be
// relative, free of control characters and backslashes, and must not climb out of
// the project with "..". Nothing here touches the file system, so a path that
// does not exist yet is fine.
func CleanPath(p string) (string, error) {
	switch {
	case p == "":
		return "", oops.Errorf("the path is empty")
	case len(p) > maxPathBytes:
		return "", oops.Errorf("the path is longer than %d bytes", maxPathBytes)
	case strings.ContainsAny(p, "\\") || hasControl(p):
		return "", oops.Errorf("the path %q has a backslash or a control character", p)
	case strings.HasPrefix(p, "/"), hasDrive(p):
		return "", oops.Errorf("the path %q must be relative to the project root", p)
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", oops.Errorf("the path %q leaves the project", p)
	}
	return clean, nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// hasDrive reports a Windows drive prefix such as "C:".
func hasDrive(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0] | 0x20
	return c >= 'a' && c <= 'z'
}

// NamespacedName is the name an item gets when another item already has its
// own: its path from the project root, with the separators turned into dots and
// the leading dot of each segment dropped (pkg/.claude/skills/baz becomes
// pkg.claude.skills.baz).
func NamespacedName(rel string) string {
	segments := strings.Split(rel, "/")
	for i, seg := range segments {
		segments[i] = strings.TrimLeft(seg, ".")
	}
	return strings.Join(segments, ".")
}

// depth is the number of directories above a project-relative path.
func depth(rel string) int {
	if rel == RootScope {
		return 0
	}
	return strings.Count(rel, "/") + 1
}

// parentOf is the directory containing a project-relative path ("." at the top).
func parentOf(rel string) string { return path.Dir(rel) }
