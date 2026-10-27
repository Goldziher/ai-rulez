//go:build windows

package gitutil

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const (
	// replaceAttempts bounds the retries of a rename that another process blocks;
	// with the doubling pause they wait about two seconds in all.
	replaceAttempts  = 12
	replaceFirstWait = time.Millisecond
	replaceMaxWait   = 500 * time.Millisecond
)

// replaceFile renames src over dst. Windows refuses to replace a file another
// process has open (a git reading .git/info/exclude while a sibling root of a
// recursive run rewrites it), so a sharing or access error is retried for a
// short while before it is returned.
func replaceFile(src, dst string) error {
	wait := replaceFirstWait
	var err error
	for range replaceAttempts {
		if err = os.Rename(src, dst); err == nil || !transientReplaceError(err) {
			return err //nolint:wrapcheck // the caller adds the paths
		}
		time.Sleep(wait)
		wait = min(2*wait, replaceMaxWait)
	}
	return err //nolint:wrapcheck // the caller adds the paths
}

func transientReplaceError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
