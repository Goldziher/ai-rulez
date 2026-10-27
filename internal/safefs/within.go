package safefs

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RelEscapes reports whether rel, a path produced by filepath.Rel, leaves the
// base it was computed against: it is ".." or starts with "../" (or "..\" on
// Windows; a slash-form path is recognized on every platform). A name that
// merely begins with two dots ("..foo", "...") is an ordinary child.
func RelEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Within reports whether target is root itself or lies below it, comparing the
// paths lexically (no symlink resolution). Both must be of the same kind
// (both absolute or both relative to the same directory); a pair filepath.Rel
// cannot relate, such as different volumes, is not within.
func Within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && !RelEscapes(rel)
}

// SubPath joins a slash-form sub-path (an include's `path`) onto root. An empty
// sub-path or "/" is root itself. A sub-path that climbs out of root ("..",
// "a/../..") is refused; a leading slash is trimmed, as the git include does, so
// the result always stays below root lexically.
func SubPath(root, sub string) (string, error) {
	sub = strings.Trim(filepath.ToSlash(sub), "/")
	clean := filepath.Clean(filepath.FromSlash(sub))
	if sub == "" || clean == "." {
		return root, nil
	}
	if RelEscapes(clean) || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return "", fmt.Errorf("path %q leaves the include source", sub)
	}
	return filepath.Join(root, clean), nil
}
