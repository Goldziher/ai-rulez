package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncSink_RecordServedDoesNotWaitForASlowSink(t *testing.T) {
	// Arrange: a sink that outlives the 3s timeout would block a synchronous caller.
	sink := NewAsyncSink("sleep 30", 4, nil)
	defer sink.Close(0)
	opts := RecordOptions{SinkCommand: "sleep 30", AsyncSink: sink, SaltPath: filepath.Join(t.TempDir(), "salt")}

	// Act
	start := time.Now()
	_, err := RecordServed(ServedLoad{Skill: "kit", Session: "s"}, opts)

	// Assert
	require.NoError(t, err)
	assert.Less(t, time.Since(start), time.Second, "load latency must not depend on the sink")
}

func TestAsyncSink_DeliversTheSameLineAsTheLog(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	logPath, out := filepath.Join(dir, "usage.jsonl"), filepath.Join(dir, "sink.out")
	sink := NewAsyncSink("cat >> '"+out+"'", 4, nil)
	opts := RecordOptions{LogPath: logPath, SinkCommand: "cat >> '" + out + "'", AsyncSink: sink}

	// Act
	_, err := RecordServed(ServedLoad{Skill: "kit", Session: "s"}, opts)
	require.NoError(t, err)
	sink.Close(5 * time.Second)

	// Assert
	logged, err := os.ReadFile(logPath)
	require.NoError(t, err)
	sunk, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, string(logged), string(sunk))
	assert.Contains(t, string(sunk), `"session":`)
}

func TestAsyncSink_DropsAndCountsWhenTheQueueIsFull(t *testing.T) {
	// Arrange: the worker blocks on the first line; the queue holds two more.
	sink := NewAsyncSink("sleep 30", 2, nil)
	defer sink.Close(0)

	// Act
	for range 10 {
		sink.Send([]byte("x\n"))
	}

	// Assert: one in flight at most, two queued, the rest dropped.
	assert.GreaterOrEqual(t, sink.Dropped(), int64(7))
}
