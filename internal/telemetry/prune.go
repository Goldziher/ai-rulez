package telemetry

import (
	"errors"
	"os"
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
	cur := s.ReadCursor()
	opts := usage.PruneOptions{Cutoff: o.Cutoff, DryRun: o.DryRun}
	if cur.Set() && !o.IgnoreCursor {
		id, _, err := LogID(logPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return usage.PruneResult{}, err
		}
		if id != cur.LogID {
			return usage.PruneResult{}, oops.Hint("Run `ai-rulez telemetry flush` to bring the cursor up to date, or pass --ignore-cursor to prune by age alone.").
				Wrapf(ErrCursorMismatch, "prune usage log")
		}
		opts.Protect, opts.ProtectFrom = true, cur.Offset
	}
	res, err := usage.PruneLog(logPath, opts)
	if err != nil || !res.Rewritten || !cur.Set() {
		return res, err //nolint:wrapcheck // usage errors carry the path
	}
	newID, _, idErr := LogID(logPath)
	if idErr != nil {
		return res, idErr
	}
	moved := res.RemovedBytes
	if o.IgnoreCursor {
		// Lines past the cursor may have been removed too: the offset cannot be mapped
		// exactly, so restart from the beginning of what remains. Events already queued
		// or delivered are skipped by id; the rest are exported.
		return res, s.UpdateCursor(func(c *Cursor) { c.LogID, c.Offset = newID, 0 })
	}
	return res, s.UpdateCursor(func(c *Cursor) {
		c.LogID, c.Offset = newID, max(c.Offset-moved, 0)
	})
}
