package includes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestDiscoverSkillDirsWarnsWhenTheWalkHitsItsCap(t *testing.T) {
	// Arrange: more skills than the walk collects; the one the entry selects sorts last
	root := t.TempDir()
	writeNumberedSkills(t, root, maxSkillHits+1)
	rec := &testutil.LogRecorder{}
	ctx := logger.WithContext(t.Context(), rec)
	// Act
	dirs, err := discoverSkillDirs(ctx, []string{root}, []string{fmt.Sprintf("s%04d", maxSkillHits)}, nil)
	// Assert
	require.NoError(t, err)
	assert.Empty(t, dirs, "the walk stopped before the selected skill")
	warns := rec.Level("WARN")
	require.Len(t, warns, 1, "the truncation is reported, not silent: %v", warns)
	assert.Contains(t, warns[0], fmt.Sprint(maxSkillHits))
}
