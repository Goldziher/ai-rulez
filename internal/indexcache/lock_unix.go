//go:build !windows

package indexcache

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive flock on f without blocking; held is false when
// another descriptor holds it.
func tryLock(f *os.File) (held bool, err error) {
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) //nolint:gosec // fd fits in int
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
