package config

import (
	"io/fs"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// existingView is the view that can read the existing file path: the workspace the
// project was loaded from, or (for a path outside it, in a real-directory
// workspace) the real directory. A project in memory or in a commit sees nothing
// outside its root, so an absolute path elsewhere reads as absent.
func (c *Config) existingView(path string) workspace.View {
	return c.ViewFor(filepath.Dir(path))
}

func absent(op, path string) error {
	return &fs.PathError{Op: op, Path: path, Err: fs.ErrNotExist}
}

// ReadExisting reads a file that may already exist where the project's outputs go
// (a settings document, the previous manifest, a hand-edited output) from the
// project's workspace, so planning against a workspace in memory or in a commit
// sees that workspace's outputs, not the disk of this process.
func (c *Config) ReadExisting(path string) ([]byte, error) {
	v := c.existingView(path)
	if v.W == nil {
		return nil, absent("open", path)
	}
	return v.ReadFile(path) //nolint:wrapcheck // a PathError naming path, like os.ReadFile
}

// StatExisting is os.Stat of an existing output, read from the project's workspace.
func (c *Config) StatExisting(path string) (fs.FileInfo, error) {
	v := c.existingView(path)
	if v.W == nil {
		return nil, absent("stat", path)
	}
	return v.Stat(path) //nolint:wrapcheck // a PathError naming path, like os.Stat
}

// LstatExisting is os.Lstat of an existing output, read from the project's workspace.
func (c *Config) LstatExisting(path string) (fs.FileInfo, error) {
	v := c.existingView(path)
	if v.W == nil {
		return nil, absent("lstat", path)
	}
	return v.Lstat(path) //nolint:wrapcheck // a PathError naming path, like os.Lstat
}

// OpenExisting is os.Open of an existing output, read from the project's workspace.
func (c *Config) OpenExisting(path string) (fs.File, error) {
	v := c.existingView(path)
	if v.W == nil {
		return nil, absent("open", path)
	}
	return v.Open(path) //nolint:wrapcheck // a PathError naming path, like os.Open
}
