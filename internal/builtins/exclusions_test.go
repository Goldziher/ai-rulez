package builtins

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
