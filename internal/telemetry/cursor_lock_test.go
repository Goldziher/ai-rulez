package telemetry

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatchUp_ConcurrentCallsQueueEachEventOnce(t *testing.T) {
	// Arrange: a cursor at the start of a log with events waiting.
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, true))
	appendLog(t, log, 0, 200)

	// Act: several catch-ups race, like a background flush and an export.
	const workers = 8
	var wg sync.WaitGroup
	queued := make([]int, workers)
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := spool.CatchUp(log, CatchUpOptions{})
			queued[i], errs[i] = res.Queued, err
		}()
	}
	wg.Wait()

	// Assert: every event is in the outbox exactly once.
	total := 0
	for i := range workers {
		require.NoError(t, errs[i])
		total += queued[i]
	}
	assert.Equal(t, 200, total)
	pending, _, err := spool.Pending()
	require.NoError(t, err)
	seen := map[string]bool{}
	for i := range pending {
		assert.False(t, seen[pending[i].EventID], "event %s queued twice", pending[i].EventID)
		seen[pending[i].EventID] = true
	}
	assert.Len(t, pending, 200)
}

func TestSpool_PruneLog_IgnoreCursorKeepsTheCursorOnTheSameLine(t *testing.T) {
	// Arrange: lines 0,1 old, 2,3 fresh; the cursor has passed lines 0-2.
	spool, log := newCursorFixture(t)
	writeTimedLog(t, log, 90, 80, 2, 1)
	_, err := spool.CatchUp(log, CatchUpOptions{All: true, Max: 3})
	require.NoError(t, err)
	require.NoError(t, spool.Remove(map[string]bool{ids2(0): true, ids2(1): true, ids2(2): true}))

	// Act
	res, err := spool.PruneLog(log, PruneOptions{Cutoff: fixedNow.AddDate(0, 0, -30), IgnoreCursor: true})

	// Assert: the offset moves by the removed bytes, so the next catch-up reads only line 3.
	require.NoError(t, err)
	require.Equal(t, 2, res.Removed)
	next, err := spool.CatchUp(log, CatchUpOptions{})
	require.NoError(t, err)
	assert.False(t, next.Reset)
	assert.Equal(t, []string{ids2(3)}, ids(next.Events), "already read events are not queued again")
}

func TestSpool_PruneLog_WaitsForTheSpoolLock(t *testing.T) {
	// Arrange: another process holds the spool lock.
	spool, log := newCursorFixture(t)
	writeTimedLog(t, log, 90, 1)
	require.NoError(t, spool.PlaceCursor(log, false))
	release, err := lock(filepath.Join(spool.Dir, lockFileName), time.Second, staleLock)
	require.NoError(t, err)

	// Act
	var finished atomic.Bool
	done := make(chan error, 1)
	go func() {
		_, pruneErr := spool.PruneLog(log, PruneOptions{Cutoff: fixedNow.AddDate(0, 0, -30)})
		finished.Store(true)
		done <- pruneErr
	}()
	time.Sleep(200 * time.Millisecond)
	before := finished.Load()
	release()

	// Assert
	require.NoError(t, <-done)
	assert.False(t, before, "the prune must not rewrite the log while the lock is held")
}

func TestSpool_PruneLog_CursorPlacedBeforeTheLogExistedProtectsEverything(t *testing.T) {
	spool, log := newCursorFixture(t)
	require.NoError(t, spool.PlaceCursor(log, false)) // no log yet: LogID "" offset 0
	writeTimedLog(t, log, 90, 1)

	res, err := spool.PruneLog(log, PruneOptions{Cutoff: fixedNow.AddDate(0, 0, -30)})

	require.NoError(t, err, "an unplaced cursor must not wedge prune")
	assert.Zero(t, res.Removed, "nothing was exported yet, so nothing old may go")
	assert.Equal(t, 1, res.Protected)
}
