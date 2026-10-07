package telemetry

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEvent(i int) *Event {
	return &Event{Version: 1, Name: EventItem, Time: "2026-10-05T09:12:44Z", EventID: fmt.Sprintf("%016x", i), Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}
}

func TestSpool_AppendPendingRemove(t *testing.T) {
	s := &Spool{Dir: filepath.Join(t.TempDir(), "local")}
	for i := 0; i < 5; i++ {
		require.NoError(t, s.Append(newTestEvent(i)))
	}
	info, err := os.Stat(s.outbox())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	events, corrupt, err := s.Pending()
	require.NoError(t, err)
	assert.Len(t, events, 5)
	assert.Zero(t, corrupt)

	require.NoError(t, s.Remove(map[string]bool{events[0].EventID: true, events[1].EventID: true}))
	events, _, err = s.Pending()
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, fmt.Sprintf("%016x", 2), events[0].EventID, "removal is by id and keeps order")
	_, statErr := os.Stat(filepath.Join(s.Dir, lockFileName))
	assert.True(t, os.IsNotExist(statErr), "the lock is released")
}

func TestSpool_BoundedDropsOldest(t *testing.T) {
	s := &Spool{Dir: t.TempDir(), MaxEvents: 20}
	for i := 0; i < 400; i++ {
		require.NoError(t, s.Append(newTestEvent(i)))
	}
	events, _, err := s.Pending()
	require.NoError(t, err)
	assert.LessOrEqual(t, s.Size(), int64(20*bytesPerEvent)+1024, "the outbox stays within its byte bound")
	assert.Less(t, len(events), 400)
	assert.Equal(t, fmt.Sprintf("%016x", 399), events[len(events)-1].EventID, "the newest event survives")
	assert.NotEqual(t, fmt.Sprintf("%016x", 0), events[0].EventID, "the oldest events were dropped")
	assert.Positive(t, s.ReadState().Dropped)
}

func TestSpool_CorruptLinesAreDroppedOnRewrite(t *testing.T) {
	s := &Spool{Dir: t.TempDir()}
	require.NoError(t, s.Append(newTestEvent(1)))
	f, err := os.OpenFile(s.outbox(), os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	_, corrupt, err := s.Pending()
	require.NoError(t, err)
	assert.Equal(t, 1, corrupt)
	require.NoError(t, s.Remove(map[string]bool{}))
	_, corrupt, _ = s.Pending()
	assert.Zero(t, corrupt)
}

func TestSpool_ConcurrentAppendsLoseNothing(t *testing.T) {
	s := &Spool{Dir: t.TempDir()}
	var wg sync.WaitGroup
	var mu sync.Mutex
	busy := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for n := 0; n < 10; n++ {
				if err := s.Append(newTestEvent(base*100 + n)); err != nil {
					mu.Lock()
					busy++
					mu.Unlock()
				}
			}
		}(i)
	}
	wg.Wait()
	events, corrupt, err := s.Pending()
	require.NoError(t, err)
	assert.Zero(t, corrupt, "no interleaved lines")
	assert.Equal(t, 80-busy, len(events), "every append either landed or reported busy")
}

func TestLock_StaleLockIsTakenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "l")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	old := time.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(path, old, old))
	release, err := lock(path, 0, staleLock)
	require.NoError(t, err)
	release()

	held, err := lock(path, 0, staleLock)
	require.NoError(t, err)
	defer held()
	_, err = lock(path, 0, staleLock)
	assert.ErrorIs(t, err, ErrBusy)
}

