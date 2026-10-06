package config

import (
	"fmt"
	"path"
	"strings"
)

// Values of [review] content: what an item sends to a judge.
const (
	ReviewContentDescriptions = "descriptions"
	ReviewContentFull         = "full"
)

// Values of [review] on_secret: what happens to an item that holds a credential.
const (
	ReviewOnSecretWithhold = "withhold"
	ReviewOnSecretRedact   = "redact"
)

// Values of [review.gate] level: the lowest severity ceiling of a dimension whose stable
// fail verdict fails the gate. A judge never reports above warning, so "error" gates nothing
// a judge says.
const (
	ReviewGateInfo    = "info"
	ReviewGateWarning = "warning"
	ReviewGateError   = "error"
)

// BuiltinRubricPrefix marks a rubric id that names an embedded rubric.
const BuiltinRubricPrefix = "builtin:"

// DefaultReviewRubric is the rubric `ai-rulez review` uses when none is set.
const DefaultReviewRubric = BuiltinRubricPrefix + "skill-quality"

// DefaultReviewGateMaxAgeDays is how old a calibration record may be when neither the
// rubric nor [review.gate] sets a limit.
const DefaultReviewGateMaxAgeDays = 90

// DefaultReviewFixMaxGrowthPercent is how much a proposed fix may grow an item.
const DefaultReviewFixMaxGrowthPercent = 25

// ReviewConfig is the [review] table: which rubric scores the content, what an item
// sends to a judge, the spend ceilings of a judged run, and the gate and fix settings
// (docs/review.md). allowed_hosts is honoured from user scope only.
type ReviewConfig struct {
	// Rubric is the rubric id: `builtin:skill-quality` or a directory under .ai-rulez/rubrics.
	Rubric string `yaml:"rubric,omitempty" json:"rubric,omitempty" toml:"rubric,omitempty"`
	// Content is descriptions (name, description and frontmatter keys; the default) or full (adds the body).
	Content string `yaml:"content,omitempty" json:"content,omitempty" toml:"content,omitempty"`
	// Exclude lists name globs of items the review skips.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
	// MaxCostUSD is the spend ceiling of one run (0 = unset). A repository value can only
	// lower the default; the user config file and --max-cost can set any value.
	MaxCostUSD float64 `yaml:"max_cost_usd,omitempty" json:"max_cost_usd,omitempty" toml:"max_cost_usd,omitempty"` //nolint:tagliatelle
	// MaxCalls is the call ceiling of one run (0 = unset); the same trust rule as MaxCostUSD.
	MaxCalls int `yaml:"max_calls,omitempty" json:"max_calls,omitempty" toml:"max_calls,omitempty"` //nolint:tagliatelle
	// OnSecret is withhold (default: an item holding a credential is never sent) or redact (the
	// credential is masked and the item is sent, marked redacted).
	OnSecret string `yaml:"on_secret,omitempty" json:"on_secret,omitempty" toml:"on_secret,omitempty"` //nolint:tagliatelle
	// AllowedHosts, when set in user scope, is the only list of hosts a judged run may send to.
	// A repository config cannot set it: it is ignored and reported.
	AllowedHosts []string `yaml:"allowed_hosts,omitempty" json:"allowed_hosts,omitempty" toml:"allowed_hosts,omitempty"` //nolint:tagliatelle
	// Gate is the [review.gate] table.
	Gate *ReviewGateConfig `yaml:"gate,omitempty" json:"gate,omitempty" toml:"gate,omitempty"`
	// Fix is the [review.fix] table.
	Fix *ReviewFixConfig `yaml:"fix,omitempty" json:"fix,omitempty" toml:"fix,omitempty"`
}

// ReviewGateConfig is the [review.gate] table: when --gate may fail a build.
type ReviewGateConfig struct {
	// Level is the lowest severity ceiling of a dimension whose stable fail verdict fails the gate.
	Level string `yaml:"level,omitempty" json:"level,omitempty" toml:"level,omitempty"`
	// RequireCalibration, when false, lets --gate run without a matching calibration record
	// (AR9G9 still reports it). Nil means true.
	RequireCalibration *bool `yaml:"require_calibration,omitempty" json:"require_calibration,omitempty" toml:"require_calibration,omitempty"` //nolint:tagliatelle
	// CalibrationMaxAgeDays overrides the rubric's calibration.max_age_days.
	CalibrationMaxAgeDays int `yaml:"calibration_max_age_days,omitempty" json:"calibration_max_age_days,omitempty" toml:"calibration_max_age_days,omitempty"` //nolint:tagliatelle
}

