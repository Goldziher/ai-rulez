package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListingSpec_Lists(t *testing.T) {
	spec := ListingSpec{Skills: true, Agents: true}
	tests := []struct {
		kind OutputKind
		want bool
	}{
		{OutputKindSkill, true},
		{OutputKindCommand, false},
		{OutputKindAgent, true},
		{OutputKindRuleFile, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			assert.Equal(t, tt.want, spec.Lists(tt.kind))
		})
	}
	assert.False(t, ListingSpec{}.Any())
	assert.True(t, spec.Any())
}

func TestAnalysisCollector_ClaimantsAndListing(t *testing.T) {
	collector := NewAnalysisCollector()
	collector.Begin("/p/.agents/skills/a/SKILL.md", "codex", OutputKindSkill, "a", "")
	collector.Attribute("cursor", "/p", []OutputFile{{Path: "/p/.agents/skills/a/SKILL.md"}})
	collector.Attribute("cursor", "/p", []OutputFile{{Path: "/p/.agents/skills/a/SKILL.md"}})

	assert.Equal(t, []string{"codex", "cursor"}, collector.Claimants("/p/.agents/skills/a/SKILL.md"))
	assert.Len(t, collector.Analyses(), 1, "one analysis per path")

	assert.True(t, collector.ListingFor("codex").IncludePath, "builtin table")
	assert.False(t, collector.ListingFor("amp").Any(), "not modeled")
	collector.DeclareListing("amp", &ListingSpec{Skills: true})
	assert.True(t, collector.ListingFor("amp").Skills, "a provider's own declaration wins")

	var none *AnalysisCollector
	assert.Nil(t, none.Claimants("x"))
	none.DeclareListing("x", &ListingSpec{})
}
