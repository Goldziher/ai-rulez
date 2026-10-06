package skillsource

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/samber/oops"
)

// Clone size limit of a git skill source. Everything a source names is
// untrusted: a repository with a huge tree or huge blobs must not fill the cache
// disk or stall start-up before the limits on what is loaded even run.
const (
	// DefaultMaxCloneBytes is the clone size limit when nothing else sets one.
	DefaultMaxCloneBytes int64 = 256 << 20
	// EnvMaxCloneBytes is the environment variable that sets the global limit
	// (bytes); [[skill_sources]] max_clone_bytes overrides it per source.
	EnvMaxCloneBytes = "AI_RULEZ_MAX_CLONE_BYTES"

	cloneWatchInterval = 100 * time.Millisecond
)

// ErrCloneTooLarge is wrapped by the error of a git source whose clone, with its
// checkout, grows past the size limit. Nothing of such a clone is kept.
var ErrCloneTooLarge = errors.New("skill source clone exceeds the size limit")

// maxCloneBytes picks the limit of a source: its own max_clone_bytes, then the
// global one passed in, then AI_RULEZ_MAX_CLONE_BYTES, then the default.
func (s Spec) maxCloneBytes(global int64) int64 {
	switch {
	case s.MaxCloneBytes > 0:
		return s.MaxCloneBytes
	case global > 0:
		return global
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(EnvMaxCloneBytes)), 10, 64); err == nil && v > 0 {
		return v
	}
	return DefaultMaxCloneBytes
}

// cloneRequest describes one fetch of a commit.
type cloneRequest struct {
	name, url, ref, kind, commit string
	// path is the source path; a non-empty path fetches only that subtree.
	path string
	// maxBytes bounds the clone (git metadata plus checkout).
	maxBytes int64
}

// partialFilter is the object filter of a fetch: trees only when a path is
// checked out sparsely (its blobs arrive on demand), and no blob over the limit
// otherwise. A server without filter support ignores it; the size watch below
// still holds.
func (r cloneRequest) partialFilter() string {
	if cleanSrcPath(r.path) != "" {
		return "--filter=blob:none"
	}
	return "--filter=blob:limit=" + strconv.FormatInt(r.maxBytes, 10)
}

func (r cloneRequest) tooLarge(grown int64) error {
	return oops.With("url", includes.RedactURL(r.url)).With("limit_bytes", r.maxBytes).
		Hint("Set `path` to the directory that holds the skills, or raise max_clone_bytes on the source (global: "+EnvMaxCloneBytes+")").
		Wrapf(ErrCloneTooLarge, "skill source %q: the clone of %s grew past max_clone_bytes = %d (reached %d bytes); nothing was cached", r.name, includes.RedactURL(r.url), r.maxBytes, grown)
}

// dirBytes sums the sizes of the regular files below dir.
func dirBytes(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error { //nolint:errcheck // a vanishing file only lowers the sum
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best-effort measurement
		}
		if info, infoErr := d.Info(); infoErr == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// sizeWatch aborts a clone that grows past its limit while git is still running.
type sizeWatch struct {
	exceeded atomic.Int64 // bytes seen when the limit was passed; 0 while within it
	stop     func()
}

// watchSize polls dest and cancels the command context once it grows past limit.
func watchSize(dest string, limit int64, cancel context.CancelFunc) *sizeWatch {
	w := &sizeWatch{}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(cloneWatchInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if n := dirBytes(dest); n > limit {
					w.exceeded.Store(n)
					cancel()
					return
				}
			}
		}
	}()
	w.stop = func() {
		close(done)
		<-finished
	}
	return w
}
