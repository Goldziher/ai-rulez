package usage

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockLog_SharedLocksCoexistAndExcludeAPrune(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		holder    bool // true: exclusive
		taker     bool
		wantError bool
	}{
		{"two appenders share the lock", false, false, false},
		{"a prune waits for an appender", false, true, true},
		{"an appender waits for a prune", true, false, true},
		{"two prunes exclude each other", true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeLog(t, line(1, "a"))
			release, err := lockLog(path, tt.holder, 0)
			require.NoError(t, err)
			defer release()

			// Act
			second, err := lockLog(path, tt.taker, 30*time.Millisecond)

			// Assert
			if tt.wantError {
				require.ErrorIs(t, err, ErrLogLocked)
				return
			}
			require.NoError(t, err)
			second()
		})
	}
}

func TestLockLog_ReleaseFreesTheLock(t *testing.T) {
	t.Parallel()
	path := writeLog(t, line(1, "a"))
	release, err := lockLog(path, true, 0)
	require.NoError(t, err)

	release()
	again, err := lockLog(path, true, 0)

	require.NoError(t, err)
	again()
}

func TestPruneLog_WaitsForAnAppenderThenRewrites(t *testing.T) {
	t.Parallel()
	// Arrange: an appender holds the shared lock while it writes
	path := writeLog(t, line(60, "old"), line(1, "fresh"))
	release, err := lockLog(path, false, 0)
	require.NoError(t, err)
	done := make(chan error, 1)

	// Act
	go func() {
		_, err := PruneLog(path, PruneOptions{Cutoff: pruneNow.AddDate(0, 0, -30)})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("the prune ran while an appender held the lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	release()

	// Assert
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the prune never ran after the lock was released")
	}
	got, err := os.ReadFile(path) //nolint:gosec // a test path
	require.NoError(t, err)
	assert.Equal(t, line(1, "fresh")+"\n", string(got))
}

func TestPruneLog_GivesUpWhenTheLogStaysLocked(t *testing.T) {
	// Arrange (not parallel: it shortens the shared wait)
	old := pruneLockWait
	pruneLockWait = 40 * time.Millisecond
	t.Cleanup(func() { pruneLockWait = old })
	path := writeLog(t, line(60, "old"), line(1, "fresh"))
	release, err := lockLog(path, false, 0)
	require.NoError(t, err)
	defer release()

	// Act
	_, err = PruneLog(path, PruneOptions{Cutoff: pruneNow.AddDate(0, 0, -30)})

	// Assert
	require.ErrorIs(t, err, ErrLogLocked)
	got, readErr := os.ReadFile(path) //nolint:gosec // a test path
	require.NoError(t, readErr)
	assert.Contains(t, string(got), `"id":"old"`, "a refused prune rewrites nothing")
}

func TestAppendLine_StillWritesWhenAPruneHoldsTheLockTooLong(t *testing.T) {
	// Arrange (not parallel: it shortens the shared wait)
	old := appendLockWait
	appendLockWait = 40 * time.Millisecond
	t.Cleanup(func() { appendLockWait = old })
	path := writeLog(t, line(1, "a"))
	release, err := lockLog(path, true, 0)
	require.NoError(t, err)
	defer release()

	// Act: a hook must not stall or drop the event behind a stuck prune
	err = appendLine(path, []byte(line(0, "late")+"\n"))

	// Assert
	require.NoError(t, err)
	got, readErr := os.ReadFile(path) //nolint:gosec // a test path
	require.NoError(t, readErr)
	assert.Contains(t, string(got), `"id":"late"`)
}

func TestPruneLog_NeverDropsALineAppendedDuringThePrune(t *testing.T) {
	t.Parallel()
	// Arrange: a log with old lines, appenders racing repeated prunes
	var seed []string
	for i := range 200 {
		seed = append(seed, line(60, fmt.Sprintf("old%d", i)))
	}
	path := writeLog(t, seed...)
	const writers, perWriter = 4, 60
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Act
	var pruneErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := PruneLog(path, PruneOptions{Cutoff: pruneNow.AddDate(0, 0, -30)}); err != nil {
				pruneErr = err
				return
			}
		}
	}()
	var writeWG sync.WaitGroup
	for w := range writers {
		writeWG.Add(1)
		go func() {
			defer writeWG.Done()
			for i := range perWriter {
				id := fmt.Sprintf("new-%d-%d", w, i)
				if err := appendLine(path, []byte(line(0, id)+"\n")); err != nil {
					t.Errorf("append %s: %v", id, err)
				}
			}
		}()
	}
	writeWG.Wait()
	close(stop)
	wg.Wait()
	_, finalErr := PruneLog(path, PruneOptions{Cutoff: pruneNow.AddDate(0, 0, -30)})

	// Assert: every appended line survived every prune, and the old ones are gone
	require.NoError(t, pruneErr)
	require.NoError(t, finalErr)
	got, err := os.ReadFile(path) //nolint:gosec // a test path
	require.NoError(t, err)
	text := string(got)
	for w := range writers {
		for i := range perWriter {
			assert.Contains(t, text, fmt.Sprintf(`"id":"new-%d-%d"`, w, i), "line lost")
		}
	}
	assert.False(t, strings.Contains(text, `"id":"old`), "old lines should be pruned")
}
