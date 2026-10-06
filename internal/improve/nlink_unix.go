//go:build unix

package improve

import (
	"io/fs"
	"syscall"
)

// hardLinked reports a regular file with more than one name, which an optimizer
// could use to alias a file outside its workspace.
func hardLinked(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}
