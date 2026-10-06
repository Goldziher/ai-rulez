package govview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

func TestFilterChanges(t *testing.T) {
	all := func() *contentlock.Diff {
		return &contentlock.Diff{Changes: []contentlock.Change{
			{Scope: contentlock.ScopeSource, Kind: "rule", ID: "a"},
			{Scope: contentlock.ScopeOutput, Path: "CLAUDE.md"},
			{Scope: contentlock.ScopeRemote, Kind: "include", ID: "shared"},
			{Scope: contentlock.ScopeRemote, Kind: "source", ID: "skills"},
			{Scope: contentlock.ScopeServed, Kind: "served", ID: "deploy"},
		}}
	}
	tests := []struct {
		kind    string
		want    int
		wantErr bool
	}{
		{"", 5, false},
		{"content", 2, false},
		{"include", 1, false},
		{"source", 1, false},
		{"served", 1, false},
		{"skill", 0, false},
		{"bogus", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			// Arrange
			diff := all()

			// Act
			err := FilterChanges(diff, tt.kind)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, diff.Changes, tt.want)
		})
	}
}
