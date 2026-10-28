package lint

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExposure returns its pairs at or over the threshold it is asked for and records the threshold.
type fakeExposure struct {
	pairs []ExposureCollision
	asked []float64
}

func (f *fakeExposure) Collisions(minScore float64) []ExposureCollision {
	f.asked = append(f.asked, minScore)
	var out []ExposureCollision
	for i := range f.pairs {
		if f.pairs[i].Score >= minScore {
			out = append(out, f.pairs[i])
		}
	}
	return out
}

func collisionPairs() []ExposureCollision {
	return []ExposureCollision{
		{Score: 0.5, Shared: []string{"customer", "details", "look", "name"}, First: ".claude/skills/charge/SKILL.md", FirstKind: "skill", FirstName: "charge", FirstArea: "billing", Second: ".claude/skills/rank/SKILL.md", SecondKind: "skill", SecondName: "rank", SecondArea: "search"},
		{Score: 0.1, Shared: []string{"customer"}, First: ".claude/agents/finder.md", FirstKind: "agent", FirstName: "finder", FirstArea: "billing", Second: ".claude/skills/rank/SKILL.md", SecondKind: "skill", SecondName: "rank", SecondArea: "search"},
	}
}

func TestExposureAreaCollision(t *testing.T) {
	const areasTable = "[[exposure.areas]]\nname = \"billing\"\nterms = [\"invoice\"]\n[[exposure.areas]]\nname = \"search\"\nterms = [\"ranker\"]\n"
	charge := `skill "charge" of area "billing" and skill "rank" of area "search" list overlapping words at session start (score 0.50, at least %s): customer, details, look, name`
	finder := `agent "finder" of area "billing" and skill "rank" of area "search" list overlapping words at session start (score 0.10, at least 0.10): customer`
	tests := []struct {
		name      string
		config    string
		noSource  bool
		want      []string
		severity  Severity
		wantAsked []float64
	}{
		{name: "no exposure table is inert", config: ""},
		{name: "off without collision_min_score", config: "[exposure]\n" + areasTable},
		{name: "silent without an exposure source", config: "[exposure]\ncollision_min_score = 0.1\n" + areasTable, noSource: true},
		{
			name:      "a warning on the description line of each pair's first listing",
			config:    "[exposure]\ncollision_min_score = 0.1\n" + areasTable,
			want:      []string{".claude/agents/finder.md:3 " + finder, ".claude/skills/charge/SKILL.md:3 " + fmt.Sprintf(charge, "0.10")},
			severity:  SeverityWarning,
			wantAsked: []float64{0.1},
		},
		{
			name:      "the threshold is collision_min_score",
			config:    "[exposure]\ncollision_min_score = 0.3\n" + areasTable,
			want:      []string{".claude/skills/charge/SKILL.md:3 " + fmt.Sprintf(charge, "0.30")},
			severity:  SeverityWarning,
			wantAsked: []float64{0.3},
		},
		{
			name:      "lint.severity turns it on at the default threshold",
			config:    "[exposure]\n" + areasTable + "[lint.severity]\nAR9W2 = \"error\"\n",
			want:      []string{".claude/agents/finder.md:3 " + finder, ".claude/skills/charge/SKILL.md:3 " + fmt.Sprintf(charge, "0.10")},
			severity:  SeverityError,
			wantAsked: []float64{DefaultExposureCollisionMinScore},
		},
		{name: "lint.severity turns it off", config: "[exposure]\ncollision_min_score = 0.1\n" + areasTable + "[lint.severity]\nAR9W2 = \"off\"\n"},
		{name: "an invalid table leaves it to AR9W5", config: "[exposure]\ncollision_min_score = 0.1\nplugins = \"some\"\n" + areasTable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":          baseConfig + "\n" + tt.config,
				".claude/skills/charge/SKILL.md": "---\nname: charge\ndescription: Look up customer invoice details by name.\n---\nbody\n",
				".claude/agents/finder.md":       "---\nname: finder\ndescription: Finds customer invoice records.\n---\nbody\n",
			})
			src := &fakeExposure{pairs: collisionPairs()}
			var opts []Option
			if !tt.noSource {
				opts = append(opts, WithExposure(src))
			}

			// Act
			fs := lintOutputs(t, root, opts...)

			// Assert
			var got []string
			for _, f := range fs {
				if f.Code != CodeExposureAreaCollision {
					continue
				}
				assert.Equal(t, tt.severity, f.Severity)
				assert.Equal(t, AnalyzerInfo{AnalyzerExposure, ScopeBundle}, AnalyzerFor(f.Code))
				got = append(got, filepath.ToSlash(f.File)+":"+strconv.Itoa(f.Line)+" "+f.Message)
			}
			require.Len(t, got, len(tt.want), dump(fs))
			for i, w := range tt.want {
				assert.Contains(t, got[i], w)
			}
			assert.Equal(t, tt.wantAsked, src.asked)
		})
	}
}
