package skillsource

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
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

	// DefaultMaxCloneFiles is the limit on the number of entries (files,
	// directories, links) of a clone when nothing else sets one.
	DefaultMaxCloneFiles = 20000
	// EnvMaxCloneFiles is the environment variable that sets the global entry
	// limit; [[skill_sources]] max_clone_files overrides it per source.
	EnvMaxCloneFiles = "AI_RULEZ_MAX_CLONE_FILES"

	// cloneEntryFloor is the least size charged per entry: a file system block.
	// Without it a million empty files would fill the disk (inodes, blocks) while
	// counting as zero bytes.
	cloneEntryFloor = 4096

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
	if v, err := strconv.ParseInt(strings.TrimSpace(ambient.Getenv(nil, EnvMaxCloneBytes)), 10, 64); err == nil && v > 0 {
		return v
	}
	return DefaultMaxCloneBytes
}

// maxCloneFiles picks the entry limit of a source: its own max_clone_files, then
// AI_RULEZ_MAX_CLONE_FILES, then the default.
func (s Spec) maxCloneFiles() int {
	if s.MaxCloneFiles > 0 {
		return s.MaxCloneFiles
	}
	if v, err := strconv.Atoi(strings.TrimSpace(ambient.Getenv(nil, EnvMaxCloneFiles))); err == nil && v > 0 {
		return v
	}
	return DefaultMaxCloneFiles
}

// cloneRequest describes one fetch of a commit.
type cloneRequest struct {
	name, url, ref, kind, commit string
	// path is the source path; a non-empty path fetches only that subtree.
	path string
	// maxBytes bounds the clone (git metadata plus checkout).
	maxBytes int64
	// maxFiles bounds the number of entries of the clone; 0 selects the default.
	maxFiles int
}

func (r cloneRequest) fileLimit() int {
	if r.maxFiles > 0 {
		return r.maxFiles
	}
	return DefaultMaxCloneFiles
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

// cloneUsage is what a clone directory holds: bytes (each entry charged at least
// cloneEntryFloor) and entries.
type cloneUsage struct {
	bytes   int64
	entries int
}

func (r cloneRequest) over(u cloneUsage) bool {
	return u.bytes > r.maxBytes || u.entries > r.fileLimit()
}

func (r cloneRequest) tooLarge(u cloneUsage) error {
	hint := "Set `path` to the directory that holds the skills, or raise max_clone_bytes / max_clone_files on the source (global: " + EnvMaxCloneBytes + ", " + EnvMaxCloneFiles + ")"
	if u.entries > r.fileLimit() {
		return oops.With("url", includes.RedactURL(r.url)).With("limit_files", r.fileLimit()).Hint(hint).
			Wrapf(ErrCloneTooLarge, "skill source %q: the clone of %s holds more than max_clone_files = %d entries; nothing was cached", r.name, includes.RedactURL(r.url), r.fileLimit())
	}
	return oops.With("url", includes.RedactURL(r.url)).With("limit_bytes", r.maxBytes).Hint(hint).
		Wrapf(ErrCloneTooLarge, "skill source %q: the clone of %s grew past max_clone_bytes = %d (reached %d bytes); nothing was cached", r.name, includes.RedactURL(r.url), r.maxBytes, u.bytes)
}

// measure walks dir and stops as soon as a limit of r is passed, so the walk is
// bounded by the limits however many entries the tree holds. Each non-directory
// is charged max(size, cloneEntryFloor) and every entry, directories included,
// counts toward the entry limit.
func (r cloneRequest) measure(dir string) cloneUsage {
	var u cloneUsage
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // a vanishing file only lowers the sum
		if err != nil || p == dir {
			return nil //nolint:nilerr // best-effort measurement
		}
		u.entries++
		size := int64(cloneEntryFloor)
		if !d.IsDir() {
			if info, infoErr := d.Info(); infoErr == nil && info.Size() > size {
				size = info.Size()
			}
		}
		u.bytes += size
		if r.over(u) {
			return fs.SkipAll
		}
		return nil
	})
	return u
}

// sizeWatch aborts a clone that grows past its limits while git is still running.
type sizeWatch struct {
	exceeded atomic.Pointer[cloneUsage] // usage seen when a limit was passed; nil while within them
	stop     func()
}

// watchSize polls dest and cancels the command context once it grows past a limit of req.
func watchSize(dest string, req cloneRequest, cancel context.CancelFunc) *sizeWatch {
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
				if u := req.measure(dest); req.over(u) {
					w.exceeded.Store(&u)
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
