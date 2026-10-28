package exposure

import (
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

func TestInventory_ClassifyWithoutAMapLeavesEverythingUnclassified(t *testing.T) {
	inv := build(t, classifiedRepo(t))

	inv.Classify(nil)

	for _, a := range inv.Artifacts {
		assert.Equal(t, areas.Unclassified, a.Area, a.Path)
		assert.Equal(t, areas.SourceNone, a.AreaSource, a.Path)
	}
}
