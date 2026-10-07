package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logEvent(i int) Event {
	return Event{Version: 1, Name: EventItem, Time: "2026-10-05T09:12:44Z", EventID: fmt.Sprintf("%016x", i), Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}
}

// appendLog appends item events to the usage log the way the JSONL emitter does.
func appendLog(t *testing.T, path string, from, to int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // test file
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck // test
	for i := from; i < to; i++ {
		e := logEvent(i)
		line, err := json.Marshal(&e)
		require.NoError(t, err)
		_, err = f.Write(append(line, '\n'))
		require.NoError(t, err)
	}
}

func ids(events []Event) []string {
	var out []string
	for i := range events {
		out = append(out, events[i].EventID)
	}
	return out
}

func newCursorFixture(t *testing.T) (*Spool, string) {
	t.Helper()
	dir := t.TempDir()
	return &Spool{Dir: dir}, filepath.Join(dir, "usage.jsonl")
}

func TestCatchUp_FirstRunPlacesTheCursorAtTheEndAndExportsNoHistory(t *testing.T) {
	spool, log := newCursorFixture(t)
	appendLog(t, log, 0, 5)

	first, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.True(t, first.Initialized)
	assert.Zero(t, first.Queued, "history from before export was on is never sent by itself")

	appendLog(t, log, 5, 8)
	second, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{ids2(5), ids2(6), ids2(7)}, ids(second.Events))
	pending, _, err := spool.Pending()
	require.NoError(t, err)
	assert.Len(t, pending, 3)

	third, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.Zero(t, third.Queued, "a second run finds nothing new")
}

func ids2(i int) string { return fmt.Sprintf("%016x", i) }

func TestCatchUp_AllExportsTheHistory(t *testing.T) {
	spool, log := newCursorFixture(t)
	appendLog(t, log, 0, 4)
	res, err := spool.CatchUp(log, CatchUpOptions{All: true})
	require.NoError(t, err)
	assert.Equal(t, 4, res.Queued)
}

func TestCatchUp_SkipsEventsTheOutboxHoldsOrWasDelivered(t *testing.T) {
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, false))
	appendLog(t, log, 0, 6)
	inOutbox := logEvent(1)
	require.NoError(t, spool.Append(&inOutbox))
	require.NoError(t, spool.MarkSent([]string{ids2(2), ids2(3)}, "2026-10-05T09:00:00Z"))

	res, err := spool.CatchUp(log, CatchUpOptions{})

	require.NoError(t, err)
	assert.Equal(t, []string{ids2(0), ids2(4), ids2(5)}, ids(res.Events))
	assert.Equal(t, 3, res.Skipped)
}

