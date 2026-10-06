package lockfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntryReleasedRoundTripsAndDoesNotAffectCovers(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	f := &File{Version: Version}
	f.Set(KindInclude, Entry{Name: "shared", Source: "https://x/y", Ref: "^1", Tag: "v1.2.4", Commit: "0f3e", Digest: "sha256:aa",
		Released: "2026-09-28T10:00:00Z", ReleasedFrom: "forge"})

	// Act
	require.NoError(t, Save(dir, f))
	got, err := Load(dir)
	require.NoError(t, err)

	// Assert
	e := got.Find(KindInclude, "shared")
	require.NotNil(t, e)
	assert.Equal(t, "2026-09-28T10:00:00Z", e.Released)
	assert.Equal(t, "forge", e.ReleasedFrom)
	want := Want{Kind: KindInclude, Name: "shared", Source: "https://x/y", Ref: "^1", Constraint: "^1", MinReleaseAge: "30d"}
	assert.True(t, e.Covers(want), "changing min_release_age never makes a pin stale")
	e.Released, e.ReleasedFrom = "", ""
	assert.True(t, e.Covers(want), "an entry without a release time is covered too")
}
