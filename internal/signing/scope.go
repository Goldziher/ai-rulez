package signing

import (
	"os"
	"path/filepath"
)

// stateScope locates configDir for the rollback state: its absolute path, and
// its slash path relative to the nearest ancestor holding a .git entry (rel is ""
// when there is none). The relative path tells the roots of one monorepo apart
// and survives moving or re-cloning the checkout.
func stateScope(configDir string) (rel, abs string) {
	abs, err := filepath.Abs(configDir)
	if err != nil {
		return "", configDir
	}
	for dir := abs; ; {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			r, rerr := filepath.Rel(dir, abs)
			if rerr != nil {
				return "", abs
			}
			return filepath.ToSlash(r), abs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", abs
		}
		dir = parent
	}
}
