package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Argument names that point into the file system.
const (
	argWorkingDirectory = "working_directory"
	argConfigFile       = "config_file"
	argConfigDir        = "config_dir"
	argBundle           = "bundle"
)

// dirPolicy confines the directories a tool call may read or write. Without it
// any caller (a prompt-injected agent included) could point working_directory
// at /etc or at another project and create, delete or generate files there; the
// CLI works on the process's current directory only.
type dirPolicy struct {
	// root is the directory tool calls are confined to ("" when it is unknown).
	root string
	// anyDir disables the confinement (mcp --allow-any-dir).
	anyDir bool
}

// confine checks a call's working_directory, config_file, config_dir and bundle
// against the root and rewrites each to the absolute, symlink-resolved path that
// was checked: working_directory becomes the root when it is absent and resolves
// against the root when relative; the others resolve against that directory, as
// the handlers do. Handing on the resolved path rather than the one the caller
// gave means a symlink swapped after the check cannot redirect the operation.
func (p dirPolicy) confine(args map[string]any) error {
	if p.anyDir {
		return nil
	}
	if p.root == "" {
		return errors.New("the server has no allowed root; start it with --root or --allow-any-dir")
	}
	base, err := p.check(argWorkingDirectory, stringArg(args, argWorkingDirectory), p.root)
	if err != nil {
		return err
	}
	args[argWorkingDirectory] = base
	for _, name := range []string{argConfigFile, argConfigDir, argBundle} {
		if v := stringArg(args, name); v != "" {
			resolved, err := p.check(name, v, base)
			if err != nil {
				return err
			}
			args[name] = resolved
		}
	}
	return nil
}

// check verifies that dir (joined to base when relative) is inside the root and
// returns its absolute form with symlinks resolved, which is what was judged.
// An empty dir means base.
func (p dirPolicy) check(arg, dir, base string) (string, error) {
	target := base
	if dir != "" {
		target = dir
		if !filepath.IsAbs(target) {
			target = filepath.Join(base, target)
		}
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", arg, dir, err)
	}
	resolved := resolveExisting(abs)
	if !safefs.Within(resolveExisting(p.root), resolved) {
		return "", fmt.Errorf("%s %q is outside the directory this server is allowed to use (%s); start the server in the project, or pass --root or --allow-any-dir", arg, dir, p.root)
	}
	return resolved, nil
}

func stringArg(args map[string]any, name string) string {
	v, _ := args[name].(string) //nolint:errcheck // a missing or non-string argument is the empty default
	return v
}

// resolveExisting resolves symlinks in the longest existing prefix of path, so
// a link that leaves the root cannot be used to escape it, and a path that does
// not exist yet (init_project) is judged by its nearest existing parent.
func resolveExisting(path string) string {
	path = filepath.Clean(path)
	var rest []string
	for current := path; ; {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, rest[i])
			}
			return resolved
		}
		if !errors.Is(err, os.ErrNotExist) {
			return path
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		rest = append(rest, filepath.Base(current))
		current = parent
	}
}
