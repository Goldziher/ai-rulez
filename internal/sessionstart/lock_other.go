//go:build !unix

package sessionstart

import "os"

// privateDir reports whether dir is a real directory; the temp directory of a
// Windows user is already private to that user.
func privateDir(dir string, _ int) bool {
	info, err := os.Lstat(dir)
	return err == nil && info.IsDir()
}
