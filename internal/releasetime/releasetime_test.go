package releasetime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	now    = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	commit = strings.Repeat("a", 40)
	other  = strings.Repeat("b", 40)
	tag    = tagresolve.RawTag{Name: "v1.2.3", Object: commit, Commit: commit}
)

const gh = "https://github.com/o/r"

func storeIn(t *testing.T) (*SeenStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	return OpenSeenStore(path), path
}

func TestSeenStoreFirstSeen(t *testing.T) {
	// Arrange
	store, path := storeIn(t)
	t1, t2 := now, now.Add(48*time.Hour)

	// Act
	first, isNew, err := store.FirstSeen(gh, "v1.2.3", commit, t1)
	require.NoError(t, err)
	again, againNew, err := store.FirstSeen(gh, "v1.2.3", commit, t2)
	require.NoError(t, err)
	moved, movedNew, err := store.FirstSeen(gh, "v1.2.3", other, t2)
	require.NoError(t, err)
	reopened, reopenedNew, err := OpenSeenStore(path).FirstSeen(gh, "v1.2.3", commit, t2)
	require.NoError(t, err)

	// Assert
	assert.True(t, isNew)
	assert.True(t, first.Equal(t1))
	assert.False(t, againNew, "the second sighting keeps the first time")
	assert.True(t, again.Equal(t1))
	assert.True(t, movedNew, "a tag that moved to another commit is a new release")
	assert.True(t, moved.Equal(t2))
	assert.False(t, reopenedNew, "the record survives a restart")
	assert.True(t, reopened.Equal(t1))
}

func TestSeenStoreObserveRecordsOnlyNewTags(t *testing.T) {
	store, path := storeIn(t)
	tags := []tagresolve.RawTag{{Name: "v1", Commit: commit}, {Name: "v2", Commit: commit}}

	require.NoError(t, store.Observe(gh, tags[:1], now))
	require.NoError(t, store.Observe(gh, tags, now.Add(24*time.Hour)))

	v1, _, err := store.FirstSeen(gh, "v1", commit, now.Add(72*time.Hour))
	require.NoError(t, err)
	v2, _, err := store.FirstSeen(gh, "v2", commit, now.Add(72*time.Hour))
	require.NoError(t, err)
	assert.True(t, v1.Equal(now))
	assert.True(t, v2.Equal(now.Add(24*time.Hour)))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `tag = 'v1'`)
}

func TestSeenStoreBadFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"not toml", "this is [not toml", "parse the first-seen record"},
		{"too large", strings.Repeat("# pad\n", maxSeenBytes/6+10), "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, path := storeIn(t)
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			_, _, err := store.FirstSeen(gh, "v1", commit, now)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			raw, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.content, string(raw), "a file that cannot be read is never overwritten")
		})
	}
}

func TestSeenStoreIsBounded(t *testing.T) {
	store, path := storeIn(t)
	for src := 0; src < maxSeenEntries/maxObservePerSource+3; src++ {
		var tags []tagresolve.RawTag
		for i := 0; i < maxObservePerSource; i++ {
			tags = append(tags, tagresolve.RawTag{Name: fmt.Sprintf("v0.0.%d", i), Commit: commit})
		}
		require.NoError(t, store.Observe(fmt.Sprintf("%s%d", gh, src), tags, now.Add(time.Duration(src)*time.Hour)))
	}

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, maxSeenEntries, strings.Count(string(raw), "[[tag]]"))
}

func forgeWith(published time.Time) *forge.Fake {
	return &forge.Fake{
		ReleasesBy: map[string][]forge.Release{"github.com/o/r": {{Tag: "v1.2.3", Published: published, Commit: commit}}},
		Tags:       map[string]forge.TagInfo{"github.com/o/r@v1.2.3": {Name: "v1.2.3", Commit: commit}},
	}
}

