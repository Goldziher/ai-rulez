package ard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckAcceptsTheFixture(t *testing.T) {
	findings, err := Check(fixtureModel())
	require.NoError(t, err)
	assert.False(t, HasErrors(findings), "%+v", findings)
}

func TestCheckReportsEveryProblemWithItsRule(t *testing.T) {
	m := Model{Publisher: "example.com", Namespace: "tools", Resources: []Resource{
		{Kind: KindSkill, Name: "bad name", URL: "https://example.com/a"},
		{Kind: KindSkill, Name: "both", URL: "https://example.com/a", Data: map[string]any{"a": 1}},
		{Kind: KindSkill, Name: "plain", URL: "http://example.com/a"},
		{Kind: KindSkill, Name: "twice", URL: "https://example.com/a"},
		{Kind: KindMCPServer, Name: "twice", URL: "https://example.com/b"},
	}}
	findings, err := Check(m)
	require.NoError(t, err)
	got := map[string]int{}
	for _, f := range findings {
		got[f.Rule]++
	}
	// the colliding entry also has no queries; the others never render
	assert.Equal(t, map[string]int{RuleIdentifier: 2, RuleEntry: 2, RuleQueries: 1}, got, "%+v", findings)
}

func TestCheckWarnsAboutQueries(t *testing.T) {
	m := Model{Publisher: "example.com", Namespace: "tools", Resources: []Resource{
		{Kind: KindSkill, Name: "one", URL: "https://example.com/a", RepresentativeQueries: []string{"only one"}},
	}}
	findings, err := Check(m)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, RuleQueries, findings[0].Rule)
	assert.Equal(t, SeverityWarning, findings[0].Severity)
	assert.Equal(t, "urn:air:example.com:tools:one", findings[0].Identifier)
}

func TestCheckBadPublisher(t *testing.T) {
	findings, err := Check(Model{Publisher: "localhost", Namespace: "t", Resources: []Resource{
		{Kind: KindSkill, Name: "a", URL: "https://example.com/a"},
	}})
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, RuleIdentifier, findings[0].Rule)
}
