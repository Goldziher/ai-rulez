package repoindex

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// buildGuardBound is the time a cold Build of the 5000-file, 100-scope
// monorepo may take. A build measures 70-250 ms here (docs/performance.md); the
// bound is several times that and never under two seconds, so only a change
// that makes the build scale badly, not a slow machine, trips it.
const buildGuardBound = 2 * time.Second

// TestBuildOfAMonorepoStaysFast guards the cold build against quadratic work in
// file classification, scope grouping or name handling.
func TestBuildOfAMonorepoStaysFast(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("performance guard")
	}
	root := testutil.BuildMonorepo(t, testutil.MonorepoSpec{Files: 5000, Scopes: 100, Git: true})

	start := time.Now()
	ix, err := Build(context.Background(), Options{Root: root, Presets: []string{"claude"}})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, ix.Items(), 300, "an AGENTS.md, a skill and a rule per scope")
	t.Logf("5000 files, 100 scopes: %s", elapsed)
	if elapsed > buildGuardBound {
		t.Errorf("Build took %s, over the %s guard", elapsed, buildGuardBound)
	}
}
