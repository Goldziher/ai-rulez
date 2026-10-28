package exposure

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestArtifact_AreaSkill(t *testing.T) {
	tests := []struct {
		name string
		a    Artifact
		want areas.Skill
	}{
		{
			name: "a repository skill is owned by its scope and classified by its listing text",
			a:    Artifact{Kind: KindSkill, Source: SourceFile, Name: "charge", QualifiedName: "charge", Scope: "services/billing", ListingText: "charge: Charges a card.", Text: "charge: Charges a card."},
			want: areas.Skill{Name: "charge", Scope: "services/billing", Text: "charge: Charges a card."},
		},
		{
			name: "the listing text is used when the text was dropped",
			a:    Artifact{Kind: KindSkill, Source: SourceCommand, Name: "a:b", QualifiedName: "a:b", Scope: RootScope, ListingText: "a:b: Runs b."},
			want: areas.Skill{Name: "a:b", Scope: RootScope, Text: "a:b: Runs b."},
		},
		{
			name: "a plugin skill is owned by its plugin directory and classified by its qualified name",
			a:    Artifact{Kind: KindSkill, Source: SourcePlugin, Name: "probe", QualifiedName: "ops:probe", Plugin: "ops", PluginPath: "tools/plugins/ops", Scope: RootScope, ListingText: "ops:probe: Probes invoices.", Text: "ops:probe: Probes invoices."},
			want: areas.Skill{Name: "probe", Plugin: "ops", Scope: "tools/plugins/ops", Text: "ops:probe"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.AreaSkill())
		})
	}
}

// classifiedRepo is a tree whose plugin skills name terms of an area other than the one that owns them, so the
// owner shows whether the plugin directory and the qualified name were used.
func classifiedRepo(t *testing.T) string {
	t.Helper()
	return writeRepo(t, map[string]string{
		MarketplaceFile: `{"name": "mkt", "plugins": [
  {"name": "ops", "source": "./tools/plugins/ops"},
  {"name": "kit", "source": "./kit"}
]}`,
		"tools/plugins/ops/skills/probe/SKILL.md":         skillMD("probe", "Probes every invoice."),
		"kit/skills/solo/SKILL.md":                        skillMD("solo", "Does one thing."),
		"kit/skills/lookup/SKILL.md":                      skillMD("lookup", "Looks things up."),
		"kit/skills/plain/SKILL.md":                       skillMD("plain", "Tunes the ranker."),
		"services/billing/.claude/skills/charge/SKILL.md": skillMD("charge", "Tunes the ranker."),
		".claude/skills/tune/SKILL.md":                    skillMD("tune", "Tunes the ranker."),
		".claude/skills/deploy/SKILL.md":                  skillMD("deploy", "Ships an invoice."),
		"CLAUDE.md":                                       "Every invoice goes through here.\n",
		"products/search/CLAUDE.md":                       "Nothing to see.\n",
	})
}

func classifiedMap(t *testing.T) *areas.Map {
	t.Helper()
	m, problems := areas.New([]config.ExposureArea{
		{Name: "billing", Paths: []string{"services/billing"}, Terms: []string{"invoice"}},
		{Name: "search", Paths: []string{"products/search"}, Terms: []string{"ranker"}},
		{Name: "tooling", Paths: []string{"tools"}, Shared: true},
	}, map[string]string{"kit:solo": "search", "lookup": "billing", "deploy": "tooling"})
	require.Empty(t, problems)
	return m
}

func TestInventory_Classify(t *testing.T) {
	// Arrange
	inv := build(t, classifiedRepo(t), func(o *Options) { o.Plugins = PluginsAll })

	// Act
	inv.Classify(classifiedMap(t))

	// Assert
	tests := []struct {
		kind   Kind
		name   string
		area   string
		source areas.Source
	}{
		{KindSkill, "ops:probe", "tooling", areas.SourcePath},
		{KindSkill, "kit:solo", "search", areas.SourceSkillAreas},
		{KindSkill, "kit:lookup", "billing", areas.SourceSkillAreas},
		{KindSkill, "kit:plain", areas.Unclassified, areas.SourceNone},
		{KindSkill, "charge", "billing", areas.SourcePath},
		{KindSkill, "tune", "search", areas.SourceTerms},
		{KindSkill, "deploy", "tooling", areas.SourceSkillAreas},
		{KindClaudeMD, "CLAUDE.md", "billing", areas.SourceTerms},
		{KindClaudeMD, "products/search/CLAUDE.md", "search", areas.SourcePath},
	}
	for _, tt := range tests {
		a := named(t, inv, tt.kind, tt.name)
		assert.Equal(t, tt.area, a.Area, "area of %s", tt.name)
		assert.Equal(t, tt.source, a.AreaSource, "source of %s", tt.name)
	}
}

