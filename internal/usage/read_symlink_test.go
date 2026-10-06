package usage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadLogs_RefuseSymlinks(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	require.NoError(t, os.WriteFile(target, []byte("{}\n"), 0o600))
	link := filepath.Join(dir, "usage.jsonl")
	require.NoError(t, os.Symlink(target, link))

	// Act
	_, _, logErr := ReadLog(link)
	_, _, feedbackErr := ReadFeedback(link)

	// Assert
	require.ErrorContains(t, logErr, "symlink")
	require.ErrorContains(t, feedbackErr, "symlink")
}
