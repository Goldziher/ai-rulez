package tagresolve

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var gateNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// fakeTimer answers from a map of tag -> age before gateNow; a missing tag fails.
type fakeTimer struct {
	ages  map[string]time.Duration
	calls []string
}

func (f *fakeTimer) ReleaseTime(_ context.Context, tag RawTag) (ReleaseTime, error) {
	f.calls = append(f.calls, tag.Name)
	age, ok := f.ages[tag.Name]
	if !ok {
		return ReleaseTime{}, errors.New("no release time")
	}
	return ReleaseTime{At: gateNow.Add(-age), From: SourceForge}, nil
}

const day = 24 * time.Hour

func heldTags(sel *Selection) []string {
	var held []string
	for _, h := range sel.Held {
		held = append(held, h.Tag)
	}
	return held
}

func TestSelectGated(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		ages      map[string]time.Duration
		min       time.Duration
		pinned    string
		wantTag   string
		wantHeld  []string
		wantErr   string
		wantCalls []string
	}{
		{
			name: "the newest tag is held back and the next one is chosen",
			tags: []string{"v1.2.4", "v1.3.0", "v1.3.1"}, min: 7 * day,
			ages:    map[string]time.Duration{"v1.3.1": 3 * day, "v1.3.0": 10 * day},
			wantTag: "v1.3.0", wantHeld: []string{"v1.3.1"}, wantCalls: []string{"v1.3.1", "v1.3.0"},
		},
		{
			name: "a tag exactly as old as the minimum passes",
			tags: []string{"v1.0.0", "v1.1.0"}, min: 7 * day,
			ages:    map[string]time.Duration{"v1.1.0": 7 * day},
			wantTag: "v1.1.0",
		},
		{
			name: "the pinned tag is exempt, so a gate never rolls a pin back",
			tags: []string{"v1.2.4", "v1.3.0", "v1.3.1"}, min: 7 * day, pinned: "v1.2.4",
			ages:    map[string]time.Duration{"v1.3.1": day, "v1.3.0": 2 * day},
			wantTag: "v1.2.4", wantHeld: []string{"v1.3.1", "v1.3.0"}, wantCalls: []string{"v1.3.1", "v1.3.0"},
		},
		{
			name: "every tag held back and no pin is AR730",
			tags: []string{"v1.0.0", "v1.1.0"}, min: 7 * day,
			ages:    map[string]time.Duration{"v1.1.0": day, "v1.0.0": 2 * day},
			wantErr: "AR730",
		},
		{
			name: "a release time that cannot be found holds the tag back (fail closed)",
			tags: []string{"v1.0.0", "v1.1.0"}, min: 7 * day,
			ages:    map[string]time.Duration{"v1.0.0": 30 * day},
			wantTag: "v1.0.0", wantHeld: []string{"v1.1.0"},
		},
		{
			name: "a release in the future (clock skew) is held back with age zero",
			tags: []string{"v1.0.0", "v1.1.0"}, min: 7 * day,
			ages:    map[string]time.Duration{"v1.1.0": -2 * day, "v1.0.0": 30 * day},
			wantTag: "v1.0.0", wantHeld: []string{"v1.1.0"},
		},
		{
			name: "no minimum means no lookup at all",
			tags: []string{"v1.0.0", "v1.1.0"}, min: 0,
			wantTag: "v1.1.0", wantCalls: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			timer := &fakeTimer{ages: tt.ages}
			gate := &AgeGate{Min: tt.min, Now: gateNow, Timer: timer}

			// Act
			sel, err := SelectGated(context.Background(), rawTags(tt.tags...), Spec{Constraint: "^1", Pinned: tt.pinned}, gate)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), "min_release_age")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTag, sel.Chosen.Tag.Name)
			assert.Equal(t, tt.wantHeld, heldTags(sel))
			if tt.wantCalls != nil || tt.min == 0 {
				assert.Equal(t, tt.wantCalls, timer.calls)
			}
		})
	}
}

func TestSelectGatedHeldDetails(t *testing.T) {
	timer := &fakeTimer{ages: map[string]time.Duration{"v1.1.0": 3*day + time.Hour, "v1.0.0": 30 * day}}
	gate := &AgeGate{Min: 7 * day, Now: gateNow, Timer: timer}

	sel, err := SelectGated(context.Background(), rawTags("v1.0.0", "v1.1.0"), Spec{Constraint: "^1"}, gate)

	require.NoError(t, err)
	require.Len(t, sel.Held, 1)
	h := sel.Held[0]
	assert.Equal(t, 3, h.AgeDays)
	assert.Equal(t, SourceForge, h.From)
	assert.True(t, h.EligibleAt.Equal(gateNow.Add(-(3*day + time.Hour)).Add(7*day)))
	assert.Equal(t, "AR733 v1.1.0 held back: released 3 days ago (forge)", h.String())
	require.NotNil(t, sel.Release, "the chosen tag's release time is kept for the lock")
	assert.Equal(t, SourceForge, sel.Release.From)
	assert.Equal(t, "v1.1.0", sel.Latest.Tag.Name, "latest ignores the gate")
}