// ReviewFixConfig is the [review.fix] table.
type ReviewFixConfig struct {
	// Model writes the fix. It must differ from the judge model unless --allow-same-model is given.
	Model string `yaml:"model,omitempty" json:"model,omitempty" toml:"model,omitempty"`
	// MaxGrowthPercent bounds how much a fix may grow an item (default 25).
	MaxGrowthPercent int `yaml:"max_growth_percent,omitempty" json:"max_growth_percent,omitempty" toml:"max_growth_percent,omitempty"` //nolint:tagliatelle
}

// OnSecretMode returns the effective on_secret value.
func (r *ReviewConfig) OnSecretMode() string {
	if r == nil || r.OnSecret == "" {
		return ReviewOnSecretWithhold
	}
	return r.OnSecret
}

// GateLevel returns the effective gate level.
func (r *ReviewConfig) GateLevel() string {
	if r == nil || r.Gate == nil || r.Gate.Level == "" {
		return ReviewGateWarning
	}
	return r.Gate.Level
}

// GateRequiresCalibration reports whether --gate needs a matching calibration record.
func (r *ReviewConfig) GateRequiresCalibration() bool {
	return r == nil || r.Gate == nil || r.Gate.RequireCalibration == nil || *r.Gate.RequireCalibration
}

// FixMaxGrowthPercent returns the effective growth limit of a fix.
func (r *ReviewConfig) FixMaxGrowthPercent() int {
	if r == nil || r.Fix == nil || r.Fix.MaxGrowthPercent <= 0 {
		return DefaultReviewFixMaxGrowthPercent
	}
	return r.Fix.MaxGrowthPercent
}

// Validate returns the problems of the table, one message each.
func (r *ReviewConfig) Validate() []string {
	if r == nil {
		return nil
	}
	var problems []string
	switch r.Content {
	case "", ReviewContentDescriptions, ReviewContentFull:
	default:
		problems = append(problems, fmt.Sprintf("review.content %q must be %q or %q", r.Content, ReviewContentDescriptions, ReviewContentFull))
	}
	switch r.OnSecret {
	case "", ReviewOnSecretWithhold, ReviewOnSecretRedact:
	default:
		problems = append(problems, fmt.Sprintf("review.on_secret %q must be %q or %q", r.OnSecret, ReviewOnSecretWithhold, ReviewOnSecretRedact))
	}
	if r.MaxCostUSD < 0 {
		problems = append(problems, "review.max_cost_usd must not be negative")
	}
	if r.MaxCalls < 0 {
		problems = append(problems, "review.max_calls must not be negative")
	}
	for _, g := range r.Exclude {
		if _, err := path.Match(g, ""); err != nil || strings.TrimSpace(g) == "" {
			problems = append(problems, fmt.Sprintf("review.exclude pattern %q is not a valid glob", g))
		}
	}
	for _, h := range r.AllowedHosts {
		if h == "" || strings.ContainsAny(h, "/@?# \t") || strings.Contains(h, "://") {
			problems = append(problems, fmt.Sprintf("review.allowed_hosts entry %q must be host or host:port, without scheme or path", h))
		}
	}
	if g := r.Gate; g != nil {
		switch g.Level {
		case "", ReviewGateInfo, ReviewGateWarning, ReviewGateError:
		default:
			problems = append(problems, fmt.Sprintf("review.gate.level %q must be %q, %q or %q", g.Level, ReviewGateInfo, ReviewGateWarning, ReviewGateError))
		}
		if g.CalibrationMaxAgeDays < 0 {
			problems = append(problems, "review.gate.calibration_max_age_days must not be negative")
		}
	}
	if f := r.Fix; f != nil && (f.MaxGrowthPercent < 0 || strings.ContainsAny(f.Model, " \t\n")) {
		problems = append(problems, "review.fix.max_growth_percent must not be negative and review.fix.model must not contain whitespace")
	}
	return problems
}