func TestTimerModes(t *testing.T) {
	published := now.Add(-5 * 24 * time.Hour)
	commitDate := now.Add(-100 * 24 * time.Hour)
	commitFn := func(context.Context, tagresolve.RawTag) (time.Time, error) { return commitDate, nil }
	failCommit := func(context.Context, tagresolve.RawTag) (time.Time, error) {
		return time.Time{}, errors.New("cannot fetch")
	}
	prior := func(t *testing.T) *SeenStore {
		store, _ := storeIn(t)
		_, _, err := store.FirstSeen(gh, "v1.2.3", commit, now.Add(-30*24*time.Hour))
		require.NoError(t, err)
		return store
	}
	brokenStore := func(t *testing.T) *SeenStore {
		_, path := storeIn(t)
		require.NoError(t, os.WriteFile(path, []byte("[[tag"), 0o600))
		return OpenSeenStore(path)
	}
	fresh := func(t *testing.T) *SeenStore { s, _ := storeIn(t); return s }

	tests := []struct {
		name     string
		mode     string
		source   string
		forge    forge.Client
		store    func(*testing.T) *SeenStore
		commit   CommitDater
		wantFrom string
		wantAt   time.Time
		wantErr  string
	}{
		{"auto prefers the forge", "", gh, forgeWith(published), prior, commitFn, tagresolve.SourceForge, published, ""},
		{"auto falls to first-seen when the tag has no release", "auto", gh, &forge.Fake{}, prior, commitFn, tagresolve.SourceFirstSeen, now.Add(-30 * 24 * time.Hour), ""},
		{"auto: a tag never seen is seen now, never waved through on a forgeable commit date", "auto", gh, &forge.Fake{}, fresh, commitFn, tagresolve.SourceFirstSeen, now, ""},
		{"auto without a forge client uses first-seen", "auto", gh, nil, prior, commitFn, tagresolve.SourceFirstSeen, now.Add(-30 * 24 * time.Hour), ""},
		{"auto on a non-forge source skips the forge (and records a new source as seen now)", "auto", "file:///srv/x.git", forgeWith(published), prior, commitFn, tagresolve.SourceFirstSeen, now, ""},
		{"auto offline forge degrades to first-seen", "auto", gh, &forge.Fake{Err: forge.ErrOffline}, prior, commitFn, tagresolve.SourceFirstSeen, now.Add(-30 * 24 * time.Hour), ""},
		{"auto never uses the forgeable commit date, even when the record cannot be kept", "auto", gh, &forge.Fake{}, brokenStore, commitFn, "", time.Time{}, "no release time for v1.2.3"},
		{"auto without a first-seen record fails closed", "auto", gh, &forge.Fake{}, nil, commitFn, "", time.Time{}, "no release time for v1.2.3"},
		{"auto with nothing available fails closed", "auto", gh, &forge.Fake{}, brokenStore, failCommit, "", time.Time{}, "no release time for v1.2.3"},
		{"forge only: no release is an error", "forge", gh, &forge.Fake{}, prior, commitFn, "", time.Time{}, "forge release time"},
		{"forge only", "forge", gh, forgeWith(published), nil, nil, tagresolve.SourceForge, published, ""},
		{"first-seen only", "first-seen", gh, forgeWith(published), prior, commitFn, tagresolve.SourceFirstSeen, now.Add(-30 * 24 * time.Hour), ""},
		{"commit only", "commit", gh, forgeWith(published), prior, commitFn, tagresolve.SourceCommit, commitDate, ""},
		{"commit only without a way to read it", "commit", gh, nil, nil, nil, "", time.Time{}, "no way to read the commit date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var store *SeenStore
			if tt.store != nil {
				store = tt.store(t)
			}
			timer := New(Options{Mode: tt.mode, Source: tt.source, Forge: tt.forge, Seen: store, Commit: tt.commit, Clock: ambient.Fixed(now)})

			// Act
			got, err := timer.ReleaseTime(context.Background(), tag)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFrom, got.From)
			assert.True(t, got.At.Equal(tt.wantAt), "got %s want %s", got.At, tt.wantAt)
		})
	}
}

func TestTimerMemoizesPerTag(t *testing.T) {
	f := forgeWith(now.Add(-24 * time.Hour))
	timer := New(Options{Source: gh, Forge: f, Clock: ambient.Fixed(now)})

	for i := 0; i < 3; i++ {
		_, err := timer.ReleaseTime(context.Background(), tag)
		require.NoError(t, err)
	}

	assert.Len(t, f.Calls(), 2, "one release and one tag call per tag per run")
}

func TestTimerUsesTheConfigModeNames(t *testing.T) {
	assert.Equal(t, config.AgeSourceForge, tagresolve.SourceForge)
	assert.Equal(t, config.AgeSourceFirstSeen, tagresolve.SourceFirstSeen)
	assert.Equal(t, config.AgeSourceCommit, tagresolve.SourceCommit)
}

func TestTimerObserveFeedsFirstSeen(t *testing.T) {
	store, _ := storeIn(t)
	timer := New(Options{Mode: config.AgeSourceFirstSeen, Source: gh, Seen: store, Clock: ambient.Fixed(now)})
	timer.Observe([]tagresolve.RawTag{tag})
	later := New(Options{Mode: config.AgeSourceFirstSeen, Source: gh, Seen: store, Clock: ambient.Fixed(now.Add(10 * 24 * time.Hour))})

	got, err := later.ReleaseTime(context.Background(), tag)

	require.NoError(t, err)
	assert.True(t, got.At.Equal(now), "the earlier observation is the release time")
}

