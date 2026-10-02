//go:build !windows

package config

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/samber/oops"
)

// lockLocalConfig takes an exclusive advisory lock on path (created owner-only)
// and returns the function that releases it. It polls without blocking and gives
// up after localLockTimeout instead of hanging behind a stuck editor.
func lockLocalConfig(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lock file beside the config
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "open local config lock")
	}
	deadline := time.Now().Add(localLockTimeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) //nolint:gosec // fd fits in int
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = f.Close() //nolint:errcheck // already failing
			return nil, oops.With("path", path).Wrapf(err, "lock local config")
		}
		if time.Now().After(deadline) {
			_ = f.Close() //nolint:errcheck // already failing
			return nil, oops.
				With("path", path).
				Hint("Wait for the other command to finish, or remove a stale lock file if no ai-rulez process is running").
				Errorf("another ai-rulez process is editing %s", filepath.Dir(path))
		}
		time.Sleep(localLockPoll)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck,gosec // released on close anyway
		_ = f.Close()                                   //nolint:errcheck // nothing to do on failure
	}, nil
}
