package pinupdate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

func TestPlan_MovedTagIsNeverHiddenByDowngrade(t *testing.T) {
	moved := tagresolve.Row{Kind: "include", Name: "a", Status: tagresolve.StatusTagMoved, Downgrade: true, Locked: &tagresolve.TagRef{Tag: "v1.2.0"}}
	nonVersion := tagresolve.Row{Kind: "include", Name: "a", Status: tagresolve.StatusLockedNonVersion}
	tests := []struct {
		name          string
		row           tagresolve.Row
		opt           Options
		wantBlocked   bool
		wantMove      bool
		wantUnchanged bool
	}{
		{"moved and downgrading is blocked", moved, Options{}, true, false, false},
		{"moved and downgrading moves only with both flags", moved, Options{AcceptMoved: true, AllowDowngrade: true}, false, true, false},
		{"moved and downgrading with only accept-moved stays unchanged", moved, Options{AcceptMoved: true}, false, false, true},
		{"a non-version pin needs consent", nonVersion, Options{}, false, false, true},
		{"a non-version pin moves with consent", nonVersion, Options{AllowDowngrade: true}, false, true, false},
		{"an unsatisfiable constraint is blocked", tagresolve.Row{Kind: "include", Name: "a", Status: tagresolve.StatusUnsatisfied}, Options{AcceptMoved: true, AllowDowngrade: true}, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rep := NewReport(false)

			// Act
			Plan(rep, []Source{{}}, []tagresolve.Row{tt.row}, tt.opt)

			// Assert
			assert.Equal(t, tt.wantBlocked, len(rep.Blocked) == 1)
			assert.Equal(t, tt.wantMove, len(rep.Moves) == 1)
			assert.Equal(t, tt.wantUnchanged, len(rep.Unchanged) == 1)
			assert.Equal(t, tt.wantBlocked, rep.Refused())
		})
	}
}

func TestReportRefused_ScanFindingsRefuse(t *testing.T) {
	rep := NewReport(true)
	rep.Updates = append(rep.Updates, Item{Kind: "include", Name: "a", Scan: &ScanSummary{Errors: 1, Refused: true}})

	assert.True(t, rep.Refused())
	rep.Updates[0].Scan.Refused = false
	assert.False(t, rep.Refused())
}

func TestRefreshFilterMatchesKindAndName(t *testing.T) {
	// Arrange: an include and an installed skill share the name "shared"; only the include moves.
	moves := map[string]*MoveTo{Key(lockfile.KindInclude, "shared"): {Row: tagresolve.Row{Kind: lockfile.KindInclude, Name: "shared"}}}
	tests := []struct {
		kind, name string
		want       bool
	}{
		{lockfile.KindInclude, "shared", true},
		{lockfile.KindSkill, "shared", false},
		{lockfile.KindSource, "shared", false},
		{lockfile.KindInclude, "other", false},
	}
	filter := RefreshFilter(moves)

	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, filter(tt.kind, tt.name))
		})
	}
}

func TestRequireOnline(t *testing.T) {
	assert.NoError(t, RequireOnline("update", false))
	assert.ErrorContains(t, RequireOnline("update", true), "cannot run with --offline")
}
