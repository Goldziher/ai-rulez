// Package workspace is the read view of a project tree that the engine loads
// configuration and content through (issue #229, step S3).
//
// A Workspace is an io/fs file system plus Lstat and ReadLink, so symlink policy
// can be decided by the caller, and a Root used for display and for translating
// the absolute paths the engine keeps in its data model. Library code reads
// through a Workspace instead of the process working directory; the OS
// implementation is the only one that touches the disk, and it is built by the
// CLI boundary (or by OS/Discover) from an explicit directory.
//
// Names passed to a Workspace are slash-separated, relative to the root and valid
// under fs.ValidPath: "." is the root, ".." and absolute names are rejected.
package workspace

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing/fstest"
)

// ErrOutside reports a path that is not below the workspace root, or a symlink
// whose target resolves outside it.
var ErrOutside = errors.New("path is outside the workspace")

// OutsideError is an ErrOutside that names the path that left the workspace: the
// target of an escaping symlink, or the absolute path that is not below the root.
type OutsideError struct{ Target string }

func (e *OutsideError) Error() string { return "path " + e.Target + " is outside the workspace" }

// Is makes errors.Is(err, ErrOutside) hold.
func (e *OutsideError) Is(target error) bool { return target == ErrOutside }

// Workspace is a read-only view of a project tree.
type Workspace interface {
	fs.FS
	// ReadFile reads a file, following symlinks (fs.ReadFileFS).
	ReadFile(name string) ([]byte, error)
	// ReadDir lists a directory sorted by name (fs.ReadDirFS).
	ReadDir(name string) ([]fs.DirEntry, error)
	// Stat describes name, following a final symlink (fs.StatFS).
	Stat(name string) (fs.FileInfo, error)
	// Lstat describes name without following a final symlink.
	Lstat(name string) (fs.FileInfo, error)
	// ReadLink returns the target of a symlink as written (fs.ReadLinkFS).
	ReadLink(name string) (string, error)
	// Root is the absolute directory the workspace is rooted at. It is used to
	// display paths and to translate absolute paths into names; implementations
	// that are not backed by a directory return a virtual absolute root.
	Root() string
}

// fsWorkspace adapts any fs.FS that supports symlink reads to Workspace.
type fsWorkspace struct {
	fsys fs.FS
	root string
	// aliases are other absolute spellings of root (the resolved path of a
	// symlinked root), so a path built from either maps to the same names.
	aliases []string
	// reach is true when the workspace may open other directories of the same
	// file system (see Sibling); false for in-memory and snapshot workspaces.
	reach bool
}

// FromFS wraps fsys, rooted at the absolute display directory root.
func FromFS(fsys fs.FS, root string) Workspace {
	return &fsWorkspace{fsys: fsys, root: filepath.Clean(root)}
}

//nolint:wrapcheck // the methods below return fs errors as the file system reports them
func (w *fsWorkspace) Open(name string) (fs.File, error) { return w.fsys.Open(name) }

//nolint:wrapcheck // as above
func (w *fsWorkspace) ReadFile(name string) ([]byte, error) { return fs.ReadFile(w.fsys, name) }

//nolint:wrapcheck // as above
func (w *fsWorkspace) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(w.fsys, name) }

//nolint:wrapcheck // as above
func (w *fsWorkspace) Stat(name string) (fs.FileInfo, error) { return fs.Stat(w.fsys, name) }

//nolint:wrapcheck // as above
func (w *fsWorkspace) Lstat(name string) (fs.FileInfo, error) { return fs.Lstat(w.fsys, name) }

//nolint:wrapcheck // as above
func (w *fsWorkspace) ReadLink(name string) (string, error) { return fs.ReadLink(w.fsys, name) }

func (w *fsWorkspace) Root() string { return w.root }

func (w *fsWorkspace) rootAliases() []string { return append([]string{w.root}, w.aliases...) }

type aliased interface{ rootAliases() []string }

// roots returns every absolute spelling of ws's root.
func roots(ws Workspace) []string {
	if a, ok := ws.(aliased); ok {
		return a.rootAliases()
	}
	return []string{ws.Root()}
}

// Mem is a writable in-memory workspace for tests and for callers that receive
// files over an API. Its root is a virtual absolute directory.
type Mem struct {
	*fsWorkspace
	files fstest.MapFS
}

// NewMem returns an empty in-memory workspace whose root is root (an absolute
// path such as "/work"; "" means "/work").
func NewMem(root string) *Mem {
	if root == "" {
		root = string(filepath.Separator) + "work"
	}
	files := fstest.MapFS{}
	return &Mem{fsWorkspace: &fsWorkspace{fsys: files, root: filepath.Clean(root)}, files: files}
}

// Set creates or replaces a regular file; parent directories exist implicitly.
func (m *Mem) Set(name, content string, mode fs.FileMode) {
	m.files[name] = &fstest.MapFile{Data: []byte(content), Mode: mode.Perm()}
}

// Symlink creates a symlink at name pointing at target (as written).
func (m *Mem) Symlink(name, target string) {
	m.files[name] = &fstest.MapFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777}
}

// Mkdir creates an (empty) directory.
func (m *Mem) Mkdir(name string) {
	m.files[name] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
}

// Remove deletes a file, symlink or directory entry.
func (m *Mem) Remove(name string) { delete(m.files, name) }

// Rel returns the slash-separated name of abs below ws's root. It fails with
// ErrOutside when abs is not below it.
func Rel(ws Workspace, abs string) (string, error) {
	abs = filepath.Clean(abs)
	for _, root := range roots(ws) {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		if rel == "." {
			return ".", nil
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		return filepath.ToSlash(rel), nil
	}
	return "", ErrOutside
}

// Abs returns the absolute path of name below ws's root.
func Abs(ws Workspace, name string) string {
	if name == "." || name == "" {
		return ws.Root()
	}
	return filepath.Join(ws.Root(), filepath.FromSlash(name))
}

// Sibling returns a workspace for dir, a directory outside ws's root, when ws is
// backed by the real file system (an include cache or a user config directory
// next to the project). In-memory and snapshot workspaces see nothing outside
// their root and return ErrOutside.
func Sibling(ws Workspace, dir string) (Workspace, error) {
	if w, ok := ws.(*fsWorkspace); ok && w.reach {
		return OS(dir)
	}
	return nil, &OutsideError{Target: dir}
}
