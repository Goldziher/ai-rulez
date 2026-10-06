package workspace

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

// vcsDirName marks the top of a version-controlled work tree.
const vcsDirName = ".git"

// OS returns a workspace over the directory root of the real file system. A
// relative root is made absolute here, which is the one place a relative path may
// meet the process working directory. Symlinks are followed on read; deciding
// whether one may be followed is the caller's policy (see Resolve).
func OS(root string) (Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, oops.With("path", root).Wrapf(err, "resolve workspace root")
	}
	w := &fsWorkspace{fsys: os.DirFS(abs), root: abs, reach: true}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != abs {
		w.aliases = []string{resolved}
	}
	return w, nil
}

// Around returns a workspace for the project that contains dir: rooted at the
// nearest ancestor holding a .git entry, or at dir itself when there is none. A
// content symlink may point anywhere in that tree, so a project nested in a
// larger repository can link to its siblings.
func Around(dir string) (Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, oops.With("path", dir).Wrapf(err, "resolve workspace root")
	}
	return OS(vcsTop(abs))
}

// vcsTop walks up from abs looking for a .git entry; it returns abs when none exists.
func vcsTop(abs string) string {
	for cur := abs; ; {
		if _, err := os.Lstat(filepath.Join(cur, vcsDirName)); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		cur = parent
	}
}
