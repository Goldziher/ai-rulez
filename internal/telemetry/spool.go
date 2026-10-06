package telemetry

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/samber/oops"
)

// Spool file names, inside the machine-local directory.
const (
	OutboxFileName = "telemetry-outbox.jsonl"
	StateFileName  = "telemetry-state.json"
	lockFileName   = ".telemetry.lock"
	flushLockName  = ".telemetry-flush.lock"
	spawnMarker    = ".telemetry-spawn"
)

// Spool limits.
const (
	// DefaultMaxEvents bounds the outbox; beyond MaxEvents*bytesPerEvent bytes the
	// oldest events are dropped down to 80% of MaxEvents.
	DefaultMaxEvents = 10000
	// bytesPerEvent sizes the trim trigger: an event line is about 300 bytes.
	bytesPerEvent   = 512
	appendLockWait  = 25 * time.Millisecond
	rewriteLockWait = 2 * time.Second
	staleLock       = 10 * time.Second
	// staleFlushLock is the stale time of the single-flusher lock. A flush holds it
	// for its whole run, so it must outlast MaxFlushTimeout with a wide margin.
	staleFlushLock = 2 * MaxFlushTimeout
)

// ErrBusy means the spool lock could not be taken in time. The recorder treats it
// as a dropped event: a hook never waits on telemetry.
var ErrBusy = errors.New("telemetry spool busy")

// Spool is the bounded on-disk outbox between the recorder (short-lived hook
// processes) and the exporter (a separate flush). Files are mode 0600.
type Spool struct {
	Dir       string
	MaxEvents int
}

