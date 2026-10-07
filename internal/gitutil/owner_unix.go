//go:build !windows

package gitutil

import (
	"io/fs"
	"os"
	"syscall"
)

func ownedByCurrentUser(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return int(st.Uid) == os.Getuid()
}

// writableByOthers reports group or world write permission.
func writableByOthers(info fs.FileInfo) bool { return info.Mode().Perm()&0o022 != 0 }