// An inventory of a directory below the project root reads the map's path claims against the scopes as the
// project root sees them, so the claim services/billing owns what lies at the inventory's own root.
func TestInventory_ClassifyBelowTheAreaBase(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"services/billing/CLAUDE.md":                 "Nothing to see.\n",
		"services/billing/.claude/skills/x/SKILL.md": skillMD("x", "Does a thing."),
		"services/billing/api/CLAUDE.md":             "Nothing here either.\n",
	})
	root := filepath.Join(repo, "services", "billing")
	tests := []struct {
		name     string
		base     string
		wantArea string
		wantSrc  areas.Source
		scopes   map[string]string
	}{
		{
			name: "below the base, the claims of the project root apply", base: repo, wantArea: "billing", wantSrc: areas.SourcePath,
			scopes: map[string]string{"CLAUDE.md": "services/billing", "x": "services/billing", "api/CLAUDE.md": "services/billing/api"},
		},
		{
			name: "without a base, the scopes are the inventory root's", wantArea: areas.Unclassified, wantSrc: areas.SourceNone,
			scopes: map[string]string{"CLAUDE.md": RootScope, "x": RootScope, "api/CLAUDE.md": "api"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			inv := build(t, root, func(o *Options) { o.AreaBase = tt.base })

			// Act
			inv.Classify(classifiedMap(t))

			// Assert
			for _, probe := range []struct {
				kind Kind
				name string
			}{{KindClaudeMD, "CLAUDE.md"}, {KindSkill, "x"}, {KindClaudeMD, "api/CLAUDE.md"}} {
				a := named(t, inv, probe.kind, probe.name)
				assert.Equal(t, tt.wantArea, a.Area, "area of %s", probe.name)
				assert.Equal(t, tt.wantSrc, a.AreaSource, "source of %s", probe.name)
				assert.Equal(t, tt.scopes[probe.name], inv.ClaimScope(&a), "claim scope of %s", probe.name)
			}
			rep := NewAreasReport(inv, classifiedMap(t), "")
			for _, c := range rep.Artifacts {
				if want, ok := tt.scopes[c.QualifiedName]; ok {
					assert.Equal(t, want, c.AreaScope, "the report's area_scope of %s", c.QualifiedName)
				}
			}
		})
	}
}

func TestInventory_ClassifyWithoutAMapLeavesEverythingUnclassified(t *testing.T) {
	inv := build(t, classifiedRepo(t))

	inv.Classify(nil)

	for _, a := range inv.Artifacts {
		assert.Equal(t, areas.Unclassified, a.Area, a.Path)
		assert.Equal(t, areas.SourceNone, a.AreaSource, a.Path)
	}
}

func TestNewAreasReport(t *testing.T) {
	// Arrange
	inv := build(t, classifiedRepo(t), func(o *Options) { o.Plugins = PluginsAll })

	// Act
	rep := NewAreasReport(inv, classifiedMap(t), "tools/areas.toml")

	// Assert
	assert.Equal(t, SchemaAreas, rep.SchemaVersion)
	assert.Equal(t, inv.Root, rep.Root)
	assert.Equal(t, "tools/areas.toml", rep.AreasFile)
	assert.Equal(t, PluginsAll, rep.Plugins)
	require.Len(t, rep.Areas, 3)
	assert.Equal(t, AreaInfo{Name: "tooling", Paths: []string{"tools"}, Terms: []string{}, Ambiguous: []string{}, Shared: true}, rep.Areas[2])
	assert.Equal(t, map[string]string{"kit:solo": "search", "lookup": "billing", "deploy": "tooling"}, rep.SkillAreas)
	require.Len(t, rep.Artifacts, len(inv.Artifacts))
	probe := rep.Artifacts[0]
	for _, a := range rep.Artifacts {
		if a.QualifiedName == "ops:probe" {
			probe = a
		}
	}
	assert.Equal(t, ClassifiedArtifact{
		Kind: KindSkill, Source: SourcePlugin, Name: "probe", QualifiedName: "ops:probe", Plugin: "ops",
		Path: "tools/plugins/ops/skills/probe/SKILL.md", Scope: RootScope, AreaScope: "tools/plugins/ops",
		Phase: PhaseStart, Area: "tooling", AreaSource: areas.SourcePath,
	}, probe)
	total := 0
	for _, n := range rep.Counts {
		total += n
	}
	assert.Equal(t, len(inv.Artifacts), total)
	assert.Equal(t, 2, rep.Counts["tooling"], "ops:probe and deploy")
	assert.Empty(t, inv.Artifacts[0].Area, "the report leaves the inventory unclassified")
}

// Every area and unclassified has a count, 0 when no artifact is in it, and
// the inventory's problems carry through, so a reader sees an artifact that was
// classified on an empty text.
func TestNewAreasReport_CountsEveryAreaAndKeepsTheProblems(t *testing.T) {
	inv := &Inventory{Root: "/r", Artifacts: []Artifact{}, Problems: []Problem{{Path: "big.md", Error: "over the read limit"}}}

	rep := NewAreasReport(inv, classifiedMap(t), "")

	assert.Equal(t, map[string]int{"billing": 0, "search": 0, "tooling": 0, areas.Unclassified: 0}, rep.Counts)
	assert.Equal(t, []Problem{{Path: "big.md", Error: "over the read limit"}}, rep.Problems)
	assert.NotNil(t, NewAreasReport(&Inventory{}, classifiedMap(t), "").Problems, "no problems is an empty list")
}
