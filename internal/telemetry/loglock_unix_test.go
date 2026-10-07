//go:build !windows

package telemetry

import (
	"os"
	"syscall"
)

// lockExclusive holds the lock `usage prune` takes on f.
func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) //nolint:gosec // fd fits in int
}
