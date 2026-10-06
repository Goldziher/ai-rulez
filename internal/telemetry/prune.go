package telemetry

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
)

// ErrCursorMismatch means the export cursor describes another log than the one
// named, so pruning could remove events that were never exported.
var ErrCursorMismatch = errors.New("the export cursor does not match this usage log")

// PruneOptions configures Spool.PruneLog.
type PruneOptions struct {
	// Cutoff removes lines older than this time.
	Cutoff time.Time
	DryRun bool
	// IgnoreCursor prunes by age alone, also lines not yet exported.
	IgnoreCursor bool
}

// PruneLog removes usage-log lines older than the cutoff that are behind the
// export cursor: events already queued, delivered or older than consent are
// dropped, events still waiting for export are kept however old. With no cursor
// nothing is waiting, so age alone decides. The cursor is moved to match the
// rewritten log. If the cursor describes another log (the file was rotated since
// the last flush) the prune refuses rather than guess; IgnoreCursor overrides.
func (s *Spool) PruneLog(logPath string, o PruneOptions) (usage.PruneResult, error) {
	if o.DryRun {
		return s.pruneLog(logPath, o)
	}
	// The cursor read, the log rewrite and the cursor update are one step: a flush
	// or catch-up in between would read the cursor against the old log.
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return usage.PruneResult{}, oops.With("path", s.Dir).Wrapf(err, "create telemetry directory")
	}
	release, err := lock(filepath.Join(s.Dir, lockFileName), rewriteLockWait, staleLock)
	if err != nil {
		return usage.PruneResult{}, err
	}
	defer release()
	return s.pruneLog(logPath, o)
}

// pruneLog is PruneLog with the spool lock already held (or not needed: DryRun).
func (s *Spool) pruneLog(logPath string, o PruneOptions) (usage.PruneResult, error) {
	cur := s.ReadCursor()
	opts := usage.PruneOptions{Cutoff: o.Cutoff, DryRun: o.DryRun}
	oldID, _, err := LogID(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return usage.PruneResult{}, err
	}
	// A cursor placed before the log existed (no id, offset 0) matches any log: it
	// has accounted for nothing, so it protects every line.
	sameLog := cur.Set() && (cur.LogID == oldID || (cur.LogID == "" && cur.Offset == 0))
	if cur.Set() && !o.IgnoreCursor {
		if !sameLog {
			return usage.PruneResult{}, oops.Hint("Pass --ignore-cursor to prune by age alone, or run `ai-rulez telemetry flush` first to bring the cursor up to date.").
				Wrapf(ErrCursorMismatch, "prune usage log")
		}
		opts.Protect, opts.ProtectFrom = true, cur.Offset
	}
	opts.TrackOffset = cur.Offset
	res, err := usage.PruneLog(logPath, opts)
	if err != nil || !res.Rewritten || !cur.Set() {
		return res, err //nolint:wrapcheck // usage errors carry the path
	}
	newID, _, idErr := LogID(logPath)
	if idErr != nil {
		return res, idErr
	}
	return res, s.updateCursorLocked(func(c *Cursor) {
		c.LogID = newID
		if sameLog {
			// Every removed line before the cursor shifts it back; lines past it (only
			// with IgnoreCursor) do not move it.
			c.Offset = max(c.Offset-res.RemovedBeforeTrack, 0)
		} else {
			c.Offset = 0 // the cursor described another log: read this one from its start
		}
	})
}
