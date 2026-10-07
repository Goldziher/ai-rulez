package telemetry

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
)

// CursorFileName is the export cursor inside the machine-local directory.
const CursorFileName = "telemetry-cursor.json"

const (
	cursorVersion = 1
	// SentRingMax bounds how many delivered event ids the cursor remembers.
	SentRingMax = 4096
	// DefaultCatchUpMax bounds the events one catch-up queues.
	DefaultCatchUpMax = 2000
)

// Cursor records how far into the usage log events are accounted for: queued for
// export, already delivered, or older than the user's consent. The usage log is
// the source of truth; the cursor is what keeps an event from being sent twice by
// the two ways one can reach the collector (the outbox the hooks fill, and a
// catch-up that reads the log).
type Cursor struct {
	Version int `json:"version"`
	// LogID identifies the log the offset belongs to: sha256 of its first line, so
	// a rotated or truncated log is noticed and read from the start.
	LogID string `json:"log_id,omitempty"`
	// Offset is the byte offset in the log up to which lines are accounted for.
	Offset      int64  `json:"offset"`
	LastEventID string `json:"last_event_id,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	// PlacedAt is when the cursor was placed (PlaceCursor, or the first catch-up
	// of the log). A cursor placed before the current consent was granted belongs
	// to a project that may have kept recording while consent was off.
	PlacedAt string `json:"placed_at,omitempty"`
	// Sent is a ring of the most recently delivered (or permanently rejected)
	// event ids, oldest first. A catch-up skips them, so an event the outbox already
	// delivered is not delivered again from the log.
	Sent []string `json:"sent,omitempty"`
}

// Set reports whether a cursor was ever written.
func (c Cursor) Set() bool { return c.Version != 0 }

func (s *Spool) cursorPath() string { return filepath.Join(s.Dir, CursorFileName) }

// ReadCursor returns the stored cursor, the zero Cursor when there is none.
func (s *Spool) ReadCursor() Cursor {
	var c Cursor
	if data, err := safefs.ReadRegular(s.cursorPath()); err == nil {
		if json.Unmarshal(data, &c) != nil || c.Version != cursorVersion {
			return Cursor{} // a corrupt or foreign cursor reads as none
		}
	}
	return c
}

// UpdateCursor applies fn to the cursor under the spool lock and writes it back.
func (s *Spool) UpdateCursor(fn func(*Cursor)) error {
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return oops.With("path", s.Dir).Wrapf(err, "create telemetry directory")
	}
	release, err := lock(filepath.Join(s.Dir, lockFileName), rewriteLockWait, staleLock)
	if err != nil {
		return err
	}
	defer release()
	return s.updateCursorLocked(fn)
}

func (s *Spool) updateCursorLocked(fn func(*Cursor)) error {
	c := s.ReadCursor()
	fn(&c)
	c.Version = cursorVersion
	if len(c.Sent) > SentRingMax {
		c.Sent = c.Sent[len(c.Sent)-SentRingMax:]
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode telemetry cursor")
	}
	return writeFileAtomic(s.cursorPath(), append(data, '\n'))
}

// MarkSent adds delivered event ids to the cursor's ring.
func (s *Spool) MarkSent(ids []string, now string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.UpdateCursor(func(c *Cursor) {
		c.Sent = append(c.Sent, ids...)
		c.UpdatedAt = now
	})
}

// AppendMany adds events to the outbox in one locked write. Like Append it waits
// at most briefly for the lock; unlike Append it waits for the rewrite time,
// because it runs in a flush, not in a hook.
func (s *Spool) AppendMany(events []Event) error {
	if len(events) == 0 {
		return nil
	}
	buf, err := encodeEvents(events)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return oops.With("path", s.Dir).Wrapf(err, "create telemetry directory")
	}
	release, err := lock(filepath.Join(s.Dir, lockFileName), rewriteLockWait, staleLock)
	if err != nil {
		return err
	}
	defer release()
	return s.appendManyLocked(buf)
}

func (s *Spool) appendManyLocked(lines []byte) error {
	if err := appendLine(s.outbox(), lines); err != nil {
		return err
	}
	if info, statErr := os.Stat(s.outbox()); statErr == nil && info.Size() > int64(s.maxEvents())*bytesPerEvent {
		return s.trimLocked()
	}
	return nil
}

func encodeEvents(events []Event) ([]byte, error) {
	var buf bytes.Buffer
	for i := range events {
		line, err := json.Marshal(&events[i])
		if err != nil {
			return nil, oops.Wrapf(err, "encode telemetry event")
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// CatchUpOptions configures CatchUp.
type CatchUpOptions struct {
	// Sample is the export sample fraction (Settings.Sample), applied the way the
	// recorder applies it: 0 exports nothing. nil applies no sampling at all, for
	// a count of the log rather than an export.
	Sample *float64
	// All reads the log from its first line instead of the cursor: the way to export
	// history, which a consent never covers on its own.
	All bool
	// Max bounds the events queued in one call (DefaultCatchUpMax); the rest is
	// picked up by the next call.
	Max int
	// DryRun computes the result without queueing or moving the cursor.
	DryRun bool
	// GrantedAt is when the consent the export runs under was given (zero for
	// none). Unless All, events recorded before it are not queued while the
	// cursor was placed before it: consent is not retroactive, so a project that
	// kept recording while consent was off does not export that gap.
	GrantedAt time.Time
}

// CatchUpResult says what a catch-up did.
type CatchUpResult struct {
	// Queued events were added to the outbox; Skipped ones were already there or
	// already delivered; Rejected lines failed validation.
	Queued, Skipped, Rejected int
	// Initialized is set when there was no cursor, so it was placed at the end of
	// the log and nothing was queued: events from before export was on are never
	// sent unless All is given.
	Initialized bool
	// Reset is set when the log no longer matches the cursor (rotated, truncated or
	// pruned elsewhere), so it was read from the start.
	Reset bool
	// More is set when Max stopped the read before the end of the log.
	More bool
	// Events are the queued events (the would-be ones for a DryRun).
	Events []Event
}

// LogID returns the identity of a usage log: sha256 of its first complete line,
// and the byte length of that line including its newline. "" for an empty log or
// one with no complete first line.
func LogID(path string) (id string, firstLen int64, err error) {
	file, err := safefs.OpenRegular(path)
	if err != nil {
		return "", 0, err //nolint:wrapcheck // callers test os.ErrNotExist
	}
	defer file.Close() //nolint:errcheck // read-only
	line, err := bufio.NewReaderSize(file, 64*1024).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", 0, oops.With("path", path).Wrapf(err, "read usage log")
	}
	if len(line) == 0 || line[len(line)-1] != '\n' {
		return "", 0, nil
	}
	sum := sha256.Sum256(line)
	return hex.EncodeToString(sum[:]), int64(len(line)), nil
}

// CatchUp reads the usage log from the cursor, queues the events that are not
// already in the outbox or delivered, and moves the cursor to the end of what it
// read. Only complete lines are read, so a hook mid-append is picked up next time.
//
// Unless it is a DryRun it holds the spool lock from reading the cursor to writing
// it back: two catch-ups (a background flush and an export) would otherwise read
// the same cursor and queue the same events twice.
func (s *Spool) CatchUp(logPath string, o CatchUpOptions) (CatchUpResult, error) {
	if o.DryRun {
		return s.catchUp(logPath, o)
	}
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return CatchUpResult{}, oops.With("path", s.Dir).Wrapf(err, "create telemetry directory")
	}
	release, err := lock(filepath.Join(s.Dir, lockFileName), rewriteLockWait, staleLock)
	if err != nil {
		return CatchUpResult{}, err
	}
	defer release()
	return s.catchUp(logPath, o)
}

// catchUp is CatchUp with the spool lock held (or not needed: DryRun).
func (s *Spool) catchUp(logPath string, o CatchUpOptions) (CatchUpResult, error) {
	var result CatchUpResult
	logID, _, err := LogID(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil // no log yet: nothing to catch up on
		}
		return result, err
	}
	cur := s.ReadCursor()
	offset := startOffset(cur, logID, o, &result)

	file, err := safefs.OpenRegular(logPath)
	if err != nil {
		return result, oops.With("path", logPath).Wrapf(err, "open usage log")
	}
	defer file.Close()      //nolint:errcheck // read-only
	if result.Initialized { // forward only: place the cursor at the end, queue nothing
		end, err := completeEnd(file)
		if err != nil || o.DryRun {
			return result, err
		}
		return result, s.updateCursorLocked(func(c *Cursor) { c.LogID, c.Offset, c.PlacedAt = logID, end, FormatTime(time.Now()) })
	}
	skip, err := s.knownIDs(cur)
	if err != nil {
		return result, err
	}
	tail, err := readTail(file, offset, skip, o, notBefore(cur, o))
	if err != nil {
		return result, err
	}
	result.Events, result.Skipped, result.Rejected, result.Queued, result.More = tail.events, tail.skipped, tail.rejected, len(tail.events), tail.more
	if o.DryRun {
		return result, nil
	}
	if len(result.Events) > 0 {
		lines, err := encodeEvents(result.Events)
		if err != nil {
			return result, err
		}
		if err := s.appendManyLocked(lines); err != nil {
			return result, err // the cursor stays: the events are read again next time
		}
	}
	return result, s.updateCursorLocked(func(c *Cursor) {
		c.LogID, c.Offset = logID, tail.end
		if tail.lastID != "" {
			c.LastEventID = tail.lastID
		}
	})
}

// startOffset decides where a catch-up starts and records in result why: from the
// start with All, at the end when no cursor was ever set (Initialized), from the
// start when the log no longer matches the cursor (Reset), else at the cursor.
func startOffset(cur Cursor, logID string, o CatchUpOptions, result *CatchUpResult) int64 {
	switch {
	case o.All:
		return 0
	case !cur.Set():
		result.Initialized = true
		return 0
	case cur.LogID != logID:
		result.Reset = true
		return 0
	}
	return cur.Offset
}

// tailRead is what readTail found.
type tailRead struct {
	events            []Event
	skipped, rejected int
	end               int64
	lastID            string
	more              bool
}

// notBefore is the earliest event time a catch-up may queue: the consent's grant
// time when the cursor predates it (or its placement is unknown), else zero.
func notBefore(cur Cursor, o CatchUpOptions) time.Time {
	if o.All || o.GrantedAt.IsZero() {
		return time.Time{}
	}
	if placed, err := time.Parse(time.RFC3339, cur.PlacedAt); err == nil && !placed.Before(o.GrantedAt) {
		return time.Time{} // placed under this consent: what it covers was chosen then (--backfill)
	}
	return o.GrantedAt
}

// recordedBefore reports whether e was recorded before t; an event without a
// readable time counts as before.
func recordedBefore(e *Event, t time.Time) bool {
	at, err := time.Parse(time.RFC3339, e.Time)
	return err != nil || at.Before(t)
}

// readTail reads complete log lines from offset until the bound or the end of the
// log, keeping the events that are not in skip, are inside the sample and were
// not recorded before notBefore.
func readTail(file *os.File, offset int64, skip map[string]bool, o CatchUpOptions, notBefore time.Time) (tailRead, error) {
	tail := tailRead{end: offset}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return tail, oops.Wrapf(err, "seek usage log")
	}
	limit := o.Max
	if limit <= 0 {
		limit = DefaultCatchUpMax
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	for len(tail.events)+tail.skipped < limit {
		line, _ := reader.ReadBytes('\n') //nolint:errcheck // a short or failed read yields an incomplete line, handled below
		if len(line) == 0 || line[len(line)-1] != '\n' {
			break // EOF, or a line a hook is still writing
		}
		lineStart := tail.end
		tail.end += int64(len(line))
		event, ok := decodeLogLine(bytes.TrimSpace(line))
		if !ok {
			continue
		}
		if event.EventID == "" {
			sum := sha256.Sum256(append(bytes.TrimSpace(line), "\x00@"+strconv.FormatInt(lineStart, 10)...))
			event.EventID = hex.EncodeToString(sum[:8])
		}
		if event.Normalize() != nil {
			tail.rejected++
			continue
		}
		tail.lastID = event.EventID
		if skip[event.EventID] || (o.Sample != nil && !Sampled(*o.Sample, &event)) ||
			(!notBefore.IsZero() && recordedBefore(&event, notBefore)) {
			tail.skipped++
			continue
		}
		skip[event.EventID] = true
		tail.events = append(tail.events, event)
	}
	if full, err := completeEnd(file); err == nil {
		tail.more = tail.end < full
	}
	return tail, nil
}

// knownIDs is the set of event ids that must not be queued again: those waiting
// in the outbox and those the cursor remembers as delivered.
func (s *Spool) knownIDs(cur Cursor) (map[string]bool, error) {
	pending, _, err := s.Pending()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(pending)+len(cur.Sent))
	for i := range pending {
		known[pending[i].EventID] = true
	}
	for _, id := range cur.Sent {
		known[id] = true
	}
	return known, nil
}

// completeEnd is the offset just past the last newline of the file.
func completeEnd(file *os.File) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, oops.Wrapf(err, "stat usage log")
	}
	size := info.Size()
	const window = 64 * 1024
	for end := size; end > 0; {
		start := max(end-window, 0)
		buf := make([]byte, end-start)
		if _, err := file.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
			return 0, oops.Wrapf(err, "read usage log")
		}
		if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
			return start + int64(i) + 1, nil
		}
		end = start
	}
	return 0, nil
}

// PlaceCursor sets the cursor on the log: at its end, so only events recorded from
// now on are exported (what `telemetry enable` does, because consent is not
// retroactive), or at its start with fromStart so the existing history is exported
// too. A missing or empty log places the cursor at offset 0.
func (s *Spool) PlaceCursor(logPath string, fromStart bool) error {
	logID, _, err := LogID(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var end int64
	if !fromStart && logID != "" {
		file, openErr := safefs.OpenRegular(logPath)
		if openErr != nil {
			return oops.With("path", logPath).Wrapf(openErr, "open usage log")
		}
		defer file.Close() //nolint:errcheck // read-only
		if end, err = completeEnd(file); err != nil {
			return err
		}
	}
	return s.UpdateCursor(func(c *Cursor) {
		c.LogID, c.Offset, c.LastEventID, c.PlacedAt = logID, end, "", FormatTime(time.Now())
	})
}

// PendingInLog counts the events after the cursor that a catch-up would look at
// (valid lines only), for `telemetry status`. It is exact for the lines it reads,
// capped at limit lines.
func (s *Spool) PendingInLog(logPath string, limit int) (int, error) {
	res, err := s.CatchUp(logPath, CatchUpOptions{DryRun: true, Max: limit})
	if err != nil {
		return 0, err
	}
	return res.Queued, nil
}