// TestTimerForgeRefusesAMovedTag models a force-pushed release tag the way a
// real forge reports it (RV-GOV-1): the release keeps its publish time, and the
// forge's tag lookup and the remote both report the NEW commit.
func TestTimerForgeRefusesAMovedTag(t *testing.T) {
	published := now.Add(-90 * 24 * time.Hour)
	movedTag := tagresolve.RawTag{Name: "v1.2.3", Object: other, Commit: other}
	forgeAt := func(releasedAt string) *forge.Fake {
		return &forge.Fake{
			ReleasesBy: map[string][]forge.Release{"github.com/o/r": {{Tag: "v1.2.3", Published: published, Commit: releasedAt}}},
			Tags:       map[string]forge.TagInfo{"github.com/o/r@v1.2.3": {Name: "v1.2.3", Commit: other}},
		}
	}
	seenAt := func(at time.Time) func(*testing.T) *SeenStore {
		return func(t *testing.T) *SeenStore {
			store, _ := storeIn(t)
			_, _, err := store.FirstSeen(gh, "v1.2.3", other, at)
			require.NoError(t, err)
			return store
		}
	}
	fresh := func(t *testing.T) *SeenStore { s, _ := storeIn(t); return s }
	tests := []struct {
		name     string
		mode     string
		released string
		store    func(*testing.T) *SeenStore
		wantErr  string
		wantFrom string
		wantAt   time.Time
	}{
		{name: "forge: the release recorded another commit", mode: "forge", released: commit, store: fresh, wantErr: "moved"},
		{name: "auto: falls to first-seen, the new commit is seen now", mode: "auto", released: commit, store: fresh, wantFrom: tagresolve.SourceFirstSeen, wantAt: now},
		{name: "forge: no recorded commit, the commit is first seen now", mode: "forge", store: fresh, wantFrom: tagresolve.SourceFirstSeen, wantAt: now},
		{name: "auto: no recorded commit, the commit is first seen now", mode: "auto", store: fresh, wantFrom: tagresolve.SourceFirstSeen, wantAt: now},
		{name: "forge: no recorded commit and no first-seen record fails closed", mode: "forge", wantErr: "does not record its commit"},
		{name: "forge: commit seen after the release counts from then", mode: "forge", store: seenAt(now.Add(-2 * 24 * time.Hour)), wantFrom: tagresolve.SourceFirstSeen, wantAt: now.Add(-2 * 24 * time.Hour)},
		{name: "forge: commit seen before the release counts from the release", mode: "forge", store: seenAt(now.Add(-100 * 24 * time.Hour)), wantFrom: tagresolve.SourceForge, wantAt: published},
		{name: "forge: the release recorded this commit", mode: "forge", released: other, wantFrom: tagresolve.SourceForge, wantAt: published},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var store *SeenStore
			if tt.store != nil {
				store = tt.store(t)
			}
			timer := New(Options{Mode: tt.mode, Source: gh, Forge: forgeAt(tt.released), Seen: store, Clock: ambient.Fixed(now)})

			// Act
			got, err := timer.ReleaseTime(context.Background(), movedTag)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFrom, got.From)
			assert.True(t, got.At.Equal(tt.wantAt), "got %s want %s", got.At, tt.wantAt)
		})
	}
}

func TestTimerForgeRefusesARemoteThatDisagreesWithTheForge(t *testing.T) {
	// Arrange: a mirror serving another commit than the forge has for the tag.
	f := forgeWith(now.Add(-90 * 24 * time.Hour))
	f.Tags["github.com/o/r@v1.2.3"] = forge.TagInfo{Name: "v1.2.3", Commit: other}
	timer := New(Options{Mode: "forge", Source: gh, Forge: f, Clock: ambient.Fixed(now)})

	// Act
	_, err := timer.ReleaseTime(context.Background(), tag)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the forge has commit")
}

func TestTimerForgeFailsClosedWhenTheTagCannotBeChecked(t *testing.T) {
	f := &forge.Fake{ReleasesBy: map[string][]forge.Release{"github.com/o/r": {{Tag: "v1.2.3", Published: now.Add(-90 * 24 * time.Hour)}}}}
	timer := New(Options{Mode: "forge", Source: gh, Forge: f, Clock: ambient.Fixed(now)})

	_, err := timer.ReleaseTime(context.Background(), tag)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag")
}

func TestSeenStoreStaysUnderTheReadCap(t *testing.T) {
	// Arrange: more long junk tags than fit in the 1 MiB a reader accepts.
	store, path := storeIn(t)
	pad := strings.Repeat("x", 200)

	// Act
	for src := 0; src < 6; src++ {
		var tags []tagresolve.RawTag
		for i := 0; i < maxObservePerSource; i++ {
			tags = append(tags, tagresolve.RawTag{Name: fmt.Sprintf("junk-%06d-%s", i, pad), Commit: commit})
		}
		require.NoError(t, store.Observe(fmt.Sprintf("%s%d", gh, src), tags, now.Add(time.Duration(src)*time.Hour)))
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	_, _, reopenErr := OpenSeenStore(path).FirstSeen(gh, "v1.2.3", commit, now)

	// Assert
	assert.LessOrEqual(t, len(raw), maxSeenBytes, "the file stays readable by safefs.ReadRegular")
	assert.NoError(t, reopenErr, "a flood of tags never makes the record unreadable")
}

func TestSeenStoreObserveCapsTagsPerSource(t *testing.T) {
	store, _ := storeIn(t)
	var tags []tagresolve.RawTag
	for i := 0; i < maxObservePerSource+50; i++ {
		tags = append(tags, tagresolve.RawTag{Name: fmt.Sprintf("v0.0.%d", i), Commit: commit})
	}

	require.NoError(t, store.Observe(gh, tags, now))

	assert.Len(t, store.by, maxObservePerSource)
}
