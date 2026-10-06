//go:build windows

package usage

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock takes a LockFileEx lock on the first byte of f without blocking; held is false when another
// handle holds a conflicting one.
func tryLock(f *os.File, exclusive bool) (held bool, err error) {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	err = windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, new(windows.Overlapped))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return false, nil
	}
	return false, err //nolint:wrapcheck // wrapped by the caller
}

func unlock(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped)) //nolint:errcheck // released on close anyway
}
