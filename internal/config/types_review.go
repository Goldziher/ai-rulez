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

// BuiltinRubricPrefix marks a rubric id that names an embedded rubric.
const BuiltinRubricPrefix = "builtin:"

// DefaultReviewRubric is the rubric `ai-rulez review` uses when none is set.
const DefaultReviewRubric = BuiltinRubricPrefix + "skill-quality"

// ReviewConfig is the [review] table: which rubric scores the content, what an
// item would send to a judge, and the spend ceilings recorded for the phases
// that call a model. Phase 0 makes no call, so the ceilings only bound the
// estimate (docs/review.md).
type ReviewConfig struct {
	// Rubric is the rubric id: `builtin:skill-quality` or a directory under .ai-rulez/rubrics.
	Rubric string `yaml:"rubric,omitempty" json:"rubric,omitempty" toml:"rubric,omitempty"`
	// Content is descriptions (name, description and frontmatter keys; the default) or full (adds the body).
	Content string `yaml:"content,omitempty" json:"content,omitempty" toml:"content,omitempty"`
	// Exclude lists name globs of items the review skips.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
	// MaxCostUSD is the spend ceiling of one run; the estimate is refused above it (0 = unset).
	MaxCostUSD float64 `yaml:"max_cost_usd,omitempty" json:"max_cost_usd,omitempty" toml:"max_cost_usd,omitempty"` //nolint:tagliatelle
	// MaxCalls is the call ceiling of one run; the estimate is refused above it (0 = unset).
	MaxCalls int `yaml:"max_calls,omitempty" json:"max_calls,omitempty" toml:"max_calls,omitempty"` //nolint:tagliatelle
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
	return problems
}