func TestLock_ReleaseRemovesOnlyItsOwnLock(t *testing.T) {
	// Arrange: holder A's lock goes stale and B takes it over
	path := filepath.Join(t.TempDir(), "l")
	releaseA, err := lock(path, 0, time.Hour)
	require.NoError(t, err)
	old := time.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(path, old, old))
	releaseB, err := lock(path, 0, time.Second)
	require.NoError(t, err)

	// Act: A, which outlived its lock, releases
	releaseA()

	// Assert: B's lock survives and still excludes others
	_, err = lock(path, 0, time.Second)
	require.ErrorIs(t, err, ErrBusy, "A's release must not delete B's lock")
	releaseB()
	releaseC, err := lock(path, 0, time.Second)
	require.NoError(t, err)
	releaseC()
}

func TestLock_StaleLockThatCannotBeTakenOverHonoursTheWait(t *testing.T) {
	// RV-LLM-18: a failed takeover skipped the deadline and the sleep, so lock
	// spun at full CPU for as long as the takeover kept failing.
	tests := []struct {
		name    string
		arrange func(t *testing.T, path string)
		wantErr error
		notErr  error
	}{
		{"a directory at the lock path is refused", func(t *testing.T, path string) {
			require.NoError(t, os.MkdirAll(filepath.Join(path, "x"), 0o700))
		}, nil, ErrBusy},
		{"a takeover guard held by a live process", func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path+".takeover", []byte("other"), 0o600))
			require.NoError(t, os.WriteFile(path, []byte("crashed"), 0o600))
		}, ErrBusy, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), ".telemetry.lock")
			tt.arrange(t, path)
			old := time.Now().Add(-time.Hour)
			require.NoError(t, os.Chtimes(path, old, old))
			done := make(chan error, 1)
			// Act
			go func() {
				release, err := lock(path, 25*time.Millisecond, time.Minute)
				if release != nil {
					release()
				}
				done <- err
			}()
			// Assert
			select {
			case err := <-done:
				require.Error(t, err)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				}
				if tt.notErr != nil {
					require.NotErrorIs(t, err, tt.notErr, "a lock path that is not a file is an error, not a busy lock")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("lock(wait=25ms) was still looping after 3s")
			}
		})
	}
}

func TestLock_ConcurrentTakeoverOfAStaleLockIsExclusive(t *testing.T) {
	for round := 0; round < 20; round++ {
		path := filepath.Join(t.TempDir(), "l")
		require.NoError(t, os.WriteFile(path, []byte("crashed"), 0o600))
		old := time.Now().Add(-time.Minute)
		require.NoError(t, os.Chtimes(path, old, old))

		var holders, maxHolders, acquired atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				release, err := lock(path, 10*time.Second, time.Second)
				if err != nil {
					t.Errorf("lock: %v", err)
					return
				}
				acquired.Add(1)
				n := holders.Add(1)
				for {
					m := maxHolders.Load()
					if n <= m || maxHolders.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				holders.Add(-1)
				release()
			}()
		}
		close(start)
		wg.Wait()
		require.EqualValues(t, 12, acquired.Load())
		require.EqualValues(t, 1, maxHolders.Load(), "two processes held the lock at once (round %d)", round)
	}
}

func TestFlushLockOutlastsTheLongestFlush(t *testing.T) {
	assert.Greater(t, staleFlushLock, MaxFlushTimeout, "a running flush must never look stale")
	assert.Less(t, FlushTimeout, MaxFlushTimeout)
}

func TestSpawnDue(t *testing.T) {
	s := &Spool{Dir: t.TempDir()}
	now := time.Now()
	assert.False(t, s.SpawnDue(now, time.Minute), "an empty outbox never spawns")
	require.NoError(t, s.Append(newTestEvent(1)))
	assert.True(t, s.SpawnDue(now, time.Minute), "never flushed: due")
	assert.False(t, s.SpawnDue(now.Add(time.Second), time.Minute), "rate limited to one spawn per minute")
	require.NoError(t, s.UpdateState(func(st *State) { st.LastFlush = FormatTime(now.Add(5 * time.Minute)) }))
	assert.False(t, s.SpawnDue(now.Add(5*time.Minute+2*time.Minute), time.Hour), "recently flushed and small: not due")
}
