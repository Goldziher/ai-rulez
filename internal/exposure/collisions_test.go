package exposure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestListingWords(t *testing.T) {
	tests := []struct {
		name string
		a    Artifact
		want []string
	}{
		{
			name: "lowercased words of three or more letters and digits, stopwords dropped",
			a:    Artifact{QualifiedName: "Deploy-Staging", ListingText: "Deploy-Staging: Use this to ship the API v2 and its DB to staging."},
			want: []string{"api", "deploy", "ship", "staging"},
		},
		{
			name: "a word starts with a letter and digits continue it",
			a:    Artifact{QualifiedName: "x", ListingText: "x: 3d k8s s3bucket 42abc"},
			want: []string{"abc", "k8s", "s3bucket"},
		},
		{
			name: "the qualified name counts when the listing text is empty",
			a:    Artifact{QualifiedName: "ops:probe-service"},
			want: []string{"ops", "probe", "service"},
		},
		{
			name: "a dotted capital I lowercases to i and a combining dot, which ends a word; the Kelvin sign to k",
			a:    Artifact{QualifiedName: "a", ListingText: "a: İstanbul Kelvin"},
			want: []string{"kelvin", "stanbul"},
		},
		{
			name: "every stopword is dropped",
			a:    Artifact{QualifiedName: "x", ListingText: "x: the and for with from into that this these those when use used using not any all can are was has have its their your you our also via per etc each such than then them they will would should only other task tasks user users need needs needed instead without within about over under before after"},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sortedWords(listingWords(&tt.a)))
		})
	}
}

func listed(kind Kind, path, area, text string) Artifact {
	return Artifact{Kind: kind, Phase: PhaseStart, Path: path, QualifiedName: path, Area: area, ListingText: text}
}

