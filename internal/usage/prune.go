package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
)

// maxPruneBytes bounds the log a prune loads: it rewrites the file as a whole.
const maxPruneBytes = 256 << 20

// pruneRetries is how often a prune re-reads a log that grew while it worked.
const pruneRetries = 3

// ErrLogBusy means the log kept changing while a prune read it, so nothing was
// rewritten: replacing it could have dropped a line a hook appended meanwhile.
var ErrLogBusy = errors.New("usage log changed while it was being pruned")

// PruneOptions configures PruneLog.
type PruneOptions struct {
	// Cutoff removes lines whose ts is before it.
	Cutoff time.Time
	// ProtectFrom keeps every line that starts at or after this byte offset, however
	// old: with an export cursor it is the offset of the first line not yet
	// exported. Protect must be true for it to apply.
	ProtectFrom int64
	Protect     bool
	// DryRun computes the result without rewriting the log.
	DryRun bool
	// TrackOffset asks PruneResult.RemovedBeforeTrack to count the removed bytes
	// of lines that start before this offset, so a caller can map an offset into
	// the old log onto the new one even when lines past it were removed too.
	TrackOffset int64
}

// PruneResult says what a prune removed.
type PruneResult struct {
	Scanned int `json:"scanned"`
	Removed int `json:"removed"`
	Kept    int `json:"kept"`
	// Protected counts old lines kept because they are past ProtectFrom; Unreadable
	// counts lines kept because they have no readable timestamp (never guessed old).
	Protected  int `json:"protected"`
	Unreadable int `json:"unreadable"`
	// RemovedBytes is the size of the removed lines including newlines. Every
	// removed line lies before ProtectFrom, so an offset into the old log maps to
	// offset-RemovedBytes in the new one.
	RemovedBytes int64 `json:"removed_bytes"`
	// RemovedBeforeTrack is the size of the removed lines that start before
	// PruneOptions.TrackOffset: an offset at a line boundary maps to
	// offset-RemovedBeforeTrack in the new log.
	RemovedBeforeTrack int64 `json:"-"`
	// Rewritten is set when the log file was replaced.
	Rewritten bool `json:"rewritten"`
}

// PruneLog removes the lines of a usage log older than the cutoff, except those at
// or after ProtectFrom, and replaces the file atomically (mode 0600). A line with
// no readable timestamp is kept.
//
// A real prune holds the exclusive log lock (flock; LockFileEx on Windows, on
// <log>.lock) that every recorder holds shared while it appends, so no line is
// written between the read and the rename. It waits a few seconds for running
// appenders and fails with ErrLogLocked otherwise. A recorder that could not get
// the lock in time appends anyway, so the prune also compares the file before and
// after reading and retries, and gives up with ErrLogBusy if it keeps changing.
// Only that last-resort path leaves a window (microseconds between the check and
// the rename) in which an unlocked append is lost; the log is a telemetry buffer,
// not a record of truth.
func PruneLog(path string, o PruneOptions) (PruneResult, error) {
	if !o.DryRun {
		release, err := lockLog(path, true, pruneLockWait)
		if err != nil {
			return PruneResult{}, err
		}
		defer release()
	}
	for attempt := 0; attempt < pruneRetries; attempt++ {
		res, kept, stable, err := pruneOnce(path, o)
		if err != nil {
			return PruneResult{}, err
		}
		if !stable {
			continue // the log grew while it was read: read it again
		}
		if res.Removed == 0 || o.DryRun {
			return res, nil
		}
		if err := safefs.WriteFileAtomic(path, kept); err != nil {
			return PruneResult{}, oops.With("path", path).Wrapf(err, "rewrite usage log")
		}
		res.Rewritten = true
		return res, nil
	}
	return PruneResult{}, ErrLogBusy
}

// pruneOnce reads the log and decides what to keep; stable is false when the file
// changed size while it was read.
func pruneOnce(path string, o PruneOptions) (res PruneResult, kept []byte, stable bool, err error) {
	file, err := safefs.OpenRegular(path)
	if err != nil {
		return res, nil, false, oops.With("path", path).Wrapf(err, "open usage log")
	}
	defer file.Close() //nolint:errcheck // read-only
	before, err := file.Stat()
	if err != nil {
		return res, nil, false, oops.Wrapf(err, "stat usage log")
	}
	if before.Size() > maxPruneBytes {
		return res, nil, false, oops.With("path", path).Errorf("usage log is larger than %d MiB; export it and start a new one", maxPruneBytes>>20)
	}

	var out bytes.Buffer
	reader := bufio.NewReaderSize(file, 256*1024)
	var offset int64
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				out.Write(line) // a line a hook is still writing: never touched
				break
			}
			res.Scanned++
			start := offset
			offset += int64(len(line))
			res.tally(classify(line, start, o), line, start, o.TrackOffset, &out)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return res, nil, false, oops.With("path", path).Wrapf(readErr, "read usage log")
		}
	}
	after, err := safefs.OpenRegular(path)
	if err != nil {
		return res, nil, false, oops.With("path", path).Wrapf(err, "reopen usage log")
	}
	defer after.Close() //nolint:errcheck // read-only
	info, err := after.Stat()
	if err != nil {
		return res, nil, false, oops.Wrapf(err, "stat usage log")
	}
	stable = info.Size() == before.Size() && info.ModTime().Equal(before.ModTime())
	return res, out.Bytes(), stable, nil
}

// tally counts one complete line of class and keeps it in out unless it is removed.
func (res *PruneResult) tally(class lineClass, line []byte, start, trackOffset int64, out *bytes.Buffer) {
	if class == lineRemove {
		res.Removed++
		res.RemovedBytes += int64(len(line))
		if start < trackOffset {
			res.RemovedBeforeTrack += int64(len(line))
		}
		return
	}
	res.Kept++
	switch class {
	case linePermanent:
		res.Unreadable++
	case lineProtected:
		res.Protected++
	}
	out.Write(line)
}

type lineClass int

const (
	lineKeep lineClass = iota
	lineRemove
	linePermanent
	lineProtected
)

// classify decides one line. Only a line with a parseable ts before the cutoff and
// before ProtectFrom is removed.
func classify(line []byte, start int64, o PruneOptions) lineClass {
	var probe struct {
		Time string `json:"ts"`
	}
	if json.Unmarshal(bytes.TrimSpace(line), &probe) != nil {
		return linePermanent
	}
	ts, err := time.Parse(time.RFC3339, probe.Time)
	if err != nil {
		return linePermanent
	}
	if !ts.Before(o.Cutoff) {
		return lineKeep
	}
	if o.Protect && start >= o.ProtectFrom {
		return lineProtected
	}
	return lineRemove
}
