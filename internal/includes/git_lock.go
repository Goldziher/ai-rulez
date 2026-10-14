package includes

import (
	"context"
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
			_ = file.Close()
			return nil, oops.Wrapf(err, "wait for include cache lock")
		}
		acquired, err := tryFetchLock(file)
		if err != nil {
			_ = file.Close()
			return nil, oops.Wrapf(err, "lock include cache")
		}
		if acquired {
			return func() { _ = releaseFetchLock(file); _ = file.Close() }, nil
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}
