package importer

import (
	"io/fs"
	"path"
	"strings"
)

// SplitSources reads the --from value of `init`: a comma-separated mix of
// importer names (native, rulesync, auto, ...) and the project paths older
// versions took (.claude, .cursor/rules, CLAUDE.md). Paths select what the native
// importer reads; a list that names native itself leaves it unrestricted.
func SplitSources(spec string) (from, nativePaths []string) {
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if _, ok := Lookup(tok); ok || tok == autoFrom {
			from = append(from, tok)
			continue
		}
		nativePaths = append(nativePaths, tok)
	}
	for _, f := range from {
		if f == nativeName {
			return from, nil
		}
	}
	if len(nativePaths) > 0 {
		from = append(from, nativeName)
	}
	return from, nativePaths
}

// restrictedFS shows only the paths a caller named, and the directories leading
// to them: `init --from .claude,CLAUDE.md` reads exactly those, not everything
// the native importer knows. Symlink checks keep working because Lstat and
// ReadLink are passed through (the reader refuses links through them).
type restrictedFS struct {
	inner fs.FS
	allow []string
}

// restrict returns fsys limited to the given slash paths (files or directories).
// An empty list leaves it unrestricted.
func restrict(fsys fs.FS, paths []string) fs.FS {
	if len(paths) == 0 {
		return fsys
	}
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		if c := path.Clean(strings.TrimSpace(p)); fs.ValidPath(c) && c != "." {
			clean = append(clean, c)
		}
	}
	return restrictedFS{inner: fsys, allow: clean}
}

// visible reports whether name is an allowed path, below one, or a directory on
// the way to one.
func (r restrictedFS) visible(name string) bool {
	if name == "." {
		return true
	}
	for _, a := range r.allow {
		if name == a || strings.HasPrefix(name, a+"/") || strings.HasPrefix(a, name+"/") {
			return true
		}
	}
	return false
}

func (r restrictedFS) Open(name string) (fs.File, error) {
	if !r.visible(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return r.inner.Open(name) //nolint:wrapcheck // a pass-through
}

func (r restrictedFS) Stat(name string) (fs.FileInfo, error) {
	if !r.visible(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return fs.Stat(r.inner, name) //nolint:wrapcheck // a pass-through
}

func (r restrictedFS) Lstat(name string) (fs.FileInfo, error) {
	if !r.visible(name) {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrNotExist}
	}
	return fs.Lstat(r.inner, name) //nolint:wrapcheck // a pass-through
}

func (r restrictedFS) ReadLink(name string) (string, error) {
	if !r.visible(name) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrNotExist}
	}
	return fs.ReadLink(r.inner, name) //nolint:wrapcheck // a pass-through
}

func (r restrictedFS) ReadFile(name string) ([]byte, error) {
	if !r.visible(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	return fs.ReadFile(r.inner, name) //nolint:wrapcheck // a pass-through
}

func (r restrictedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !r.visible(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries, err := fs.ReadDir(r.inner, name)
	if err != nil {
		return nil, err //nolint:wrapcheck // a pass-through
	}
	out := entries[:0]
	for _, e := range entries {
		if r.visible(path.Join(name, e.Name())) {
			out = append(out, e)
		}
	}
	return out, nil
}
