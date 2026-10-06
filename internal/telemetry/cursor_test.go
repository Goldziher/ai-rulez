package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	res, err := spool.CatchUp(log, CatchUpOptions{Sample: 0.0001})
	require.NoError(t, err)
	assert.Less(t, res.Queued, 5)
	assert.Equal(t, 50, res.Queued+res.Skipped)
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
