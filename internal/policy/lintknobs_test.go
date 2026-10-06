package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func intPtr(n int) *int { return &n }

func TestParseLintKnobs(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[lint.security]\ndirective_tags = [\"assistant\", \"human\", \"assistant\"]\ntrusted_orgs = [\" Acme \", \"Anthropics\"]\n" +
		"[lint.capability]\nmax_network_commands = 3\n[lint.load_budgets]\nclaude-skill-listing = 1000\n"

	// Act
	_, p, err := Parse("p.toml", []byte(body))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"assistant", "human"}, p.Lint.Security.DirectiveTags)
	assert.Equal(t, List{Set: true, Items: []string{"acme", "anthropics"}}, p.Lint.Security.TrustedOrgs)
	assert.Equal(t, intPtr(3), p.Lint.Capability.MaxNetworkCommands)
	assert.Equal(t, map[string]int{"claude-skill-listing": 1000}, p.Lint.LoadBudgets)
}

func TestParseLintKnobsRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"bad tag", "policy_version = 1\n[lint.security]\ndirective_tags = [\"a b\"]\n", "directive_tags"},
		{"empty org", "policy_version = 1\n[lint.security]\ntrusted_orgs = [\"\"]\n", "trusted_orgs"},
		{"negative network", "policy_version = 1\n[lint.capability]\nmax_network_commands = -1\n", "max_network_commands"},
		{"unknown budget", "policy_version = 1\n[lint.load_budgets]\nnope = 5\n", "unknown limit"},
		{"zero budget", "policy_version = 1\n[lint.load_budgets]\nclaude-skill-listing = 0\n", "must be positive"},
		{"unknown capability key", "policy_version = 1\n[lint.capability]\nmax_net = 1\n", "unknown key"},
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

func TestMergeLintKnobs(t *testing.T) {
	a := Policy{Lint: Lint{
		Security:    Security{DirectiveTags: []string{"a"}, TrustedOrgs: List{Set: true, Items: []string{"x", "y"}}},
		Capability:  Capability{MaxNetworkCommands: intPtr(3)},
		LoadBudgets: map[string]int{"claude-skill-listing": 1000, "cursor-rule-lines": 400},
	}}
	b := Policy{Lint: Lint{
		Security:    Security{DirectiveTags: []string{"b"}, TrustedOrgs: List{Set: true, Items: []string{"y", "z"}}},
		Capability:  Capability{MaxNetworkCommands: intPtr(1)},
		LoadBudgets: map[string]int{"claude-skill-listing": 2000, "codex-agents-chain": 100},
	}}

	got := Merge(a, b)

	assert.Equal(t, []string{"a", "b"}, got.Lint.Security.DirectiveTags, "tags union")
	assert.Equal(t, List{Set: true, Items: []string{"y"}}, got.Lint.Security.TrustedOrgs, "orgs intersect")
	assert.Equal(t, intPtr(1), got.Lint.Capability.MaxNetworkCommands, "the lower limit wins")
	assert.Equal(t, map[string]int{"claude-skill-listing": 1000, "cursor-rule-lines": 400, "codex-agents-chain": 100}, got.Lint.LoadBudgets, "the lower budget wins per key")
	assert.Equal(t, got, Merge(b, a))
	assert.Equal(t, a.Lint.Capability, Merge(a, Policy{}).Lint.Capability, "unset constrains nothing")
}

func TestApplyDirectiveTags(t *testing.T) {
	pol := Policy{Lint: Lint{Security: Security{DirectiveTags: []string{"assistant"}}}}
	tests := []struct {
		name string
		repo []string
		want []string
	}{
		{"unset takes the policy tags", nil, []string{"assistant"}},
		{"the repository may add tags", []string{"human"}, []string{"assistant", "human"}},
		{"omitting a policy tag is not a loosening", []string{"assistant"}, []string{"assistant"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "")
			cfg.Lint = &config.LintConfig{Security: &config.LintSecurity{DirectiveTags: tt.repo}}
			// Act
			res := Resolve([]Layer{layer("managed", pol)}).Apply(cfg)
			// Assert
			assert.ElementsMatch(t, tt.want, cfg.Lint.Security.DirectiveTags)
			assert.Empty(t, res.Outcome.Violations)
		})
	}
}

func TestApplyTrustedOrgs(t *testing.T) {
	pol := Policy{Lint: Lint{Security: Security{TrustedOrgs: List{Set: true, Items: []string{"acme", "anthropics"}}}}}
	tests := []struct {
		name     string
		repo     []string
		want     []string
		wantViol int
		accepted bool
	}{
		{"unset takes the policy list", nil, []string{"acme", "anthropics"}, 0, false},
		{"a narrower list is accepted", []string{"Acme"}, []string{"acme"}, 0, true},
		{"an outside org is dropped and reported", []string{"acme", "evil"}, []string{"acme"}, 1, true},
		{"nothing left falls back to the policy list", []string{"evil"}, []string{"acme", "anthropics"}, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "[lint.security]\ntrusted_orgs = []\n")
			cfg.Lint = &config.LintConfig{Security: &config.LintSecurity{TrustedOrgs: tt.repo}}
			// Act
			res := Resolve([]Layer{layer("managed", pol)}).Apply(cfg)
			// Assert
			assert.Equal(t, tt.want, cfg.Lint.Security.TrustedOrgs)
			require.Len(t, res.Outcome.Violations, tt.wantViol)
			for _, v := range res.Outcome.Violations {
				assert.Equal(t, "AR740", v.Code)
				assert.Equal(t, "lint.security.trusted_orgs", v.Key)
				assert.Equal(t, "managed", v.Origin)
			}
			assert.Equal(t, tt.accepted, len(res.Accepted) > 0)
		})
	}
}

