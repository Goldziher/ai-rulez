package telemetry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpool_SymlinkedFilesAreNotFollowed(t *testing.T) {
	// Arrange: a committed .ai-rulez/local/outbox symlink to a file that holds
	// something that looks like an event.
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere.jsonl")
	require.NoError(t, os.WriteFile(target, []byte(`{"event_id":"leak","name":"item"}`+"\n"), 0o600))
	s := &Spool{Dir: filepath.Join(dir, "local")}
	require.NoError(t, os.MkdirAll(s.Dir, 0o750))
	require.NoError(t, os.Symlink(target, s.outbox()))
	require.NoError(t, os.Symlink(target, s.statePath()))

	// Act
	events, _, pendingErr := s.Pending()
	size := s.Size()
	state := s.ReadState()

	// Assert
	require.Error(t, pendingErr)
	assert.Contains(t, pendingErr.Error(), "symlink")
	assert.Empty(t, events)
	assert.Zero(t, size, "a link is not an outbox")
	assert.Equal(t, State{}, state)
}
