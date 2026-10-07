package config

import (
	"fmt"
	"math"
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// Values of [improve] isolation: how `improve run` confines the optimizer.
const (
	ImproveIsolationAuto    = "auto"
	ImproveIsolationNone    = "none"
	ImproveIsolationRequire = "require"
)

// ImproveMaxSkillGrowth is the largest [improve] max_skill_growth accepted.
const ImproveMaxSkillGrowth = 2.0

// ImproveMinHoldoutCases is the fewest held-out cases `improve` ever accepts; min_holdout_cases may raise it.
const ImproveMinHoldoutCases = 3

// Defaults of the acceptance gate. A repository [improve] table may only tighten them (ResolveImprove);
// internal/improve keeps its own copies, which a test holds equal to these.
const (
	ImproveDefaultMinGain         = 0.05
	ImproveDefaultMaxRegressions  = 0
	ImproveDefaultHoldoutFraction = 0.3
	ImproveDefaultMaxSkillGrowth  = 1.25
	ImproveDefaultRuns            = 3
	ImproveDefaultMaxRounds       = 3
	ImproveDefaultMaxHoldoutEvals = 3
)

// ImproveConfig is the [improve] table: defaults of `ai-rulez improve run` (docs/improve.md). A flag always
// wins. optimizer and env_pass choose a command and the environment it receives, so a repository config
// is honored for them only with --trust-repo-optimizer; the thresholds apply from any config.
//
//nolint:tagliatelle // config keys are snake_case by project convention
type ImproveConfig struct {
	// HoldoutTag marks the cases held out from the optimizer (default "holdout").
	HoldoutTag string `yaml:"holdout_tag,omitempty" json:"holdout_tag,omitempty" toml:"holdout_tag,omitempty"`
	// HoldoutFraction is the deterministic held-out share when no case carries the tag (default 0.3).
	HoldoutFraction *float64 `yaml:"holdout_fraction,omitempty" json:"holdout_fraction,omitempty" toml:"holdout_fraction,omitempty"`
	// MinHoldoutCases is the fewest scored held-out cases a run needs (default 3; it cannot go lower).
	MinHoldoutCases int `yaml:"min_holdout_cases,omitempty" json:"min_holdout_cases,omitempty" toml:"min_holdout_cases,omitempty"`
	// MinGain is the held-out pass-rate gain a candidate needs (default 0.05).
	MinGain *float64 `yaml:"min_gain,omitempty" json:"min_gain,omitempty" toml:"min_gain,omitempty"`
	// MaxRegressions is how many held-out cases may flip from pass to fail (default 0).
	MaxRegressions *int `yaml:"max_regressions,omitempty" json:"max_regressions,omitempty" toml:"max_regressions,omitempty"`
	// Runs is the eval runs per case and arm (default 3).
	Runs int `yaml:"runs,omitempty" json:"runs,omitempty" toml:"runs,omitempty"`
	// MaxRounds is the optimizer invocations (default 3).
	MaxRounds int `yaml:"max_rounds,omitempty" json:"max_rounds,omitempty" toml:"max_rounds,omitempty"`
	// MaxHoldoutEvals is how often the held-out set may be evaluated (default 3).
	MaxHoldoutEvals int `yaml:"max_holdout_evals,omitempty" json:"max_holdout_evals,omitempty" toml:"max_holdout_evals,omitempty"`
	// MaxSkillGrowth bounds SKILL.md growth as a factor of its original tokens (default 1.25, at most 2).
	MaxSkillGrowth float64 `yaml:"max_skill_growth,omitempty" json:"max_skill_growth,omitempty" toml:"max_skill_growth,omitempty"`
	// RequireCIAboveZero makes the bootstrap interval of the gain a gate condition (default false).
	RequireCIAboveZero bool `yaml:"require_ci_above_zero,omitempty" json:"require_ci_above_zero,omitempty" toml:"require_ci_above_zero,omitempty"`
	// Isolation is none (default: the optimizer runs as you), auto (confine when a backend works) or require (refuse without one).
	Isolation string `yaml:"isolation,omitempty" json:"isolation,omitempty" toml:"isolation,omitempty"`
	// EnvPass names environment variables forwarded to the optimizer. Used from a repository config only with --trust-repo-optimizer.
	EnvPass []string `yaml:"env_pass,omitempty" json:"env_pass,omitempty" toml:"env_pass,omitempty"`
	// Optimizer is the optimizer command (argv, no shell). Used from a repository config only with --trust-repo-optimizer.
	Optimizer string `yaml:"optimizer,omitempty" json:"optimizer,omitempty" toml:"optimizer,omitempty"`
}

// Validate returns the problems of the table, one message each.
func (c *ImproveConfig) Validate() []string {
	if c == nil {
		return nil
	}
	var problems []string
	if strings.ContainsAny(c.HoldoutTag, " \t\n") {
		problems = append(problems, "improve.holdout_tag must not contain whitespace")
	}
	if f := c.HoldoutFraction; f != nil && (*f < 0 || *f >= 1 || math.IsNaN(*f)) {
		problems = append(problems, fmt.Sprintf("improve.holdout_fraction must be in [0, 1), got %v", *f))
	}
	if c.MinHoldoutCases != 0 && c.MinHoldoutCases < ImproveMinHoldoutCases {
		problems = append(problems, fmt.Sprintf("improve.min_holdout_cases must be at least %d, got %d", ImproveMinHoldoutCases, c.MinHoldoutCases))
	}
	if g := c.MinGain; g != nil && (*g < 0 || *g > 1 || math.IsNaN(*g)) {
		problems = append(problems, fmt.Sprintf("improve.min_gain must be between 0 and 1, got %v", *g))
	}
	if r := c.MaxRegressions; r != nil && *r < 0 {
		problems = append(problems, "improve.max_regressions must not be negative")
	}
	for name, v := range map[string]int{"runs": c.Runs, "max_rounds": c.MaxRounds, "max_holdout_evals": c.MaxHoldoutEvals} {
		if v < 0 {
			problems = append(problems, fmt.Sprintf("improve.%s must not be negative", name))
		}
	}
	if g := c.MaxSkillGrowth; g != 0 && (g < 1 || g > ImproveMaxSkillGrowth || math.IsNaN(g)) {
		problems = append(problems, fmt.Sprintf("improve.max_skill_growth must be between 1 and %v, got %v", ImproveMaxSkillGrowth, g))
	}
	switch c.Isolation {
	case "", ImproveIsolationAuto, ImproveIsolationNone, ImproveIsolationRequire:
	default:
		problems = append(problems, fmt.Sprintf("improve.isolation %q must be %q, %q or %q", c.Isolation, ImproveIsolationAuto, ImproveIsolationNone, ImproveIsolationRequire))
	}
	for _, name := range c.EnvPass {
		if name == "" || strings.ContainsAny(name, "= \t\n") {
			problems = append(problems, fmt.Sprintf("improve.env_pass entry %q is not an environment variable name", name))
		}
	}
	return problems
}

// HasTrustSensitive reports whether the table sets a key that must not take effect from a repository
// config without --trust-repo-optimizer: the optimizer command and the environment it receives.
func (c *ImproveConfig) HasTrustSensitive() bool {
	return c != nil && (strings.TrimSpace(c.Optimizer) != "" || len(c.EnvPass) > 0)
}

// ImproveResolution is the effective [improve] table of one `improve run`.
type ImproveResolution struct {
	// Effective is the merged table (never nil).
	Effective ImproveConfig
	// IgnoredRepoKeys names the repository keys that were not used because the run did not trust the repository.
	IgnoredRepoKeys []string
	// LoosenedRepoKeys names the repository gate keys that were not used because they are looser than the
	// default (a lower min_gain, more max_regressions, a smaller holdout_fraction, a larger max_skill_growth)
	// and the run did not trust the repository.
	LoosenedRepoKeys []string
	// UserFile is the user config file that was read ("" when none exists).
	UserFile string
}

// loadUserImprove reads the [improve] table of the user config file; a missing file or table is not an error.
func loadUserImprove(path string) (*ImproveConfig, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user's own config file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", path).Wrapf(err, "read user config")
	}
	var doc struct {
		Improve *ImproveConfig `toml:"improve"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse user config")
	}
	return doc.Improve, nil
}

// ResolveImprove merges [improve] from the repository and the user config file. The user file wins key by key;
// a repository optimizer and env_pass are used only when trustRepo is set (a hostile repository must not choose
// a command that runs on your machine or the variables it receives), and a repository gate key looser than the
// default is dropped unless trustRepo is set (LoosenedRepoKeys). getenv may be nil (os.Getenv).
func (c *Config) ResolveImprove(trustRepo bool, getenv func(string) string) (ImproveResolution, error) {
	if getenv == nil {
		getenv = func(name string) string { return ambient.Getenv(nil, name) }
	}
	var out ImproveResolution
	if c != nil && c.Improve != nil {
		out.Effective = *c.Improve
		if c.Improve.HasTrustSensitive() && !trustRepo {
			if strings.TrimSpace(c.Improve.Optimizer) != "" {
				out.IgnoredRepoKeys = append(out.IgnoredRepoKeys, "optimizer")
			}
			if len(c.Improve.EnvPass) > 0 {
				out.IgnoredRepoKeys = append(out.IgnoredRepoKeys, "env_pass")
			}
			out.Effective.Optimizer, out.Effective.EnvPass = "", nil
		}
		if !trustRepo {
			out.LoosenedRepoKeys = out.Effective.dropLooseGateKeys()
		}
	}
	path := UserConfigFile(getenv)
	user, err := loadUserImprove(path)
	if err != nil {
		return out, err
	}
	if user == nil {
		return out, nil
	}
	out.UserFile = path
	if problems := user.Validate(); len(problems) > 0 {
		return out, oops.Errorf("user config [improve]: %s", strings.Join(problems, "; "))
	}
	mergeImprove(&out.Effective, user)
	return out, nil
}

// LooserGateKeys names the gate keys of c that are looser than the defaults. A repository config may tighten the
// acceptance gate but must not weaken it: a hostile or careless table could otherwise accept a candidate that gained
// nothing (min_gain 0), tolerate regressions, shrink the held-out share or allow a much larger skill.
// require_ci_above_zero and min_holdout_cases only ever tighten.
func (c *ImproveConfig) LooserGateKeys() []string {
	if c == nil {
		return nil
	}
	var loose []string
	if g := c.MinGain; g != nil && *g+1e-9 < ImproveDefaultMinGain {
		loose = append(loose, "min_gain")
	}
	if r := c.MaxRegressions; r != nil && *r > ImproveDefaultMaxRegressions {
		loose = append(loose, "max_regressions")
	}
	if f := c.HoldoutFraction; f != nil && *f+1e-9 < ImproveDefaultHoldoutFraction {
		loose = append(loose, "holdout_fraction")
	}
	if g := c.MaxSkillGrowth; g > ImproveDefaultMaxSkillGrowth+1e-9 {
		loose = append(loose, "max_skill_growth")
	}
	// Fewer runs weaken the confidence interval; a larger round or held-out budget lets a candidate be tuned
	// against the held-out set and spends more on the optimizer.
	if c.Runs > 0 && c.Runs < ImproveDefaultRuns {
		loose = append(loose, "runs")
	}
	if c.MaxRounds > ImproveDefaultMaxRounds {
		loose = append(loose, "max_rounds")
	}
	if c.MaxHoldoutEvals > ImproveDefaultMaxHoldoutEvals {
		loose = append(loose, "max_holdout_evals")
	}
	return loose
}

// dropLooseGateKeys unsets the gate keys LooserGateKeys names and returns them.
func (c *ImproveConfig) dropLooseGateKeys() []string {
	loose := c.LooserGateKeys()
	for _, key := range loose {
		switch key {
		case "min_gain":
			c.MinGain = nil
		case "max_regressions":
			c.MaxRegressions = nil
		case "holdout_fraction":
			c.HoldoutFraction = nil
		case "max_skill_growth":
			c.MaxSkillGrowth = 0
		case "runs":
			c.Runs = 0
		case "max_rounds":
			c.MaxRounds = 0
		case "max_holdout_evals":
			c.MaxHoldoutEvals = 0
		}
	}
	return loose
}

// mergeImprove copies the keys user sets over dst.
func mergeImprove(dst, user *ImproveConfig) {
	if user.HoldoutTag != "" {
		dst.HoldoutTag = user.HoldoutTag
	}
	if user.HoldoutFraction != nil {
		dst.HoldoutFraction = user.HoldoutFraction
	}
	if user.MinHoldoutCases != 0 {
		dst.MinHoldoutCases = user.MinHoldoutCases
	}
	if user.MinGain != nil {
		dst.MinGain = user.MinGain
	}
	if user.MaxRegressions != nil {
		dst.MaxRegressions = user.MaxRegressions
	}
	if user.Runs != 0 {
		dst.Runs = user.Runs
	}
	if user.MaxRounds != 0 {
		dst.MaxRounds = user.MaxRounds
	}
	if user.MaxHoldoutEvals != 0 {
		dst.MaxHoldoutEvals = user.MaxHoldoutEvals
	}
	if user.MaxSkillGrowth != 0 {
		dst.MaxSkillGrowth = user.MaxSkillGrowth
	}
	dst.RequireCIAboveZero = dst.RequireCIAboveZero || user.RequireCIAboveZero
	if user.Isolation != "" {
		dst.Isolation = user.Isolation
	}
	if user.Optimizer != "" {
		dst.Optimizer = user.Optimizer
	}
	if len(user.EnvPass) > 0 {
		dst.EnvPass = user.EnvPass
	}
}