func TestCatchUp_DoesNotReadAPartialLastLine(t *testing.T) {
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, false))
	appendLog(t, log, 0, 2)
	f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test file
	require.NoError(t, err)
	_, err = f.WriteString(`{"v":1,"event":"item_event","event_id":"00000000000000ff","kind":"rule","id":"half`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	res, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Queued, "the half-written line waits for its newline")

	f, err = os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test file
	require.NoError(t, err)
	_, err = f.WriteString(`","source":"hook","outcome":"loaded"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	res, err = spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Queued)
}

func TestCatchUp_BoundsEachRunAndResumes(t *testing.T) {
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, true))
	appendLog(t, log, 0, 7)

	first, err := spool.CatchUp(log, CatchUpOptions{Max: 3})
	require.NoError(t, err)
	assert.Equal(t, 3, first.Queued)
	assert.True(t, first.More)

	rest, err := spool.CatchUp(log, CatchUpOptions{Max: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{ids2(3), ids2(4), ids2(5), ids2(6)}, ids(rest.Events))
	assert.False(t, rest.More)
}

func TestCatchUp_ARotatedLogIsReadFromTheStartWithoutResendingDelivered(t *testing.T) {
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, true))
	appendLog(t, log, 0, 3)
	_, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	require.NoError(t, spool.Remove(map[string]bool{ids2(0): true, ids2(1): true, ids2(2): true}))
	require.NoError(t, spool.MarkSent([]string{ids2(0), ids2(1), ids2(2)}, ""))

	// The log is replaced: new first line, shorter than the old offset.
	require.NoError(t, os.Remove(log))
	appendLog(t, log, 2, 5)

	res, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.True(t, res.Reset)
	assert.Equal(t, []string{ids2(3), ids2(4)}, ids(res.Events), "event 2 was delivered before the rotation")
}

func TestCatchUp_DryRunChangesNothing(t *testing.T) {
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, true))
	appendLog(t, log, 0, 3)
	before := spool.ReadCursor()

	res, err := spool.CatchUp(log, CatchUpOptions{DryRun: true})

	require.NoError(t, err)
	assert.Equal(t, 3, res.Queued)
	assert.Equal(t, before, spool.ReadCursor())
	pending, _, _ := spool.Pending()
	assert.Empty(t, pending)
}

func TestCatchUp_SamplingAppliesToTheLog(t *testing.T) {
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, true))
	appendLog(t, log, 0, 50)
	res, err := spool.CatchUp(log, CatchUpOptions{Sample: ptrFloat(0.0001)})
	require.NoError(t, err)
	assert.Less(t, res.Queued, 5)
	assert.Equal(t, 50, res.Queued+res.Skipped)
}

func TestCatchUp_SampleZeroExportsNothing(t *testing.T) {
	// RV-LLM-20: sample = 0 records nothing, so catch-up must export nothing
	// either; only an unset sample (a count, not an export) reads every event.
	tests := []struct {
		name       string
		sample     *float64
		wantQueued int
	}{
		{"sample 0", ptrFloat(0), 0},
		{"sample 1", ptrFloat(1), 20},
		{"no sampling", nil, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			spool, log := newCursorFixture(t)
			require.NoError(t, spool.PlaceCursor(log, true))
			appendLog(t, log, 0, 20)
			// Act
			res, err := spool.CatchUp(log, CatchUpOptions{Sample: tt.sample})
			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantQueued, res.Queued)
			assert.Equal(t, 20, res.Queued+res.Skipped)
		})
	}
}

// appendLogAt appends item events recorded at the given time.
func appendLogAt(t *testing.T, path string, from, to int, at time.Time) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // test file
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck // test
	for i := from; i < to; i++ {
		e := logEvent(i)
		e.Time = FormatTime(at)
		line, err := json.Marshal(&e)
		require.NoError(t, err)
		_, err = f.Write(append(line, '\n'))
		require.NoError(t, err)
	}
}

func TestCatchUp_ConsentIsNotRetroactive(t *testing.T) {
	// RV-LLM-29: project B kept its cursor and kept recording while consent was
	// off; re-enabling consent elsewhere must not export that gap.
	granted := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	tests := []struct {
		name       string
		placedAt   string // "" keeps the cursor as written before placed_at existed
		backfill   bool
		all        bool
		grantedAt  time.Time
		wantQueued []string
	}{
		{"old cursor drops the gap", "", false, false, granted, []string{fmt.Sprintf("%016x", 3), fmt.Sprintf("%016x", 4)}},
		{"cursor placed before the grant drops the gap", FormatTime(granted.Add(-24 * time.Hour)), false, false, granted,
			[]string{fmt.Sprintf("%016x", 3), fmt.Sprintf("%016x", 4)}},
		{"--backfill under this consent exports history", "", true, false, granted,
			[]string{fmt.Sprintf("%016x", 0), fmt.Sprintf("%016x", 1), fmt.Sprintf("%016x", 2), fmt.Sprintf("%016x", 3), fmt.Sprintf("%016x", 4)}},
		{"--all exports history", "", false, true, granted,
			[]string{fmt.Sprintf("%016x", 0), fmt.Sprintf("%016x", 1), fmt.Sprintf("%016x", 2), fmt.Sprintf("%016x", 3), fmt.Sprintf("%016x", 4)}},
		{"no consent record keeps every event", "", false, false, time.Time{},
			[]string{fmt.Sprintf("%016x", 0), fmt.Sprintf("%016x", 1), fmt.Sprintf("%016x", 2), fmt.Sprintf("%016x", 3), fmt.Sprintf("%016x", 4)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: a cursor at the start of the log, three events recorded
			// while consent was off and two after it was granted.
			spool, log := newCursorFixture(t)
			appendLogAt(t, log, 0, 3, granted.Add(-30*time.Minute))
			appendLogAt(t, log, 3, 5, granted.Add(time.Minute))
			if tt.backfill {
				require.NoError(t, spool.PlaceCursor(log, true))
			} else {
				id, _, err := LogID(log)
				require.NoError(t, err)
				require.NoError(t, spool.UpdateCursor(func(c *Cursor) { c.LogID, c.Offset, c.PlacedAt = id, 0, tt.placedAt }))
			}
			// Act
			res, err := spool.CatchUp(log, CatchUpOptions{GrantedAt: tt.grantedAt, All: tt.all})
			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantQueued, ids(res.Events))
			assert.Equal(t, 5, res.Queued+res.Skipped)
		})
	}
}

func TestFlush_QueuesNoEventRecordedBeforeTheConsent(t *testing.T) {
	// Arrange: an old cursor and events recorded before the consent record's grant.
	granted := time.Now().UTC().Truncate(time.Second)
	spool, log := newCursorFixture(t)
	appendLogAt(t, log, 0, 3, granted.Add(-time.Hour))
	id, _, err := LogID(log)
	require.NoError(t, err)
	require.NoError(t, spool.UpdateCursor(func(c *Cursor) { c.LogID, c.Offset = id, 0 }))
	settings := Settings{Sample: 1, ConsentState: ConsentRecord, Consent: &Consent{GrantedAt: FormatTime(granted)}}
	p := &Pipeline{Spool: spool, LogPath: log, Settings: settings, Exporter: &Exporter{Spool: spool}}
	// Act
	_, _ = p.Flush(context.Background()) //nolint:errcheck // the exporter has no endpoint; the outbox is what counts
	// Assert
	pending, _, err := spool.Pending()
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestSettings_ConsentGrantedAtOnlyForARecordGrant(t *testing.T) {
	at := "2026-10-01T10:00:00Z"
	tests := []struct {
		name  string
		s     Settings
		wantZ bool
	}{
		{"record grant", Settings{ConsentState: ConsentRecord, Consent: &Consent{GrantedAt: at}}, false},
		{"allow_network grant", Settings{ConsentState: ConsentConfig, Consent: &Consent{GrantedAt: at}}, true},
		{"no record", Settings{ConsentState: ConsentRecord}, true},
		{"unreadable time", Settings{ConsentState: ConsentRecord, Consent: &Consent{GrantedAt: "yesterday"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantZ, tt.s.ConsentGrantedAt().IsZero())
		})
	}
}

func TestFlush_SampleZeroSendsNoCatchUpEvents(t *testing.T) {
	// Arrange: a pipeline whose settings say sample = 0, and logged events past the cursor.
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, true))
	appendLog(t, log, 0, 5)
	p := &Pipeline{Spool: spool, LogPath: log, Settings: Settings{Sample: 0}, Exporter: &Exporter{Spool: spool}}
	// Act
	_, _ = p.Flush(context.Background()) //nolint:errcheck // the exporter has no endpoint; the outbox is what counts
	// Assert
	pending, _, err := spool.Pending()
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestCatchUp_MissingLogIsNotAnError(t *testing.T) {
	spool, log := newCursorFixture(t)
	res, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.Equal(t, CatchUpResult{}, res)
}

func TestPlaceCursor_AtEndOrStart(t *testing.T) {
	spool, log := newCursorFixture(t)
	appendLog(t, log, 0, 3)
	require.NoError(t, spool.PlaceCursor(log, false))
	info, err := os.Stat(log)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), spool.ReadCursor().Offset)
	require.NoError(t, spool.PlaceCursor(log, true))
	assert.Zero(t, spool.ReadCursor().Offset)
	assert.NotEmpty(t, spool.ReadCursor().LogID)
}

func TestExporter_FlushRemembersDeliveredIDsSoACatchUpDoesNotResend(t *testing.T) {
	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	x, spool, _ := newExporter(t, srv.URL)
	log := filepath.Join(spool.Dir, "usage.jsonl")
	require.NoError(t, spool.PlaceCursor(log, false))
	appendLog(t, log, 0, 3)
	for i := 0; i < 3; i++ { // what a hook does: the event goes to the log and the outbox
		e := logEvent(i)
		require.NoError(t, spool.Append(&e))
	}

	_, err := x.Flush(context.Background())
	require.NoError(t, err)
	res, err := spool.CatchUp(log, CatchUpOptions{})

	require.NoError(t, err)
	assert.Zero(t, res.Queued)
	assert.Equal(t, 3, res.Skipped, "delivered events are not delivered a second time from the log")
	assert.Len(t, spool.ReadCursor().Sent, 3)
}

func TestSentRingIsBounded(t *testing.T) {
	spool, _ := newCursorFixture(t)
	batch := make([]string, SentRingMax+10)
	for i := range batch {
		batch[i] = ids2(i)
	}
	require.NoError(t, spool.MarkSent(batch, ""))
	got := spool.ReadCursor().Sent
	assert.Len(t, got, SentRingMax)
	assert.Equal(t, ids2(10), got[0], "the oldest ids are dropped first")
}

func TestExporter_FailuresAreCountedAndResetOnSuccess(t *testing.T) {
	c := &collector{statuses: []int{503, 503, 503, 503, 200}}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	x, spool, _ := newExporter(t, srv.URL)
	x.Retries = 1
	fill(t, spool, 1)

	for i := 0; i < 2; i++ {
		_, err := x.Flush(context.Background())
		require.Error(t, err)
	}
	st := spool.ReadState()
	assert.Equal(t, int64(2), st.Failures)
	assert.Equal(t, int64(2), st.ConsecutiveFailures)

	_, err := x.Flush(context.Background())
	require.NoError(t, err)
	st = spool.ReadState()
	assert.Equal(t, int64(2), st.Failures, "the total is kept")
	assert.Zero(t, st.ConsecutiveFailures)
}
