package usage

import (
	"errors"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
)

// ErrLogLocked means another process held the usage log's lock for the whole wait.
var ErrLogLocked = errors.New("usage log is locked by another ai-rulez process")

// How long a prune waits for the appenders to finish, and an appender for a prune. An appender is a hook
// that must not stall its harness: past the wait it appends without the lock (the narrow window the prune
// still re-reads for), so the lock only ever makes a lost line less likely, never a hook slower than this.
var (
	pruneLockWait  = 5 * time.Second
	appendLockWait = 2 * time.Second
)

const (
	logLockSuffix = ".lock"
	lockPoll      = 5 * time.Millisecond
)

// lockLog takes the advisory lock of the usage log at path, on a sidecar file because a prune replaces the
// log itself (a lock on its inode would be lost with the rename). An appender takes it shared, so appenders
// never wait for one another; a prune takes it exclusive, which excludes every appender for the read and the
// rewrite. It polls for up to wait (0 tries once) and returns ErrLogLocked when the lock stays taken. The
// release function is safe to call once.
func lockLog(path string, exclusive bool, wait time.Duration) (release func(), err error) {
	f, err := safefs.OpenLockFile(path + logLockSuffix)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "open usage log lock")
	}
	deadline := ambient.Clock(nil).Now().Add(wait)
	for {
		held, lockErr := tryLock(f, exclusive)
		if lockErr != nil {
			_ = f.Close() //nolint:errcheck // already failing
			return nil, oops.With("path", path).Wrapf(lockErr, "lock usage log")
		}
		if held {
			return func() {
				unlock(f)
				_ = f.Close() //nolint:errcheck // nothing to do on failure
			}, nil
		}
		if !ambient.Clock(nil).Now().Before(deadline) {
			_ = f.Close() //nolint:errcheck // not held
			return nil, oops.With("path", path).Hint("Wait for the other command to finish").Wrap(ErrLogLocked)
		}
		time.Sleep(lockPoll)
	}
}
