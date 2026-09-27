package builtins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExcludedRules_parsesPerRuleExclusions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  map[string]bool
	}{
		{
			name:  "per-rule exclusion is captured",
			input: []string{"git-workflow", "!git-workflow/commit-messages"},
			want:  map[string]bool{"git-workflow/commit-messages": true},
		},
		{
			name:  "whole-domain exclusion is not a per-rule exclusion",
			input: []string{"!git-workflow"},
			want:  map[string]bool{},
		},
		{
			name:  "plain names are ignored",
			input: []string{"git-workflow", "testing"},
			want:  map[string]bool{},
		},
		{
			name:  "multiple per-rule exclusions",
			input: []string{"!testing/tdd-workflow", "!git-workflow/branch-hygiene"},
			want:  map[string]bool{"testing/tdd-workflow": true, "git-workflow/branch-hygiene": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ExcludedRules(tt.input))
		})
	}
}

// TestResolveBuiltins_perRuleExclusionKeepsDomain verifies that a "!domain/rule"
// exclusion does not disable the whole domain, while a bare "!domain" does.
func TestResolveBuiltins_perRuleExclusionKeepsDomain(t *testing.T) {
	t.Parallel()

	withRule := ResolveBuiltins([]string{"git-workflow", "!git-workflow/commit-messages"})
	assert.Contains(t, withRule, "git-workflow", "per-rule exclusion must not drop the domain")

	withoutDomain := ResolveBuiltins([]string{"git-workflow", "!git-workflow"})
	assert.NotContains(t, withoutDomain, "git-workflow", "whole-domain exclusion must drop the domain")
}

// TestUnknownExclusions covers the reporting of exclusions that suppress nothing.
// The "valid" cases matter as much as the unknown ones: an exclusion that does
// resolve must stay silent, or the warning becomes noise on every real project.
func TestUnknownExclusions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []UnknownExclusion
	}{
		{
			name:  "unknown rule in a real domain is reported",
			input: []string{"!ai-governance/this-rule-does-not-exist"},
			want:  []UnknownExclusion{{Spec: "ai-governance/this-rule-does-not-exist"}},
		},
		{
			name:  "unknown domain is reported",
			input: []string{"!nosuchpack"},
			want:  []UnknownExclusion{{Spec: "nosuchpack"}},
		},
		{
			name:  "unknown domain in a rule-level exclusion is reported",
			input: []string{"!nosuchpack/whatever"},
			want:  []UnknownExclusion{{Spec: "nosuchpack/whatever"}},
		},
		{
			name:  "a mistyped rule name suggests the real rule",
			input: []string{"!git-workflow/commit-message"},
			want: []UnknownExclusion{{
				Spec:       "git-workflow/commit-message",
				Suggestion: "git-workflow/commit-messages",
			}},
		},
		{
			name:  "a mistyped domain name suggests the real domain",
			input: []string{"!ai-govrnance"},
			want:  []UnknownExclusion{{Spec: "ai-govrnance", Suggestion: "ai-governance"}},
		},
		{
			name:  "a valid rule-level exclusion is not reported",
			input: []string{"git-workflow", "!git-workflow/commit-messages"},
			want:  nil,
		},
		{
			name:  "a valid domain-level exclusion is not reported",
			input: []string{"rust", "!ai-governance"},
			want:  nil,
		},
		{
			name:  "valid non-rule content exclusions are not reported",
			input: []string{"!security/owasp-quick-reference", "!security/security-auditor", "!agent-delegation/agent-delegation"},
			want:  nil,
		},
		{
			name:  "plain includes are never reported",
			input: []string{"rust", "security", "testing"},
			want:  nil,
		},
		{
			name:  "a repeated unknown exclusion is reported once",
			input: []string{"!nosuchpack", "!nosuchpack"},
			want:  []UnknownExclusion{{Spec: "nosuchpack"}},
		},
		{
			name:  "only the unknown entry of a mixed list is reported",
			input: []string{"!testing/tdd-workflow", "!testing/tdd-workflows", "!security"},
			want:  []UnknownExclusion{{Spec: "testing/tdd-workflows", Suggestion: "testing/tdd-workflow"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, UnknownExclusions(tt.input))
		})
	}
}

// TestUnknownExclusions_everyRealIdentifierResolves guards the check against
// drifting out of step with the embedded builtins: every domain name and every
// "domain/name" content identifier must be accepted as an exclusion.
func TestUnknownExclusions_everyRealIdentifierResolves(t *testing.T) {
	t.Parallel()

	var specs []string
	for _, domain := range ResolveAll() {
		specs = append(specs, "!"+domain)
		for _, qualified := range qualifiedContentNames(domain) {
			specs = append(specs, "!"+qualified)
		}
	}

	require.Greater(t, len(specs), len(ResolveAll()), "expected qualified identifiers from the embedded builtins")
	assert.Empty(t, UnknownExclusions(specs))
}