func TestApplyEmptyTrustedOrgsPolicyTrustsNobody(t *testing.T) {
	cfg := testConfig(t, "")
	cfg.Lint = &config.LintConfig{Security: &config.LintSecurity{TrustedOrgs: []string{"acme"}}}
	res := Resolve([]Layer{layer("managed", Policy{Lint: Lint{Security: Security{TrustedOrgs: List{Set: true}}}})}).Apply(cfg)
	assert.Equal(t, []string{noHostSentinel}, cfg.Lint.Security.TrustedOrgs, "an empty list would restore the built-in orgs")
	assert.Len(t, res.Outcome.Violations, 1)
}

func TestApplyMaxNetworkCommands(t *testing.T) {
	tests := []struct {
		name     string
		policy   int
		repo     *int
		want     int
		wantViol int
		accepted bool
	}{
		{"unset takes the policy limit", 3, nil, 3, 0, false},
		{"a lower limit is accepted", 3, intPtr(1), 1, 0, true},
		{"an equal limit is accepted", 3, intPtr(3), 3, 0, false},
		{"a higher limit is clamped and reported", 3, intPtr(10), 3, 1, false},
		{"a bound above the default does not raise an unset limit", 9, nil, 5, 0, false},
		{"a bound above the default still clamps a higher value", 9, intPtr(20), 9, 1, false},
		{"zero is the lowest limit", 0, intPtr(2), 0, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "[lint.capability]\nmax_network_commands = 10\n")
			cfg.Lint = &config.LintConfig{Capability: &config.LintCapability{MaxNetworkCommands: tt.repo}}
			pol := Policy{Lint: Lint{Capability: Capability{MaxNetworkCommands: intPtr(tt.policy)}}}
			// Act
			res := Resolve([]Layer{layer("env", pol)}).Apply(cfg)
			// Assert
			require.NotNil(t, cfg.Lint.Capability.MaxNetworkCommands)
			assert.Equal(t, tt.want, *cfg.Lint.Capability.MaxNetworkCommands)
			require.Len(t, res.Outcome.Violations, tt.wantViol)
			for _, v := range res.Outcome.Violations {
				assert.Equal(t, "AR740", v.Code)
				assert.Equal(t, "lint.capability.max_network_commands", v.Key)
				assert.Equal(t, "env", v.Origin)
				assert.Equal(t, 2, v.Line, "points at the repository's key")
			}
			assert.Equal(t, tt.accepted, len(res.Accepted) > 0)
		})
	}
}

func TestApplyLoadBudgets(t *testing.T) {
	pol := Policy{Lint: Lint{LoadBudgets: map[string]int{"claude-skill-listing": 1000}}}
	tests := []struct {
		name     string
		repo     map[string]int
		want     map[string]int
		wantViol int
		accepted bool
	}{
		{"unset takes the policy budget", nil, map[string]int{"claude-skill-listing": 1000}, 0, false},
		{"a lower budget is accepted", map[string]int{"claude-skill-listing": 500}, map[string]int{"claude-skill-listing": 500}, 0, true},
		{"a higher budget is clamped and reported", map[string]int{"claude-skill-listing": 5000}, map[string]int{"claude-skill-listing": 1000}, 1, false},
		{"other budgets are untouched", map[string]int{"cursor-rule-lines": 9000}, map[string]int{"claude-skill-listing": 1000, "cursor-rule-lines": 9000}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "[lint.load_budgets]\nclaude-skill-listing = 5000\n")
			cfg.Lint = &config.LintConfig{LoadBudgets: tt.repo}
			// Act
			res := Resolve([]Layer{layer("managed", pol)}).Apply(cfg)
			// Assert
			assert.Equal(t, tt.want, cfg.Lint.LoadBudgets)
			require.Len(t, res.Outcome.Violations, tt.wantViol)
			for _, v := range res.Outcome.Violations {
				assert.Equal(t, "AR740", v.Code)
				assert.Equal(t, "lint.load_budgets.claude-skill-listing", v.Key)
				assert.Equal(t, "managed", v.Origin)
			}
			assert.Equal(t, tt.accepted, len(res.Accepted) > 0)
		})
	}
}

func TestApplyLoadBudgetAboveTheBuiltInDoesNotRaiseIt(t *testing.T) {
	cfg := testConfig(t, "")
	pol := Policy{Lint: Lint{LoadBudgets: map[string]int{"claude-skill-listing": 99999}}}
	Resolve([]Layer{layer("managed", pol)}).Apply(cfg)
	assert.Equal(t, map[string]int{"claude-skill-listing": lint.LoadBudgetDefault("claude-skill-listing")}, cfg.Lint.LoadBudgets)
}

func TestLintKnobsShowInThePolicyView(t *testing.T) {
	// Arrange
	pol := Policy{Lint: Lint{
		Security:    Security{DirectiveTags: []string{"assistant"}, TrustedOrgs: List{Set: true, Items: []string{"acme"}}},
		Capability:  Capability{MaxNetworkCommands: intPtr(2)},
		LoadBudgets: map[string]int{"claude-skill-listing": 900},
	}}
	// Act
	res := Resolve([]Layer{layer("flag", pol)})
	flat := map[string]string{}
	for _, e := range flatten(res.Policy.Tree()) {
		flat[e.key] = e.value
	}
	// Assert
	assert.Equal(t, `["assistant"]`, flat["lint.security.directive_tags"])
	assert.Equal(t, `["acme"]`, flat["lint.security.trusted_orgs"])
	assert.Equal(t, "2", flat["lint.capability.max_network_commands"])
	assert.Equal(t, "900", flat["lint.load_budgets.claude-skill-listing"])
	for key := range flat {
		assert.Equal(t, "flag", res.Provenance[key], "origin of %s", key)
	}
}
