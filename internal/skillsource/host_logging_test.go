package skillsource

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestDiscoverReportsToTheLoggerOfItsContext(t *testing.T) {
	// Arrange: a source whose only skill dir is a symlink, discovered by two runs.
	root := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "stolen/SKILL.md", skillMD("stolen", "Outside the source"))
	write(t, root, "ok/SKILL.md", skillMD("ok", "Inside the source"))
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "stolen"), filepath.Join(root, "link"))
	recs := [2]*testutil.LogRecorder{{}, {}}

	// Act
	for _, rec := range recs {
		skills, err := Discover(logger.WithContext(t.Context(), rec), Spec{Name: "s"}, root)
		require.NoError(t, err)
		require.Len(t, skills, 1)
	}

	// Assert
	for i, rec := range recs {
		got := rec.Level("WARN")
		require.Len(t, got, 1, "run %d: %v", i, got)
		assert.Contains(t, got[0], "Skipping a symlink in a skill source")
	}
}
