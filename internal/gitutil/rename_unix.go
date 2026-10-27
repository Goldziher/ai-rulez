//go:build !windows

package gitutil

import "os"

// replaceFile renames src over dst; POSIX rename replaces a file other
// processes have open.
func replaceFile(src, dst string) error {
	return os.Rename(src, dst) //nolint:wrapcheck // the caller adds the paths
}
