package exposure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

func TestListingBudgetChars(t *testing.T) {
	tests := []struct {
		window int
		want   int
	}{
		{200000, 8000},
		{1000000, 40000},
		{1000, 40},
		{12345, 493},
		{175, 7},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ListingBudgetChars(tt.window), "window %d", tt.window)
	}
	assert.Equal(t, harnesslimits.MustValue("claude.skill_listing_total_chars"), ListingBudgetChars(200000), "the limits table holds the budget at 200000 tokens")
}

func reportInventory() *Inventory {
	return &Inventory{
		Harness: HarnessClaude, Root: "/r",
		Plugins: PluginSummary{Mode: PluginsAll, Loaded: []string{"ops"}},
		Artifacts: []Artifact{
			{Kind: KindClaudeMD, QualifiedName: "CLAUDE.md", Path: "CLAUDE.md", Scope: RootScope, Phase: PhaseStart, Tokens: 100},
			{Kind: KindSkill, QualifiedName: "a", Path: ".claude/skills/a/SKILL.md", Scope: RootScope, Phase: PhaseStart, Tokens: 10, ListingChars: 40},
			{Kind: KindSkill, Name: "p", QualifiedName: "ops:p", Path: "tools/ops/skills/p/SKILL.md", Scope: RootScope, Phase: PhaseStart, Tokens: 20, ListingChars: 70, Truncated: true, Area: "platform"},
			{Kind: KindSkill, QualifiedName: "broken", Path: ".claude/skills/broken/SKILL.md", Scope: RootScope, Phase: PhaseUnknown, Tokens: 5, ListingChars: 30},
			{Kind: KindSkill, QualifiedName: "gated", Path: "svc/.claude/skills/gated/SKILL.md", Scope: "svc", Phase: PhaseOnDemand, Tokens: 7, ListingChars: 50, Truncated: true},
			{Kind: KindRule, QualifiedName: "r", Path: "svc/.claude/rules/r.md", Scope: "svc", Phase: PhaseOnDemand, Tokens: 300},
			{Kind: KindSkill, QualifiedName: "m", Path: ".claude/skills/m/SKILL.md", Scope: RootScope, Phase: PhaseManual, Tokens: 9, ListingChars: 20},
			{Kind: KindSettings, QualifiedName: ".claude/settings.json", Path: ".claude/settings.json", Scope: RootScope, Phase: PhaseStart},
		},
	}
}

func TestNewReport(t *testing.T) {
	// Act
	rep := NewReport(reportInventory(), ReportOptions{ContextWindow: 3000, StartBudget: 100})

	// Assert
	assert.Equal(t, SchemaReport, rep.SchemaVersion)
	assert.Equal(t, "/r", rep.Root)
	assert.Equal(t, []string{"ops"}, rep.Plugins.Loaded)
	assert.Equal(t, ListingBudget{ContextWindow: 3000, Skills: 3, Chars: 140, BudgetChars: 120, OverBudgetChars: 20, DescriptionsCapped: 1}, rep.Listing)
	assert.Equal(t, 135, rep.StartTokens)
	assert.Equal(t, map[Phase]int{PhaseStart: 130, PhaseUnknown: 5, PhaseOnDemand: 307, PhaseManual: 9}, rep.TokensByPhase)
	assert.Equal(t, map[Kind]int{KindClaudeMD: 100, KindSkill: 35, KindSettings: 0}, rep.StartTokensByKind)
	assert.Equal(t, map[Phase]map[Kind]int{
		PhaseStart:    {KindClaudeMD: 1, KindSkill: 2, KindSettings: 1},
		PhaseUnknown:  {KindSkill: 1},
		PhaseOnDemand: {KindSkill: 1, KindRule: 1},
		PhaseManual:   {KindSkill: 1},
	}, rep.CountsByPhaseAndKind)
	require.Len(t, rep.Items, 8)
	assert.Equal(t, ReportItem{Kind: KindSkill, Name: "ops:p", Path: "tools/ops/skills/p/SKILL.md", Scope: RootScope, Phase: PhaseStart, Tokens: 20, Area: "platform"}, rep.Items[2])
	assert.True(t, rep.OverStartBudget())
	assert.Equal(t, 5, rep.StartArtifacts())
}

func TestNewReportEmptyInventory(t *testing.T) {
	rep := NewReport(&Inventory{Harness: HarnessClaude}, ReportOptions{ContextWindow: 200000})

	assert.Equal(t, ListingBudget{ContextWindow: 200000, BudgetChars: 8000}, rep.Listing)
	assert.Empty(t, rep.TokensByPhase)
	assert.Equal(t, map[Phase]map[Kind]int{PhaseStart: {}, PhaseOnDemand: {}, PhaseManual: {}}, rep.CountsByPhaseAndKind)
	assert.NotNil(t, rep.Items)
	assert.False(t, rep.OverStartBudget())
}

func TestOverStartBudget(t *testing.T) {
	tests := []struct {
		name   string
		budget int
		want   bool
	}{
		{"no budget is never over", 0, false},
		{"under", 136, false},
		{"equal is not over", 135, false},
		{"over", 134, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep := NewReport(reportInventory(), ReportOptions{ContextWindow: 200000, StartBudget: tt.budget})
			assert.Equal(t, tt.want, rep.OverStartBudget())
		})
	}
}

func TestHeaviest(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want []string
	}{
		{"top two", 2, []string{"CLAUDE.md", "ops:p"}},
		{"unknown counts and a tie keeps inventory order", 5, []string{"CLAUDE.md", "ops:p", "a", "broken", ".claude/settings.json"}},
		{"more than there are", 50, []string{"CLAUDE.md", "ops:p", "a", "broken", ".claude/settings.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var names []string
			for _, it := range NewReport(reportInventory(), ReportOptions{ContextWindow: 200000}).Heaviest(tt.n) {
				names = append(names, it.Name)
			}
			assert.Equal(t, tt.want, names)
		})
	}
}

func TestNewReportOverABuiltInventory(t *testing.T) {
	// Arrange
	root := writeRepo(t, map[string]string{
		"CLAUDE.md":                         "# Repo\nRead the docs.\n",
		".claude/skills/deploy/SKILL.md":    skillMD("deploy", "Deploys the service."),
		".claude/skills/manual/SKILL.md":    skillMD("manual", "Only by hand.", "disable-model-invocation: true\n"),
		"svc/.claude/skills/local/SKILL.md": skillMD("local", "Svc only."),
	})
	inv := build(t, root)

	// Act
	rep := NewReport(inv, ReportOptions{ContextWindow: 200000})

	// Assert
	assert.Equal(t, 1, rep.Listing.Skills)
	assert.Equal(t, len("deploy: Deploys the service."), rep.Listing.Chars)
	assert.Equal(t, map[Kind]int{KindClaudeMD: 1, KindSkill: 1}, rep.CountsByPhaseAndKind[PhaseStart])
	assert.Equal(t, map[Kind]int{KindSkill: 1}, rep.CountsByPhaseAndKind[PhaseManual])
	assert.Equal(t, map[Kind]int{KindSkill: 1}, rep.CountsByPhaseAndKind[PhaseOnDemand])
	total := 0
	for _, a := range inv.Artifacts {
		if a.Phase == PhaseStart {
			total += a.Tokens
		}
	}
	assert.Equal(t, total, rep.StartTokens)
	assert.Positive(t, rep.StartTokens)
}
