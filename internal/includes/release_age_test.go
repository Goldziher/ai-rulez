package includes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ageNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// agesTimer reports a release time per tag name (ages before ageNow) and counts lookups.
type agesTimer struct {
	ages  map[string]time.Duration
	calls int
}

func (a *agesTimer) ReleaseTime(_ context.Context, tag tagresolve.RawTag) (tagresolve.ReleaseTime, error) {
	a.calls++
	age, ok := a.ages[tag.Name]
	if !ok {
		return tagresolve.ReleaseTime{}, errors.New("unknown")
	}
	return tagresolve.ReleaseTime{At: ageNow.Add(-age), From: tagresolve.SourceForge}, nil
}

func useAgeGate(t *testing.T, min time.Duration, timer *agesTimer) {
	t.Helper()
	ReleaseGate = func(lockfile.Want) *tagresolve.AgeGate {
		return &tagresolve.AgeGate{Min: min, Now: ageNow, Timer: timer}
	}
	t.Cleanup(func() { ReleaseGate = nil })
}

const dayAge = 24 * time.Hour

func TestReleaseAge_LockPinsTheNewestTagThatIsOldEnough(t *testing.T) {
	// Arrange: v1.1.0 is a day old, v1.0.0 a month.
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	timer := &agesTimer{ages: map[string]time.Duration{"v1.1.0": dayAge, "v1.0.0": 30 * dayAge}}
	useAgeGate(t, 7*dayAge, timer)

	// Act
	lock, problems := f.refresh(t, nil)

	// Assert
	require.Empty(t, problems)
	inc := lock.Find(lockfile.KindInclude, "shared")
	require.NotNil(t, inc)
	assert.Equal(t, "v1.0.0", inc.Tag, "v1.1.0 is held back (AR733)")
	assert.Equal(t, ageNow.Add(-30*dayAge).Format(time.RFC3339), inc.Released)
	assert.Equal(t, tagresolve.SourceForge, inc.ReleasedFrom)
}

func TestReleaseAge_NothingOldEnoughAndNoPinIsAR730(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	useAgeGate(t, 7*dayAge, &agesTimer{ages: map[string]time.Duration{"v1.1.0": dayAge, "v1.0.0": 2 * dayAge}})

	_, problems := f.refresh(t, nil)

	require.NotEmpty(t, problems)
	assert.Contains(t, problems[0], "AR730")
	assert.Contains(t, problems[0], "min_release_age")
}

func TestReleaseAge_LockKeepsASatisfiedPinWithoutAskingForTimes(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	first := f.refresh2(t) // no gate: pinned at v1.1.0
	timer := &agesTimer{ages: map[string]time.Duration{"v1.1.0": 30 * dayAge}}
	useAgeGate(t, 7*dayAge, timer)

	kept, problems := f.refresh(t, first)

	require.Empty(t, problems)
	assert.Equal(t, "v1.1.0", kept.Find(lockfile.KindInclude, "shared").Tag)
	assert.Zero(t, timer.calls, "a pin's age was decided when it was pinned")
}

func TestReleaseAge_UpdateMovesToTheNewestOldEnoughTagAndNeverBelowThePin(t *testing.T) {
	tests := []struct {
		name     string
		ages     map[string]time.Duration
		wantTag  string
		wantFrom string
	}{
		{"a newer tag is old enough", map[string]time.Duration{"v1.3.0": 20 * dayAge, "v1.2.0": 40 * dayAge}, "v1.3.0", tagresolve.SourceForge},
		{"the newest is held, the next one is old enough", map[string]time.Duration{"v1.3.0": dayAge, "v1.2.0": 20 * dayAge}, "v1.2.0", tagresolve.SourceForge},
		{"every newer tag is held: the pin stays", map[string]time.Duration{"v1.3.0": dayAge, "v1.2.0": 2 * dayAge}, "v1.1.0", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
			first := f.refresh2(t)
			f.release(t, "release 1.2", "v1.2.0", false)
			f.release(t, "release 1.3", "v1.3.0", false)
			useAgeGate(t, 7*dayAge, &agesTimer{ages: tt.ages})
			Advance = func(kind, _ string) bool { return kind == lockfile.KindInclude }

			// Act
			moved, problems := f.refresh(t, first)

			// Assert
			require.Empty(t, problems)
			inc := moved.Find(lockfile.KindInclude, "shared")
			assert.Equal(t, tt.wantTag, inc.Tag)
			assert.Equal(t, tt.wantFrom, inc.ReleasedFrom)
		})
	}
}

func TestReleaseAge_NoGateIsTheOldBehaviour(t *testing.T) {
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)

	lock, problems := f.refresh(t, nil)

	require.Empty(t, problems)
	inc := lock.Find(lockfile.KindInclude, "shared")
	assert.Equal(t, "v1.1.0", inc.Tag)
	assert.Empty(t, inc.Released, "no min_release_age, no release time recorded")
}