// State is the exporter's bookkeeping, shown by `telemetry doctor`.
type State struct {
	LastAttempt string `json:"last_attempt,omitempty"`
	LastFlush   string `json:"last_flush,omitempty"`
	LastStatus  string `json:"last_status,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	Sent        int64  `json:"sent"`
	Dropped     int64  `json:"dropped"`
	Rejected    int64  `json:"rejected"`
}

func (s *Spool) outbox() string    { return filepath.Join(s.Dir, OutboxFileName) }
func (s *Spool) statePath() string { return filepath.Join(s.Dir, StateFileName) }

func (s *Spool) maxEvents() int {
	if s.MaxEvents > 0 {
		return s.MaxEvents
	}
	return DefaultMaxEvents
}

// Append adds one event. It takes the lock for at most 25 ms and otherwise
// returns ErrBusy.
func (s *Spool) Append(event *Event) error {
	line, err := json.Marshal(event)
	if err != nil {
		return oops.Wrapf(err, "encode telemetry event")
	}
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return oops.With("path", s.Dir).Wrapf(err, "create telemetry directory")
	}
	release, err := lock(filepath.Join(s.Dir, lockFileName), appendLockWait, staleLock)
	if err != nil {
		return err
	}
	defer release()
	if err := appendLine(s.outbox(), append(line, '\n')); err != nil {
		return err
	}
	if info, statErr := os.Stat(s.outbox()); statErr == nil && info.Size() > int64(s.maxEvents())*bytesPerEvent {
		return s.trimLocked()
	}
	return nil
}

// trimLocked drops the oldest events down to 80% of the bound. The caller holds the lock.
func (s *Spool) trimLocked() error {
	lines, err := readLines(s.outbox())
	if err != nil {
		return err
	}
	keep := s.maxEvents() * 4 / 5
	if len(lines) <= keep {
		return nil
	}
	dropped := len(lines) - keep
	if err := writeLines(s.outbox(), lines[dropped:]); err != nil {
		return err
	}
	return s.updateStateLocked(func(st *State) { st.Dropped += int64(dropped) })
}

// Pending returns the parsed events waiting in the outbox, oldest first, and the
// number of unreadable lines (which the next rewrite drops).
func (s *Spool) Pending() (events []Event, corrupt int, err error) {
	lines, err := readLines(s.outbox())
	if err != nil {
		return nil, 0, err
	}
	for _, line := range lines {
		var event Event
		if json.Unmarshal(line, &event) != nil || event.EventID == "" {
			corrupt++
			continue
		}
		events = append(events, event)
	}
	return events, corrupt, nil
}

// Size returns the outbox size in bytes, 0 when absent.
func (s *Spool) Size() int64 {
	info, err := os.Stat(s.outbox())
	if err != nil {
		return 0
	}
	return info.Size()
}

// Remove deletes the events with the given ids (delivered or quarantined) and any
// unreadable line. Matching by id, not by position, makes it safe when a trim or
// another append ran since the events were read.
func (s *Spool) Remove(ids map[string]bool) error {
	release, err := lock(filepath.Join(s.Dir, lockFileName), rewriteLockWait, staleLock)
	if err != nil {
		return err
	}
	defer release()
	lines, err := readLines(s.outbox())
	if err != nil {
		return err
	}
	kept := lines[:0:0]
	for _, line := range lines {
		var event Event
		if json.Unmarshal(line, &event) != nil || ids[event.EventID] {
			continue
		}
		kept = append(kept, line)
	}
	return writeLines(s.outbox(), kept)
}

// ReadState returns the exporter bookkeeping; a missing file is the zero State.
func (s *Spool) ReadState() State {
	var st State
	if data, err := os.ReadFile(s.statePath()); err == nil { //nolint:gosec // machine-local state file
		_ = json.Unmarshal(data, &st) //nolint:errcheck // a corrupt state file reads as empty
	}
	return st
}

// UpdateState applies fn to the state under the lock.
func (s *Spool) UpdateState(fn func(*State)) error {
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return oops.With("path", s.Dir).Wrapf(err, "create telemetry directory")
	}
	release, err := lock(filepath.Join(s.Dir, lockFileName), rewriteLockWait, staleLock)
	if err != nil {
		return err
	}
	defer release()
	return s.updateStateLocked(fn)
}

func (s *Spool) updateStateLocked(fn func(*State)) error {
	st := s.ReadState()
	fn(&st)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode telemetry state")
	}
	return writeFileAtomic(s.statePath(), append(data, '\n'))
}

// TryFlushLock takes the single-flusher lock without waiting; ok is false when
// another flusher holds it.
func (s *Spool) TryFlushLock() (release func(), ok bool) {
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return nil, false
	}
	release, err := lock(filepath.Join(s.Dir, flushLockName), 0, staleFlushLock)
	if err != nil {
		return nil, false
	}
	return release, true
}

// SpawnDue reports whether a detached flush should start now: the outbox is big
// enough, or the last flush is older than interval, and no flush was started in
// the last minute. It records the start when it answers true.
func (s *Spool) SpawnDue(now time.Time, interval time.Duration) bool {
	if s.Size() == 0 {
		return false
	}
	marker := filepath.Join(s.Dir, spawnMarker)
	if info, err := os.Stat(marker); err == nil && now.Sub(info.ModTime()) < time.Minute {
		return false
	}
	due := s.Size() >= 64<<10
	if !due {
		last, err := time.Parse(time.RFC3339, s.ReadState().LastFlush)
		due = err != nil || now.Sub(last) >= interval
	}
	if due {
		_ = os.WriteFile(marker, nil, 0o600) //nolint:errcheck // the marker only rate-limits
	}
	return due
}

func readLines(path string) ([][]byte, error) {
	file, err := os.Open(path) //nolint:gosec // machine-local spool
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", path).Wrapf(err, "open telemetry outbox")
	}
	defer file.Close() //nolint:errcheck // read-only
	var lines [][]byte
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		if line := bytes.TrimSpace(scanner.Bytes()); len(line) > 0 {
			lines = append(lines, append([]byte(nil), line...))
		}
	}
	return lines, oops.Wrapf(scanner.Err(), "read telemetry outbox")
}

func writeLines(path string, lines [][]byte) error {
	var buf bytes.Buffer
	for _, line := range lines {
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return writeFileAtomic(path, buf.Bytes())
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return oops.With("path", path).Wrapf(err, "create temp file")
	}
	name := tmp.Name()
	if err := os.Chmod(name, 0o600); err != nil {
		_ = tmp.Close()     //nolint:errcheck // the chmod error is the one to report
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		return oops.Wrapf(err, "chmod temp file")
	}
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		return oops.With("path", path).Wrapf(err, "write temp file")
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		return oops.With("path", path).Wrapf(err, "replace file")
	}
	return nil
}

// lock takes an exclusive lock file, waiting up to wait. A lock older than
// stale is treated as left by a crashed process and taken over.
//
// The lock file holds a random token. Release removes the file only while it
// still holds this holder's token, so a holder whose lock was taken over (it ran
// past stale) never deletes the new owner's lock. A takeover runs under a short
// guard file (path + ".takeover"): two processes that both found the same stale
// lock cannot each remove the other's fresh one.
func lock(path string, wait, stale time.Duration) (release func(), err error) {
	token := newLockToken()
	deadline := time.Now().Add(wait)
	for {
		if created, createErr := createLock(path, token); createErr != nil {
			return nil, oops.With("path", path).Wrapf(createErr, "take telemetry lock")
		} else if created {
			return func() { releaseLock(path, token) }, nil
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > stale {
			if takeOver(path, token, info.ModTime(), stale) {
				return func() { releaseLock(path, token) }, nil
			}
			continue
		}
		if !time.Now().Before(deadline) {
			return nil, ErrBusy
		}
		time.Sleep(time.Millisecond)
	}
}

func newLockToken() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(raw[:])
}

// createLock creates path exclusively holding token. created is false when it exists.
func createLock(path, token string) (created bool, err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // machine-local lock file
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err //nolint:wrapcheck // the caller adds the path
	}
	_, writeErr := file.WriteString(token)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		_ = os.Remove(path) //nolint:errcheck // do not leave a lock without an owner token
		return false, err   //nolint:wrapcheck // the caller adds the path
	}
	return true, nil
}

// releaseLock removes path only while it still holds token.
func releaseLock(path, token string) {
	if data, err := os.ReadFile(path); err == nil && string(data) == token { //nolint:gosec // machine-local lock file
		_ = os.Remove(path) //nolint:errcheck // best-effort unlock
	}
}

// takeOver replaces the stale lock (last modified at seen) with one holding
// token, serialised by the guard file. It reports whether the lock is now ours.
func takeOver(path, token string, seen time.Time, stale time.Duration) bool {
	guard := path + ".takeover"
	if created, err := createLock(guard, token); err != nil || !created {
		if info, statErr := os.Stat(guard); statErr == nil && time.Since(info.ModTime()) > stale {
			_ = os.Remove(guard) //nolint:errcheck // a guard left by a crash; the next round retries
		}
		return false
	}
	defer releaseLock(guard, token)
	// Re-check under the guard: another process may already have replaced the lock.
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(seen) {
		return false
	}
	_ = os.Remove(path) //nolint:errcheck // checked just above, under the guard
	created, err := createLock(path, token)
	return err == nil && created
}
