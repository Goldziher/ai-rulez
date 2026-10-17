//go:build unix

package sessionstart

import (
	"os"
	"syscall"
)

// privateDir reports whether dir is a real directory (not a symlink) owned by uid
// that no one else can write to; a directory of ours with looser bits is tightened.
func privateDir(dir string, uid int) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != uid {
		return false
	}
	if info.Mode().Perm()&0o077 != 0 {
		return os.Chmod(dir, 0o700) == nil //nolint:gosec // the directory is ours; 0700 is the point
	}
	return true
}
