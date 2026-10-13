package commands

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

func TestForEachRoot_EmitsInInputOrderWhateverTheFinishOrder(t *testing.T) {
	// Arrange: the earlier a root is in the list, the longer it takes.
	const n = 24
	paths := make([]string, n)
	for i := range paths {
		paths[i] = fmt.Sprintf("root%02d", i)
	}
	var seen []string

	// Act
	forEachRoot(paths, func(p string) string {
		var i int
		_, err := fmt.Sscanf(p, "root%d", &i)
		assert.NoError(t, err)
		time.Sleep(time.Duration(n-i) * time.Millisecond)
		return "done " + p
	}, func(index int, path string, value string, _ time.Duration) {
		assert.Equal(t, len(seen), index, "emitted in order")
		assert.Equal(t, "done "+path, value)
		seen = append(seen, path)
	})

	// Assert
	assert.Equal(t, paths, seen)
}

func TestForEachRoot_RunsEachRootOnceWithinTheWorkerBound(t *testing.T) {
	// Arrange
	prev := runtime.GOMAXPROCS(3)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })
	var running, peak, runs atomic.Int32
	paths := make([]string, 30)
	for i := range paths {
		paths[i] = fmt.Sprint(i)
	}

	// Act
	total := 0
	forEachRoot(paths, func(string) int {
		cur := running.Add(1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		runs.Add(1)
		running.Add(-1)
		return 1
	}, func(_ int, _ string, v int, _ time.Duration) { total += v }) // emit is single-threaded: no lock

	// Assert
	assert.Equal(t, int32(30), runs.Load())
	assert.Equal(t, 30, total)
	assert.LessOrEqual(t, peak.Load(), int32(3), "never more workers than GOMAXPROCS")
}

func TestForEachRoot_NoPathsDoesNothing(t *testing.T) {
	forEachRoot(nil, func(string) int { t.Fatal("work called"); return 0 },
		func(int, string, int, time.Duration) { t.Fatal("emit called") })
}

func TestForEachRoot_ReportsElapsedTime(t *testing.T) {
	var got time.Duration
	forEachRoot([]string{"a"}, func(string) int { time.Sleep(5 * time.Millisecond); return 0 },
		func(_ int, _ string, _ int, elapsed time.Duration) { got = elapsed })
	assert.GreaterOrEqual(t, got, 5*time.Millisecond)
}

func TestReportRootProgress(t *testing.T) {
	tests := []struct {
		name  string
		quiet bool
		want  string
	}{
		{name: "printed", want: "[2/5] svc/.ai-rulez/config.toml (1.235s)\n"},
		{name: "suppressed when quiet", quiet: true, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress.SetQuiet(tt.quiet)
			t.Cleanup(func() { progress.SetQuiet(false) })

			stdout, stderr := capture(t, func() {
				reportRootProgress(1, 5, "svc/.ai-rulez/config.toml", 1234500*time.Microsecond)
			})

			require.Empty(t, stdout, "progress never lands on stdout")
			assert.Equal(t, tt.want, stderr)
		})
	}
}

func TestForEachRootBounded_RunsOneRootAtATimeWithALimitOfOne(t *testing.T) {
	// Arrange
	var running, peak atomic.Int32
	paths := []string{"a", "b", "c", "d", "e", "f"}

	// Act
	var order []string
	forEachRootBounded(1, paths, func(p string) string {
		cur := running.Add(1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		running.Add(-1)
		return p
	}, func(_ int, _ string, v string, _ time.Duration) { order = append(order, v) })

	// Assert
	assert.Equal(t, int32(1), peak.Load())
	assert.Equal(t, paths, order)
}

// The external scanners write to shared places (stdout in a dry run, one
// baseline file), so a recursive run with --external checks one root at a time.
func TestRecursiveValidateWorkers(t *testing.T) {
	tests := []struct {
		name   string
		extern bool
		want   int
	}{
		{"without --external the pool is GOMAXPROCS", false, runtime.GOMAXPROCS(0)},
		{"--external serialises the roots", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := validateExtern
			t.Cleanup(func() { validateExtern = old })
			validateExtern = tt.extern

			assert.Equal(t, tt.want, recursiveValidateWorkers())
		})
	}
}
