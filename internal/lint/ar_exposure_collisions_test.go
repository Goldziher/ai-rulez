package lint

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCollisions returns its pairs at or over the threshold it is asked for and records the threshold.
type fakeCollisions struct {
	noExposure
	pairs  []ExposureCollision
	asked  []float64
	inputs []string
	root   string
}

func (f *fakeCollisions) Inputs() []string { return f.inputs }
func (f *fakeCollisions) Root() string     { return f.root }

func (f *fakeCollisions) Collisions(minScore float64) []ExposureCollision {
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
			src := &fakeCollisions{pairs: collisionPairs()}
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

const collisionsConfig = "[exposure]\ncollision_min_score = 0.1\n[[exposure.areas]]\nname = \"billing\"\nterms = [\"invoice\"]\n[[exposure.areas]]\nname = \"search\"\nterms = [\"ranker\"]\n"

// collisionProject writes the listings of collisionPairs and an unrelated file.
func collisionProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":          baseConfig + "\n" + collisionsConfig,
		".claude/skills/charge/SKILL.md": "---\nname: charge\ndescription: Look up customer invoice details by name.\n---\nbody\n",
		".claude/skills/rank/SKILL.md":   "---\nname: rank\ndescription: Look up customer ranker details by name.\n---\nbody\n",
		".claude/agents/finder.md":       "---\nname: finder\ndescription: Finds customer invoice records.\n---\nbody\n",
		"tools/areas.toml":               "# unused\n",
		"README.md":                      "unrelated\n",
	})
	gitAdd(t, root)
	return root
}

// Under --changed, an AR9W2 finding sits on the pair's first listing but depends on the second listing and on
// the area map and the configuration, so editing rank into a collision with charge keeps the finding on charge.
func TestExposureAreaCollisionUnderChanged(t *testing.T) {
	both := []string{".claude/agents/finder.md", ".claude/skills/charge/SKILL.md"}
	tests := []struct {
		name    string
		changed []string
		want    []string
	}{
		{name: "a changed first listing keeps its own finding", changed: []string{".claude/skills/charge/SKILL.md"}, want: []string{".claude/skills/charge/SKILL.md"}},
		{name: "a changed second listing keeps the findings of its pairs", changed: []string{".claude/skills/rank/SKILL.md"}, want: both},
		{name: "a changed areas file keeps every pair", changed: []string{"tools/areas.toml"}, want: both},
		{name: "a changed configuration keeps every pair", changed: []string{".ai-rulez/config.toml"}, want: both},
		{name: "an unrelated change keeps none", changed: []string{"README.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := collisionProject(t)
			src := &fakeCollisions{pairs: collisionPairs(), inputs: []string{
				filepath.Join(root, "tools", "areas.toml"), filepath.Join(root, ".ai-rulez", "config.toml"),
			}}
			rep := leakRun(t, root, WithExposure(src))

			// Act
			NarrowToChanged(rep, tt.changed, "HEAD")

			// Assert
			var got []string
			for _, f := range codeFindings(rep.Findings, CodeExposureAreaCollision) {
				got = append(got, f.RepoPath())
			}
			assert.Equal(t, tt.want, got, dump(rep.Findings))
		})
	}
}

// A source rooted below the project (exposure leakage --root) names its listings relative to that root.
func TestExposureAreaCollisionUnderTheSourceRoot(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":              baseConfig + "\n" + collisionsConfig,
		"svc/.claude/skills/charge/SKILL.md": "---\nname: charge\ndescription: Look up customer invoice details by name.\n---\nbody\n",
	})
	pairs := collisionPairs()[:1]
	src := &fakeCollisions{pairs: pairs, root: filepath.Join(root, "svc")}

	// Act
	fs := codeFindings(lintOutputs(t, root, WithExposure(src)), CodeExposureAreaCollision)

	// Assert
	require.Len(t, fs, 1, dump(fs))
	assert.Equal(t, "svc/.claude/skills/charge/SKILL.md", filepath.ToSlash(fs[0].File))
	assert.Equal(t, 3, fs[0].Line, "the description line of the listing below the source root")
}

// AR9W2 reports the DefaultExposureCollisionLimit highest pairs one by one and sums up the rest in one finding
// on the collision_min_score line, at the rule's severity.
func TestExposureAreaCollisionIsCapped(t *testing.T) {
	pairsOf := func(n int) []ExposureCollision {
		out := make([]ExposureCollision, 0, n)
		for i := range n {
			out = append(out, ExposureCollision{
				Score: 0.9 - float64(i)/100, Shared: []string{"customer"},
				First: fmt.Sprintf(".claude/skills/s%02d/SKILL.md", i), FirstKind: "skill", FirstName: fmt.Sprintf("s%02d", i), FirstArea: "billing",
				Second: ".claude/skills/rank/SKILL.md", SecondKind: "skill", SecondName: "rank", SecondArea: "search",
			})
		}
		return out
	}
	tests := []struct {
		name        string
		config      string
		pairs       int
		wantPairs   int
		wantSummary string
		severity    Severity
	}{
		{name: "at the limit, no summary", config: collisionsConfig, pairs: DefaultExposureCollisionLimit, wantPairs: DefaultExposureCollisionLimit, severity: SeverityWarning},
		{
			name: "past the limit, one summary", config: collisionsConfig, pairs: DefaultExposureCollisionLimit + 5, wantPairs: DefaultExposureCollisionLimit,
			wantSummary: "and 5 more pairs of start-of-session listings of different areas overlap at or over 0.10, past the 20 reported one by one; see `ai-rulez exposure collisions`",
			severity:    SeverityWarning,
		},
		{
			name: "the summary takes the configured severity", config: collisionsConfig + "[lint.severity]\nAR9W2 = \"error\"\n", pairs: DefaultExposureCollisionLimit + 1,
			wantPairs: DefaultExposureCollisionLimit, wantSummary: "and 1 more pair of", severity: SeverityError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig + "\n" + tt.config})

			// Act
			fs := codeFindings(lintOutputs(t, root, WithExposure(&fakeCollisions{pairs: pairsOf(tt.pairs)})), CodeExposureAreaCollision)

			// Assert
			var pairs, summaries []Finding
			for _, f := range fs {
				assert.Equal(t, tt.severity, f.Severity, "%s", f.Message)
				if strings.HasPrefix(f.Message, "and ") {
					summaries = append(summaries, f)
					continue
				}
				pairs = append(pairs, f)
			}
			assert.Len(t, pairs, tt.wantPairs, dump(fs))
			assert.NotContains(t, dump(pairs), fmt.Sprintf("s%02d", DefaultExposureCollisionLimit), "the lowest pairs are the ones summed up")
			if tt.wantSummary == "" {
				assert.Empty(t, summaries, dump(fs))
				return
			}
			require.Len(t, summaries, 1, dump(fs))
			assert.Contains(t, summaries[0].Message, tt.wantSummary)
			assert.Equal(t, ".ai-rulez/config.toml", filepath.ToSlash(summaries[0].File))
			assert.Equal(t, 6, summaries[0].Line, "the collision_min_score line")
		})
	}
}
