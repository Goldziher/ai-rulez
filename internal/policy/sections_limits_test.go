package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const week = 7 * 24 * time.Hour

func TestParseLimits(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[sources]\nmin_release_age = \"1w\"\n[lint.budgets.skill]\nmax_tokens = 4000\n[lint.budgets.rule]\nmax_lines = 100\nmax_tokens = 1000\n"

	// Act
	_, p, err := Parse("p.toml", []byte(body))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, week, p.Sources.MinReleaseAge)
	assert.Equal(t, map[string]SizeBudget{"skill": {MaxTokens: 4000}, "rule": {MaxLines: 100, MaxTokens: 1000}}, p.Lint.SizeBudgets)
}

func TestParseLimitsRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"not an age", "policy_version = 1\n[sources]\nmin_release_age = \"soon\"\n", "min_release_age"},
		{"negative age", "policy_version = 1\n[sources]\nmin_release_age = \"-1d\"\n", "min_release_age"},
		{"unknown kind", "policy_version = 1\n[lint.budgets.poem]\nmax_lines = 3\n", "unknown content kind"},
		{"zero lines", "policy_version = 1\n[lint.budgets.rule]\nmax_lines = 0\n", "must be positive"},
		{"unknown budget key", "policy_version = 1\n[lint.budgets.rule]\nmax_words = 3\n", "unknown key"},
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

func TestMergeLimits(t *testing.T) {
	a := Policy{
		Sources: Sources{MinReleaseAge: 3 * 24 * time.Hour},
		Lint:    Lint{SizeBudgets: map[string]SizeBudget{"rule": {MaxLines: 100}, "skill": {MaxTokens: 4000}}},
	}
	b := Policy{
		Sources: Sources{MinReleaseAge: week},
		Lint:    Lint{SizeBudgets: map[string]SizeBudget{"rule": {MaxLines: 150, MaxTokens: 900}}},
	}

	got := Merge(a, b)

	assert.Equal(t, week, got.Sources.MinReleaseAge, "the longer age floor")
	assert.Equal(t, map[string]SizeBudget{"rule": {MaxLines: 100, MaxTokens: 900}, "skill": {MaxTokens: 4000}}, got.Lint.SizeBudgets, "the lower bound per kind and field")
	assert.Equal(t, got, Merge(b, a))
	assert.Equal(t, a.Lint.SizeBudgets, Merge(a, Policy{}).Lint.SizeBudgets)
}

func TestApplyMinReleaseAge(t *testing.T) {
	tests := []struct {
		name         string
		lock         string
		include      string
		wantLock     string
		wantInclude  string
		wantViol     []string
		wantAccepted bool
	}{
		{"unset values take the floor", "", "", "7d", "", nil, false},
		{"a younger [lock] age is raised and reported", "2d", "", "7d", "", []string{"AR740 lock.min_release_age"}, false},
		{"an explicit zero is a younger age", "0", "", "7d", "", []string{"AR740 lock.min_release_age"}, false},
		{"an older [lock] age is accepted", "30d", "", "30d", "", nil, true},
		{"a per-source age below the floor is raised and reported", "30d", "1d", "30d", "7d", []string{"AR740 includes.shared.min_release_age"}, true},
		{"a per-source age above the floor is accepted", "", "14d", "7d", "14d", nil, true},
		{"hours below the floor are reported", "100h", "", "7d", "", []string{"AR740 lock.min_release_age"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "name = \"x\"\n")
			cfg.Lock = &config.LockConfig{MinReleaseAge: tt.lock}
			cfg.Includes = []config.IncludeConfig{{Name: "shared", Source: "github.com/a/b", Version: "^1", MinReleaseAge: tt.include}}
			res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{MinReleaseAge: week}})})
			// Act
			result := res.Apply(cfg)
			// Assert
			assert.Equal(t, tt.wantLock, cfg.Lock.MinReleaseAge)
			assert.Equal(t, tt.wantInclude, cfg.Includes[0].MinReleaseAge)
			assert.ElementsMatch(t, tt.wantViol, codes(result.Outcome))
			assert.Equal(t, tt.wantAccepted, len(result.Accepted) > 0, "%v", result.Accepted)
		})
	}
}

func TestApplyMinReleaseAgeCoversInstalledSkillsAndSkillSources(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.InstalledSkills = []config.InstalledSkillConfig{{Name: "s", Source: "github.com/a/b", Version: "^1", MinReleaseAge: "1d"}}
	cfg.SkillSources = []config.SkillSourceConfig{{Name: "src", URL: "github.com/a/c", Version: "^1", MinReleaseAge: "2d"}}
	res := Resolve([]Layer{layer("env", Policy{Sources: Sources{MinReleaseAge: week}})})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	assert.Equal(t, "7d", cfg.InstalledSkills[0].MinReleaseAge)
	assert.Equal(t, "7d", cfg.SkillSources[0].MinReleaseAge)
	assert.ElementsMatch(t, []string{"AR740 installed_skills.s.min_release_age", "AR740 skill_sources.src.min_release_age"}, codes(out))
	assert.Equal(t, "env", out.Violations[0].Origin)
}

func TestApplySizeBudgets(t *testing.T) {
	pol := map[string]SizeBudget{"skill": {MaxLines: 400, MaxTokens: 6000}, "rule": {MaxTokens: 1000}}
	tests := []struct {
		name         string
		repo         map[string]config.LintBudget
		want         map[string]config.LintBudget
		wantViol     []string
		wantAccepted bool
	}{
		{
			"unset values run at the lower of the bound and the built-in",
			nil,
			map[string]config.LintBudget{"skill": {MaxLines: 400, MaxTokens: 5000}, "rule": {MaxTokens: 1000}},
			nil, false,
		},
		{
			"a higher repository budget is lowered and reported",
			map[string]config.LintBudget{"skill": {MaxLines: 900, MaxTokens: 9000}, "rule": {MaxTokens: 5000}},
			map[string]config.LintBudget{"skill": {MaxLines: 400, MaxTokens: 6000}, "rule": {MaxTokens: 1000}},
			[]string{"AR740 lint.budgets.skill.max_lines", "AR740 lint.budgets.skill.max_tokens", "AR740 lint.budgets.rule.max_tokens"}, false,
		},
		{
			"a lower repository budget is accepted",
			map[string]config.LintBudget{"skill": {MaxLines: 100, MaxTokens: 100}, "rule": {MaxTokens: 10}},
			map[string]config.LintBudget{"skill": {MaxLines: 100, MaxTokens: 100}, "rule": {MaxTokens: 10}},
			nil, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "name = \"x\"\n")
			cfg.Lint = &config.LintConfig{Budgets: tt.repo}
			res := Resolve([]Layer{layer("managed", Policy{Lint: Lint{SizeBudgets: pol}})})
			// Act
			result := res.Apply(cfg)
			// Assert
			assert.Equal(t, tt.want, cfg.Lint.Budgets)
			assert.ElementsMatch(t, tt.wantViol, codes(result.Outcome))
			assert.Equal(t, tt.wantAccepted, len(result.Accepted) > 0, "%v", result.Accepted)
		})
	}
}
