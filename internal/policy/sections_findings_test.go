package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFindingLimits(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[lint]\nno_inline_ignore = [\"secret-detected\", \"AR005\", \"AR001\"]\n[lint.max_findings]\nAR010 = 0\nsecret-detected = 2\n"

	// Act
	_, p, err := Parse("p.toml", []byte(body))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"AR001", "AR005"}, p.Lint.NoInlineIgnore)
	assert.Equal(t, map[string]int{"AR010": 0, "AR001": 2}, p.Lint.MaxFindings)
}

func TestParseFindingLimitsRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"unknown ceiling rule", "policy_version = 1\n[lint.max_findings]\nAR9999 = 0\n", "unknown rule"},
		{"negative ceiling", "policy_version = 1\n[lint.max_findings]\nAR001 = -1\n", "must not be negative"},
		{"unknown no_inline_ignore rule", "policy_version = 1\n[lint]\nno_inline_ignore = [\"nope\"]\n", "unknown rule"},
		{"ceiling is not a number", "policy_version = 1\n[lint.max_findings]\nAR001 = \"none\"\n", "AR743"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, _, err := Parse("p.toml", []byte(tt.body))
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestMergeFindingLimits(t *testing.T) {
	a := Policy{Lint: Lint{MaxFindings: map[string]int{"AR001": 3, "AR005": 0}, NoInlineIgnore: []string{"AR001"}}}
	b := Policy{Lint: Lint{MaxFindings: map[string]int{"AR001": 1, "AR008": 4}, NoInlineIgnore: []string{"AR005"}}}

	got := Merge(a, b)

	assert.Equal(t, map[string]int{"AR001": 1, "AR005": 0, "AR008": 4}, got.Lint.MaxFindings, "the lower ceiling per code, and 0 is a ceiling")
	assert.Equal(t, []string{"AR001", "AR005"}, got.Lint.NoInlineIgnore, "inline refusals union")
	assert.Equal(t, got, Merge(b, a))
	assert.Equal(t, a.Lint.MaxFindings, Merge(a, Policy{}).Lint.MaxFindings)
}

func TestApplyPublishesFindingLimitsToTheLintOutcome(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	res := Resolve([]Layer{
		layer("managed", Policy{Lint: Lint{MaxFindings: map[string]int{"AR001": 0}, NoInlineIgnore: []string{"AR001"}}}),
		layer("env", Policy{Lint: Lint{MaxFindings: map[string]int{"AR001": 5, "AR005": 1}}}),
	})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	assert.Equal(t, map[string]int{"AR001": 0, "AR005": 1}, out.MaxFindings)
	assert.Equal(t, []string{"AR001"}, out.NoInlineIgnore)
	assert.Empty(t, out.Violations, "limits are enforced by the lint run, not reported at load")
}

func TestShowPolicyListsFindingLimits(t *testing.T) {
	// Arrange
	res := Resolve([]Layer{
		layer("managed", Policy{Lint: Lint{MaxFindings: map[string]int{"AR001": 0}, NoInlineIgnore: []string{"AR001"}}}),
	})
	// Act
	tree := res.Policy.Tree()
	// Assert
	lintTable, ok := tree["lint"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"AR001": 0}, lintTable["max_findings"])
	assert.Equal(t, []string{"AR001"}, lintTable["no_inline_ignore"])
	assert.Equal(t, "managed", res.Provenance["lint.max_findings.AR001"])
	assert.Equal(t, "managed", res.Provenance["lint.no_inline_ignore"])
}
