package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

func TestUpdateRefreshFilterMatchesKindAndName(t *testing.T) {
	// Arrange: an include and an installed skill share the name "shared"; only the include moves.
	moves := map[string]*moveTo{
		moveKey(lockfile.KindInclude, "shared"): {row: tagresolve.Row{Kind: lockfile.KindInclude, Name: "shared"}},
	}
	tests := []struct {
		kind, name string
		want       bool
	}{
		{lockfile.KindInclude, "shared", true},
		{lockfile.KindSkill, "shared", false},
		{lockfile.KindSource, "shared", false},
		{lockfile.KindInclude, "other", false},
	}
	filter := updateRefreshFilter(moves)

	for _, tt := range tests {
		t.Run(tt.kind+"/"+tt.name, func(t *testing.T) {
			// Act and Assert
			assert.Equal(t, tt.want, filter(tt.kind, tt.name))
		})
	}
}