func TestSelectGatedLookupCap(t *testing.T) {
	var names []string
	ages := map[string]time.Duration{}
	for i := 0; i < maxGateLookups+10; i++ {
		n := "v1.0." + strconv.Itoa(i)
		names = append(names, n)
		ages[n] = day
	}
	timer := &fakeTimer{ages: ages}

	_, err := SelectGated(context.Background(), rawTags(names...), Spec{Constraint: "^1"}, &AgeGate{Min: 7 * day, Now: gateNow, Timer: timer})

	require.Error(t, err)
	assert.Len(t, timer.calls, maxGateLookups, "release time lookups are bounded")
}

func TestEvaluateGated(t *testing.T) {
	want := lockfile.Want{Kind: "include", Name: "shared", Source: "https://x/y", Constraint: "^1", Ref: "^1"}
	entry := &lockfile.Entry{Name: "shared", Tag: "v1.2.4", Commit: shaA}
	tags := []RawTag{{Name: "v1.2.4", Object: shaA, Commit: shaA}, {Name: "v1.3.0", Object: shaB, Commit: shaB}, {Name: "v1.3.1", Object: shaC, Commit: shaC}}
	tests := []struct {
		name       string
		ages       map[string]time.Duration
		wantStatus string
		wantAllow  string
		wantHeld   int
	}{
		{"everything newer is held back: up to date", map[string]time.Duration{"v1.3.1": day, "v1.3.0": 2 * day}, StatusUpToDate, "v1.2.4", 2},
		{"the newest is held back, the next is allowed", map[string]time.Duration{"v1.3.1": day, "v1.3.0": 20 * day}, StatusUpdatable, "v1.3.0", 1},
		{"nothing held back", map[string]time.Duration{"v1.3.1": 20 * day}, StatusUpdatable, "v1.3.1", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := &AgeGate{Min: 7 * day, Now: gateNow, Timer: &fakeTimer{ages: tt.ages}}

			row := EvaluateGated(context.Background(), want, entry, tags, gate)

			assert.Equal(t, tt.wantStatus, row.Status)
			assert.Equal(t, tt.wantAllow, row.Allowed.Tag)
			assert.Len(t, row.Held, tt.wantHeld)
			assert.Equal(t, "v1.3.1", row.Latest.Tag)
		})
	}
	t.Run("report counts and renders the held tags", func(t *testing.T) {
		gate := &AgeGate{Min: 7 * day, Now: gateNow, Timer: &fakeTimer{ages: map[string]time.Duration{"v1.3.1": day, "v1.3.0": day}}}
		row := EvaluateGated(context.Background(), want, entry, tags, gate)
		rep := NewReport([]Row{row})
		assert.Equal(t, 1, rep.Summary.HeldBack)
		var out strings.Builder
		require.NoError(t, rep.WriteText(&out))
		assert.Contains(t, out.String(), "AR733 v1.3.1 held back")
		assert.Contains(t, out.String(), "1 held back")
	})
}

func TestHeldJSON(t *testing.T) {
	known := Held{Tag: "v1.3.1", Commit: shaA, Released: gateNow.Add(-3 * day), From: SourceForge, AgeDays: 3, EligibleAt: gateNow.Add(4 * day)}
	unknown := Held{Tag: "v1.3.2", Reason: "no release time"}

	k, err := json.Marshal(known)
	require.NoError(t, err)
	u, err := json.Marshal(unknown)
	require.NoError(t, err)

	assert.JSONEq(t, `{"tag":"v1.3.1","commit":"`+shaA+`","released":"2026-10-03T12:00:00Z","released_from":"forge","age_days":3,"eligible_at":"2026-10-10T12:00:00Z"}`, string(k))
	assert.JSONEq(t, `{"tag":"v1.3.2","age_days":0,"reason":"no release time"}`, string(u))
}

func TestMarkOutdated(t *testing.T) {
	rows := func() []Row {
		return []Row{
			{Kind: "include", Name: "a", Status: StatusUpdatable},
			{Kind: "include", Name: "b", Status: StatusUpToDate},
			{Kind: "include", Name: "c", Status: StatusTagMoved, Code: CodeTagMoved},
		}
	}
	tests := []struct {
		name        string
		severity    string
		wantCode    string
		wantErrors  int
		wantFailing bool
	}{
		{"off", "off", "", 1, true},
		{"empty", "", "", 1, true},
		{"unknown value", "loud", "", 1, true},
		{"info", "info", CodeOutdated, 1, true},
		{"warning", "warning", CodeOutdated, 1, true},
		{"error adds an error", "error", CodeOutdated, 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep := NewReport(rows())

			rep.MarkOutdated(tt.severity)

			assert.Equal(t, tt.wantCode, rep.Sources[0].Code)
			assert.Equal(t, "", rep.Sources[1].Code, "an up to date source is never a finding")
			assert.Equal(t, CodeTagMoved, rep.Sources[2].Code, "an existing code is kept")
			assert.Equal(t, tt.wantErrors, rep.Summary.Errors)
			assert.Equal(t, tt.wantFailing, rep.Failing())
		})
	}
	t.Run("only an error severity fails an otherwise clean report", func(t *testing.T) {
		clean := func() *Report { return NewReport([]Row{{Name: "a", Status: StatusUpdatable}}) }
		warn, fail := clean(), clean()
		warn.MarkOutdated("warning")
		fail.MarkOutdated("error")
		assert.False(t, warn.Failing())
		assert.True(t, fail.Failing())
	})
}
