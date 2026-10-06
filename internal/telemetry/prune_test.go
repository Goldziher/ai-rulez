package telemetry

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func timedEvent(i, daysAgo int) Event {
	e := logEvent(i)
	e.Time = fixedNow.AddDate(0, 0, -daysAgo).Format(time.RFC3339)
	return e
}

func writeTimedLog(t *testing.T, path string, ages ...int) {
	t.Helper()
	var sb strings.Builder
	for i, age := range ages {
		e := timedEvent(i, age)
		fmt.Fprintf(&sb, `{"v":1,"event":"item_event","ts":%q,"event_id":%q,"kind":"rule","id":"r","source":"hook","outcome":"loaded"}`+"\n", e.Time, e.EventID)
	}
	require.NoError(t, os.WriteFile(path, []byte(sb.String()), 0o600))
}

func TestSpool_PruneLog_OnlyRemovesLinesBehindTheCursor(t *testing.T) {
	// Arrange: lines 0,1 old and exported; line 2 old but not yet exported; line 3 fresh.
	spool, log := newCursorFixture(t)
	writeTimedLog(t, log, 90, 80, 70, 1)
	_, err := spool.CatchUp(log, CatchUpOptions{All: true, Max: 2})
	require.NoError(t, err)
	cutoff := fixedNow.AddDate(0, 0, -30)

	// Act
	res, err := spool.PruneLog(log, PruneOptions{Cutoff: cutoff})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, res.Removed)
	assert.Equal(t, 1, res.Protected, "the old line the cursor has not passed is kept")
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(data), "\n"))

	// The cursor still points at the first unexported line, so a catch-up queues it.
	next, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{ids2(2), ids2(3)}, ids(next.Events))
	assert.False(t, next.Reset, "the cursor was moved to the rewritten log, not left stale")
}

func TestSpool_PruneLog_WithoutACursorAgeDecides(t *testing.T) {
	spool, log := newCursorFixture(t)
	writeTimedLog(t, log, 90, 1)

	res, err := spool.PruneLog(log, PruneOptions{Cutoff: fixedNow.AddDate(0, 0, -30)})

	require.NoError(t, err)
	assert.Equal(t, 1, res.Removed)
	assert.False(t, spool.ReadCursor().Set(), "pruning does not create a cursor")
}

func TestSpool_PruneLog_RefusesAMismatchedCursorUnlessIgnored(t *testing.T) {
	spool, log := newCursorFixture(t)
	writeTimedLog(t, log, 90, 1)
	require.NoError(t, spool.PlaceCursor(log, false))
	writeTimedLog(t, log, 91, 2) // the log was replaced since the cursor was placed
	cutoff := fixedNow.AddDate(0, 0, -30)

	_, err := spool.PruneLog(log, PruneOptions{Cutoff: cutoff})
	require.ErrorIs(t, err, ErrCursorMismatch)
	before, readErr := os.ReadFile(log)
	require.NoError(t, readErr)
	assert.Equal(t, 2, strings.Count(string(before), "\n"), "a refused prune changes nothing")

	res, err := spool.PruneLog(log, PruneOptions{Cutoff: cutoff, IgnoreCursor: true})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Removed)
	assert.Zero(t, spool.ReadCursor().Offset, "after an ignored cursor the offset restarts at the beginning")
}

func TestSpool_PruneLog_DryRunLeavesLogAndCursorAlone(t *testing.T) {
	spool, log := newCursorFixture(t)
	writeTimedLog(t, log, 90, 1)
	require.NoError(t, spool.PlaceCursor(log, false))
	cursor := spool.ReadCursor()
	before, err := os.ReadFile(log)
	require.NoError(t, err)

	res, err := spool.PruneLog(log, PruneOptions{Cutoff: fixedNow.AddDate(0, 0, -30), DryRun: true})

	require.NoError(t, err)
	assert.Equal(t, 1, res.Removed)
	after, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, cursor, spool.ReadCursor())
}
