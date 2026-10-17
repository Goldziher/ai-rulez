package sessionstart

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// lockPoll is how often a waiting run looks at a held lock again.
const lockPoll = 100 * time.Millisecond

// lockPath is the lock file of a project, or "" when no safe place for it exists. It
// lives in a per-user private directory of the temp directory, not in the repository,
// so a read-only checkout can still run, nothing is left to commit, and another local
// user cannot pre-create the name (a directory of ours is verified, not trusted).
func lockPath(d Deps, root string) string {
	dir := filepath.Join(d.TempDir, fmt.Sprintf("ai-rulez-session-start-%d", d.UID))
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return ""
	}
	if !privateDir(dir, d.UID) {
		return ""
	}
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(dir, hex.EncodeToString(sum[:6])+".lock")
}

// acquire takes the lock file, waiting for a holder until ctx ends. A lock file older
// than stale belongs to a run that died and is removed. The returned function releases it.
func acquire(ctx context.Context, path string, stale time.Duration, now func() time.Time) (release func(), ok bool) {
	if path == "" {
		return func() {}, true // no safe place for a lock: run unlocked
	}
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // a fixed name in the temp directory
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())     //nolint:errcheck // informational
			f.Close()                               //nolint:errcheck,gosec // best-effort close of the lock file
			return func() { os.Remove(path) }, true //nolint:errcheck,gosec // best-effort release
		}
		if !errors.Is(err, os.ErrExist) {
			// The lock cannot be created (read-only temp directory): run unlocked
			// rather than skip the work; generate's own writes are atomic.
			return func() {}, true
		}
		if info, statErr := os.Stat(path); statErr == nil && now().Sub(info.ModTime()) > stale {
			os.Remove(path) //nolint:errcheck,gosec // best-effort removal of a stale lock; the next attempt retries
			continue
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(lockPoll):
		}
	}
}
