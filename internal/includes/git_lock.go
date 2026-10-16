package includes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"
)

const fetchLockPoll = 20 * time.Millisecond

// ~keep The lock lives beside the checkout because refreshing deletes the checkout itself.
func lockFetchCache(ctx context.Context, cacheDir string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(cacheDir), cacheDirMode); err != nil {
		return nil, oops.Wrapf(err, "create include cache parent")
	}
	file, err := openFetchLock(cacheDir + ".lock")
	if err != nil {
		return nil, oops.Wrapf(err, "open include cache lock")
	}
	ticker := time.NewTicker(fetchLockPoll)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, oops.Wrapf(errors.Join(err, file.Close()), "wait for include cache lock")
		}
		acquired, err := tryFetchLock(file)
		if err != nil {
			return nil, oops.Wrapf(errors.Join(err, file.Close()), "lock include cache")
		}
		if acquired {
			return func() { _ = errors.Join(releaseFetchLock(file), file.Close()) }, nil //nolint:errcheck // a release has nowhere to report to
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}
