//go:build !windows

package usage

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes a flock on f without blocking; held is false when another descriptor holds a conflicting one.
func tryLock(f *os.File, exclusive bool) (held bool, err error) {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	err = syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB) //nolint:gosec // fd fits in int
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN):
		return false, nil
	}
	return false, err //nolint:wrapcheck // wrapped by the caller
}

func unlock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck,gosec // released on close anyway
}
