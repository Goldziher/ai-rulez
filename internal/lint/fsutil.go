package lint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// maxReadBytes bounds the files the config-shape checks read.
const maxReadBytes = 2 << 20

var errTooLarge = errors.New("file too large")

// realCase returns rel (a slash path below top) with every component spelled as
// the file system stores it. On a case-sensitive file system rel is returned
// unchanged; on a case-insensitive one (the default on macOS and Windows) a
// path typed as agents.md is the file AGENTS.md, and the two spellings must not
// be told apart by name, classified differently or linted twice.
func realCase(top, rel string) string {
	if rel == "" || rel == "." || !foldsCase(top, rel) {
		return rel
	}
	parts := strings.Split(rel, "/")
	cur := top
	for i, p := range parts {
		parts[i] = storedName(cur, p)
		cur = filepath.Join(cur, parts[i])
	}
	return strings.Join(parts, "/")
}

// foldsCase reports whether the file system of top treats the last component of
// rel and its case-swapped spelling as one file.
func foldsCase(top, rel string) bool {
	abs := filepath.Join(top, filepath.FromSlash(rel))
	dir, base := filepath.Split(abs)
	swapped := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsUpper(r):
			return unicode.ToLower(r)
		case unicode.IsLower(r):
			return unicode.ToUpper(r)
		}
		return r
	}, base)
	if swapped == base {
		return false
	}
	a, errA := os.Stat(abs)
	b, errB := os.Stat(filepath.Join(dir, swapped))
	return errA == nil && errB == nil && os.SameFile(a, b)
}

// storedName is the spelling of name that the directory dir stores: name itself
// when an entry has it, else the entry that names the same file.
func storedName(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return name
	}
	want, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		return name
	}
	found := name
	for _, e := range entries {
		if e.Name() == name {
			return name
		}
		if got, serr := os.Stat(filepath.Join(dir, e.Name())); serr == nil && strings.EqualFold(e.Name(), name) && os.SameFile(want, got) {
			found = e.Name()
		}
	}
	return found
}

func readSmallFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers only test for failure
	}
	if info.Size() > maxReadBytes {
		return nil, errTooLarge
	}
	return os.ReadFile(path) //nolint:wrapcheck // callers only test for failure
}
