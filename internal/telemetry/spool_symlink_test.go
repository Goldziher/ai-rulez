package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func plantLink(t *testing.T, link string) (victim string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on windows")
	}
	victim = filepath.Join(t.TempDir(), "victim")
	require.NoError(t, os.WriteFile(victim, []byte("keep\n"), 0o600))
	testutil.SymlinkOrSkip(t, victim, link)
	return victim
}

func TestSpool_RefusesSymlinkedOutbox(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ai-rulez", "local")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	victim := plantLink(t, filepath.Join(dir, OutboxFileName))
	spool := &Spool{Dir: dir}

	err := spool.Append(newTestEvent(1))

	require.ErrorContains(t, err, "refusing")
	data, _ := os.ReadFile(victim) //nolint:errcheck // test
	assert.Equal(t, "keep\n", string(data))
}

func TestJSONL_RefusesSymlinkedLog(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ai-rulez", "local")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	victim := plantLink(t, filepath.Join(dir, "usage.jsonl"))

	err := JSONL{Path: filepath.Join(dir, "usage.jsonl")}.Emit(context.Background(), newTestEvent(1))

	require.ErrorContains(t, err, "refusing")
	data, _ := os.ReadFile(victim) //nolint:errcheck // test
	assert.Equal(t, "keep\n", string(data))
}

func TestSpawnDue_DoesNotWriteThroughASymlinkedMarker(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ai-rulez", "local")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	victim := plantLink(t, filepath.Join(dir, spawnMarker))
	spool := &Spool{Dir: dir}
	require.NoError(t, spool.Append(newTestEvent(1)))

	spool.SpawnDue(time.Now().Add(time.Hour), time.Minute)

	data, _ := os.ReadFile(victim) //nolint:errcheck // test
	assert.Equal(t, "keep\n", string(data), "the marker must not truncate the link target")
}