func TestFindCollisions(t *testing.T) {
	tests := []struct {
		name     string
		arts     []Artifact
		minScore float64
		compared int
		want     []Collision
	}{
		{
			name: "two listings of different areas share words",
			arts: []Artifact{
				listed(KindSkill, "a", "billing", "lookup customer details"),
				listed(KindAgent, "b", "search", "lookup customer tables"),
			},
			minScore: 0.1,
			compared: 2,
			want: []Collision{{
				Score: 0.5, Shared: []string{"customer", "lookup"},
				First: "a", FirstKind: KindSkill, FirstName: "a", FirstArea: "billing",
				Second: "b", SecondKind: KindAgent, SecondName: "b", SecondArea: "search",
			}},
		},
		{
			name: "listings of the same area never collide, unclassified included",
			arts: []Artifact{
				listed(KindSkill, "a", "billing", "lookup customer"),
				listed(KindSkill, "b", "billing", "lookup customer"),
				listed(KindSkill, "c", areas.Unclassified, "lookup customer"),
				listed(KindSkill, "d", areas.Unclassified, "lookup customer"),
			},
			minScore: 0,
			compared: 4,
			want: []Collision{
				{Score: 1, Shared: []string{"customer", "lookup"}, First: "a", FirstKind: KindSkill, FirstName: "a", FirstArea: "billing", Second: "c", SecondKind: KindSkill, SecondName: "c", SecondArea: areas.Unclassified},
				{Score: 1, Shared: []string{"customer", "lookup"}, First: "a", FirstKind: KindSkill, FirstName: "a", FirstArea: "billing", Second: "d", SecondKind: KindSkill, SecondName: "d", SecondArea: areas.Unclassified},
				{Score: 1, Shared: []string{"customer", "lookup"}, First: "b", FirstKind: KindSkill, FirstName: "b", FirstArea: "billing", Second: "c", SecondKind: KindSkill, SecondName: "c", SecondArea: areas.Unclassified},
				{Score: 1, Shared: []string{"customer", "lookup"}, First: "b", FirstKind: KindSkill, FirstName: "b", FirstArea: "billing", Second: "d", SecondKind: KindSkill, SecondName: "d", SecondArea: areas.Unclassified},
			},
		},
		{
			name: "only skills and agents at session start are compared, an unknown phase failing closed",
			arts: []Artifact{
				listed(KindSkill, "a", "billing", "lookup customer"),
				{Kind: KindSkill, Phase: PhaseOnDemand, Path: "b", Area: "search", ListingText: "lookup customer"},
				{Kind: KindSkill, Phase: PhaseManual, Path: "c", Area: "search", ListingText: "lookup customer"},
				{Kind: KindSkill, Phase: PhaseUnknown, Path: "d", QualifiedName: "d", Area: "search", ListingText: "lookup customer"},
				{Kind: KindClaudeMD, Phase: PhaseStart, Path: "CLAUDE.md", Area: "search", Text: "lookup customer"},
				{Kind: KindRule, Phase: PhaseStart, Path: "e", Area: "search", Text: "lookup customer"},
			},
			minScore: 0,
			compared: 2,
			want: []Collision{
				{Score: 1, Shared: []string{"customer", "lookup"}, First: "a", FirstKind: KindSkill, FirstName: "a", FirstArea: "billing", Second: "d", SecondKind: KindSkill, SecondName: "d", SecondArea: "search"},
			},
		},
		{
			name: "a pair under the minimum score is dropped and one at it is kept",
			arts: []Artifact{
				listed(KindSkill, "a", "billing", "alpha beta gamma delta epsilon zeta eta theta iota kappa"),
				listed(KindSkill, "b", "search", "alpha lambda omicron sigma upsilon phi chi psi omega rho"),
				listed(KindSkill, "c", "tooling", "kappa lambda xis"),
			},
			minScore: 1.0 / 12,
			compared: 3,
			want: []Collision{
				{Score: 1.0 / 12, Shared: []string{"kappa"}, First: "a", FirstKind: KindSkill, FirstName: "a", FirstArea: "billing", Second: "c", SecondKind: KindSkill, SecondName: "c", SecondArea: "tooling"},
				{Score: 1.0 / 12, Shared: []string{"lambda"}, First: "b", FirstKind: KindSkill, FirstName: "b", FirstArea: "search", Second: "c", SecondKind: KindSkill, SecondName: "c", SecondArea: "tooling"},
			},
		},
		{
			name: "pairs without a shared word are dropped even at a zero minimum",
			arts: []Artifact{
				listed(KindSkill, "a", "billing", "alpha"),
				listed(KindSkill, "b", "search", "beta"),
			},
			minScore: 0,
			compared: 2,
			want:     []Collision{},
		},
		{
			name: "ranked by score, then first path, then second path; first is the earlier artifact",
			arts: []Artifact{
				listed(KindSkill, "z", "billing", "one two"),
				listed(KindSkill, "y", "search", "one two"),
				listed(KindSkill, "x", "tooling", "one two three four"),
			},
			minScore: 0,
			compared: 3,
			want: []Collision{
				{Score: 1, Shared: []string{"one", "two"}, First: "z", FirstKind: KindSkill, FirstName: "z", FirstArea: "billing", Second: "y", SecondKind: KindSkill, SecondName: "y", SecondArea: "search"},
				{Score: 0.5, Shared: []string{"one", "two"}, First: "y", FirstKind: KindSkill, FirstName: "y", FirstArea: "search", Second: "x", SecondKind: KindSkill, SecondName: "x", SecondArea: "tooling"},
				{Score: 0.5, Shared: []string{"one", "two"}, First: "z", FirstKind: KindSkill, FirstName: "z", FirstArea: "billing", Second: "x", SecondKind: KindSkill, SecondName: "x", SecondArea: "tooling"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			compared, got := FindCollisions(tt.arts, tt.minScore)

			// Assert
			assert.Equal(t, tt.compared, compared)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewCollisionReport(t *testing.T) {
	// Arrange
	root := writeRepo(t, map[string]string{
		"services/billing/.claude/skills/x/SKILL.md": skillMD("x", "Look up customer details."),
		".claude/skills/charge/SKILL.md":             skillMD("charge", "Look up customer invoice details by name."),
		".claude/skills/rank/SKILL.md":               skillMD("rank", "Look up customer ranker details by name."),
		".claude/skills/tune/SKILL.md":               skillMD("tune", "Tune the ranker weights by name."),
		".claude/agents/finder.md":                   "---\nname: finder\ndescription: Finds customer invoice records.\n---\nBody.\n",
	})
	inv := build(t, root, func(o *Options) { o.Plugins = PluginsNone })
	m, problems := areas.New([]config.ExposureArea{
		{Name: "billing", Terms: []string{"invoice"}},
		{Name: "search", Terms: []string{"ranker"}},
	}, map[string]string{"tune": "billing"})
	require.Empty(t, problems)

	tests := []struct {
		name  string
		limit int
		want  []string
		total int
	}{
		{name: "no limit keeps every pair", limit: 0, want: []string{"charge<>rank", "rank<>tune", "finder<>rank"}, total: 3},
		{name: "the limit keeps the top rows and the total counts every pair", limit: 2, want: []string{"charge<>rank", "rank<>tune"}, total: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			rep := NewCollisionReport(inv, m, 0.1, tt.limit)

			// Assert
			got := make([]string, 0, len(rep.Collisions))
			for _, c := range rep.Collisions {
				got = append(got, c.FirstName+"<>"+c.SecondName)
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, SchemaCollisions, rep.SchemaVersion)
			assert.Equal(t, 4, rep.ArtifactsCompared)
			assert.Equal(t, tt.total, rep.Total)
			assert.Equal(t, PluginsNone, rep.Plugins)
			assert.InDelta(t, 0.1, rep.MinScore, 0)
			assert.Empty(t, inv.Artifacts[0].Area, "the report classifies a copy")
		})
	}
}
