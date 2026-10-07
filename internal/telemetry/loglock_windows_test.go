package telemetry

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockExclusive holds the lock `usage prune` takes on f.
func lockExclusive(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, new(windows.Overlapped))
}
