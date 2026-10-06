package workspace

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

// maxLinks bounds symlink resolution, like the operating system's limit.
const maxLinks = 255

// ErrNoWorkspace is returned by a zero View.
var ErrNoWorkspace = errors.New("no workspace")

// View reads a Workspace through the absolute paths the engine keeps in its data
// model (Config.BaseDir, ContentFile.Path, ...). Errors name the absolute path,
// so messages read as they did when the files were read from the disk directly.
// A path below the workspace root maps to a name; any other path fails with
// ErrOutside wrapped in a *fs.PathError. The zero View has no workspace and
// fails every call with ErrNoWorkspace.
type View struct {
	W Workspace
	// cwd makes a relative path mean the process working directory, like the os
	// package does. Only OSView sets it: it is the view of the CLI boundary.
	cwd bool
}

// NewView returns a View of w.
func NewView(w Workspace) View { return View{W: w} }

// OSView returns a view of the real directory dir, or the zero View when dir
// cannot be resolved. It is for entry points that name their own root.
func OSView(dir string) View {
	ws, err := OS(dir)
	if err != nil {
		return View{}
	}
	return View{W: ws, cwd: true}
}

// For returns v when dir is inside v's workspace; otherwise a view of dir itself
// when the workspace is backed by the real file system (included content in a
// cache, a user config directory), and the zero View when it is not.
func (v View) For(dir string) View {
	if v.W == nil {
		return OSView(dir)
	}
	if _, err := Rel(v.W, dir); err == nil {
		return v
	}
	ws, err := Sibling(v.W, dir)
	if err != nil {
		return View{}
	}
	return View{W: ws}
}

// Root is the workspace root, "" for the zero View.
func (v View) Root() string {
	if v.W == nil {
		return ""
	}
	return v.W.Root()
}

func (v View) name(op, abs string) (string, error) {
	if v.W == nil {
		return "", &fs.PathError{Op: op, Path: abs, Err: ErrNoWorkspace}
	}
	target := abs
	if v.cwd && !filepath.IsAbs(target) {
		resolved, err := filepath.Abs(target)
		if err != nil {
			return "", &fs.PathError{Op: op, Path: abs, Err: err}
		}
		target = resolved
	}
	rel, err := Rel(v.W, target)
	if err != nil {
		return "", &fs.PathError{Op: op, Path: abs, Err: &OutsideError{Target: target}}
	}
	return rel, nil
}

// withPath rewrites a PathError's path to abs.
func withPath(err error, abs string) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return &fs.PathError{Op: pe.Op, Path: abs, Err: pe.Err}
	}
	return err
}

// Stat is os.Stat.
func (v View) Stat(abs string) (fs.FileInfo, error) {
	name, err := v.name("stat", abs)
	if err != nil {
		return nil, err
	}
	info, err := v.W.Stat(name)
	return info, withPath(err, abs)
}

// Lstat is os.Lstat.
func (v View) Lstat(abs string) (fs.FileInfo, error) {
	name, err := v.name("lstat", abs)
	if err != nil {
		return nil, err
	}
	info, err := v.W.Lstat(name)
	return info, withPath(err, abs)
}

// ReadDir is os.ReadDir.
func (v View) ReadDir(abs string) ([]fs.DirEntry, error) {
	name, err := v.name("open", abs)
	if err != nil {
		return nil, err
	}
	entries, err := v.W.ReadDir(name)
	return entries, withPath(err, abs)
}

// ReadFile is os.ReadFile.
func (v View) ReadFile(abs string) ([]byte, error) {
	name, err := v.name("open", abs)
	if err != nil {
		return nil, err
	}
	data, err := v.W.ReadFile(name)
	return data, withPath(err, abs)
}

// Open is os.Open.
func (v View) Open(abs string) (fs.File, error) {
	name, err := v.name("open", abs)
	if err != nil {
		return nil, err
	}
	f, err := v.W.Open(name)
	return f, withPath(err, abs)
}

// Exists reports whether abs exists (following a final symlink).
func (v View) Exists(abs string) bool {
	_, err := v.Stat(abs)
	return err == nil
}

// IsRegularFile reports whether abs is an existing non-directory.
func (v View) IsRegularFile(abs string) bool {
	info, err := v.Stat(abs)
	return err == nil && !info.IsDir()
}

// EvalSymlinks is filepath.EvalSymlinks inside the workspace: the absolute path
// with every symlink resolved. A link that resolves outside the workspace root
// fails with ErrOutside.
func (v View) EvalSymlinks(abs string) (string, error) {
	name, err := v.name("evalsymlinks", abs)
	if err != nil {
		return "", err
	}
	resolved, err := Resolve(v.W, name)
	if err != nil {
		return "", withPath(err, abs)
	}
	return Abs(v.W, resolved), nil
}

// Resolve resolves every symlink in name and returns the resulting name. A link
// whose target leaves the workspace root fails with ErrOutside; a missing
// component fails with the file system's error.
func Resolve(ws Workspace, name string) (string, error) {
	queue := splitName(name)
	var done []string
	for links := 0; len(queue) > 0; {
		part := queue[0]
		queue = queue[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			if len(done) == 0 {
				return "", &OutsideError{Target: filepath.Join(append([]string{ws.Root(), ".."}, queue...)...)}
			}
			done = done[:len(done)-1]
			continue
		}
		cand := strings.Join(append(append([]string(nil), done...), part), "/")
		info, err := ws.Lstat(cand)
		if err != nil {
			return "", err //nolint:wrapcheck // the caller reports the file system's error
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			done = append(done, part)
			continue
		}
		if links++; links > maxLinks {
			return "", &fs.PathError{Op: "evalsymlinks", Path: name, Err: errors.New("too many links")}
		}
		target, err := ws.ReadLink(cand)
		if err != nil {
			return "", err //nolint:wrapcheck // as above
		}
		if filepath.IsAbs(target) {
			rel, err := Rel(ws, target)
			if err != nil {
				return "", &OutsideError{Target: filepath.Join(append([]string{target}, queue...)...)}
			}
			done = nil
			queue = append(splitName(rel), queue...)
			continue
		}
		queue = append(splitName(filepath.ToSlash(target)), queue...)
	}
	if len(done) == 0 {
		return ".", nil
	}
	return strings.Join(done, "/"), nil
}

func splitName(name string) []string { return strings.Split(name, "/") }
