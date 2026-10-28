package config

import (
	"fmt"
	"math"
)

// Values of [exposure] harness: the harness whose load model `exposure` applies.
const ExposureHarnessClaude = "claude"

// Values of [exposure] plugins: which plugin skills count as start-of-session skills.
const (
	ExposurePluginsNone    = "none"
	ExposurePluginsEnabled = "enabled"
	ExposurePluginsAll     = "all"
)

// ExposureDefaultContextWindow is the context window, in tokens, when [exposure] context_window is unset.
const ExposureDefaultContextWindow = 200000

// ExposureDefaultCollisionMinScore is the word overlap at which two listings collide when [exposure]
// collision_min_score is unset: the default of `exposure collisions --min-score`, and AR9W2's when a severity
// turns it on.
const ExposureDefaultCollisionMinScore = 0.1

// ExposureDefaultCollisionLimit is the number of pairs `exposure collisions` prints by default, and AR9W2
// reports one by one before it sums up the rest.
const ExposureDefaultCollisionLimit = 20

// ExposureConfig is the [exposure] table: what a session sees at start and the area map that names which part
// of the repository an artifact belongs to (docs/exposure.md). The map is either inline ([[exposure.areas]] and
// [exposure.skill_areas]) or in areas_file, never both. Every key is optional; the rules that read the table are
// inert without it.
//
//nolint:tagliatelle // config keys are snake_case by project convention
type ExposureConfig struct {
	// Harness is the harness whose load model applies; only "claude" (the default).
	Harness string `yaml:"harness,omitempty" json:"harness,omitempty" toml:"harness,omitempty"`
	// ContextWindow is the model context window in tokens (default 200000).
	ContextWindow int `yaml:"context_window,omitempty" json:"context_window,omitempty" toml:"context_window,omitempty"`
	// Plugins selects the plugin skills counted at start: none, enabled (default) or all.
	Plugins string `yaml:"plugins,omitempty" json:"plugins,omitempty" toml:"plugins,omitempty"`
	// AreasFile is a TOML file, relative to the project root, holding [[area]] tables and [skill_areas].
	AreasFile string `yaml:"areas_file,omitempty" json:"areas_file,omitempty" toml:"areas_file,omitempty"`
	// StartBudget is the most estimated start-of-session tokens allowed; 0 turns the check off.
	StartBudget int `yaml:"start_budget,omitempty" json:"start_budget,omitempty" toml:"start_budget,omitempty"`
	// CollisionMinScore is the word overlap (0 to 1) at which two listings of different areas collide; 0 is off.
	CollisionMinScore float64 `yaml:"collision_min_score,omitempty" json:"collision_min_score,omitempty" toml:"collision_min_score,omitempty"`
	// Areas is the inline area map.
	Areas []ExposureArea `yaml:"areas,omitempty" json:"areas,omitempty" toml:"areas,omitempty"`
	// SkillAreas assigns skills (by name, or plugin:name) to areas explicitly; it wins over path claims and terms.
	SkillAreas map[string]string `yaml:"skill_areas,omitempty" json:"skill_areas,omitempty" toml:"skill_areas,omitempty"`
}

// ExposureArea is one area of the map: a part of the repository, who owns it, and the words that name it.
type ExposureArea struct {
	// Name is the unique lower-kebab identifier of the area.
	Name string `yaml:"name" json:"name" toml:"name"`
	// Description finishes the sentence "I work on ..." for the area.
	Description string `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	// Owner is free text (a team, people); it is reported and never parsed.
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty" toml:"owner,omitempty"`
	// Paths are the directories the area claims: repository-relative prefixes, "!" excludes, a trailing "*" is a
	// name prefix, and the longest claim on a directory wins.
	Paths []string `yaml:"paths,omitempty" json:"paths,omitempty" toml:"paths,omitempty"`
	// Terms are words that name the area, matched case-insensitively; a term with "_", "/" or "." matches as a
	// substring, any other on word boundaries.
	Terms []string `yaml:"terms,omitempty" json:"terms,omitempty" toml:"terms,omitempty"`
	// Ambiguous are words the area uses in its own sense that other areas use too; a term wins over them.
	Ambiguous []string `yaml:"ambiguous,omitempty" json:"ambiguous,omitempty" toml:"ambiguous,omitempty"`
	// Shared marks repository-wide tooling: its terms classify artifacts but never count as leakage.
	Shared bool `yaml:"shared,omitempty" json:"shared,omitempty" toml:"shared,omitempty"`
	// Declare is the repository path a session of this area declares it works on (context evals).
	Declare string `yaml:"declare,omitempty" json:"declare,omitempty" toml:"declare,omitempty"`
}

// Validate returns the problems of the scalar keys of the table, one message each. The area map is checked by
// internal/areas, which also loads areas_file.
func (c *ExposureConfig) Validate() []string {
	if c == nil {
		return nil
	}
	var problems []string
	if c.Harness != "" && c.Harness != ExposureHarnessClaude {
		problems = append(problems, fmt.Sprintf("exposure.harness %q must be %q", c.Harness, ExposureHarnessClaude))
	}
	switch c.Plugins {
	case "", ExposurePluginsNone, ExposurePluginsEnabled, ExposurePluginsAll:
	default:
		problems = append(problems, fmt.Sprintf("exposure.plugins %q must be %q, %q or %q", c.Plugins, ExposurePluginsNone, ExposurePluginsEnabled, ExposurePluginsAll))
	}
	if c.ContextWindow < 0 {
		problems = append(problems, "exposure.context_window must not be negative")
	}
	if c.StartBudget < 0 {
		problems = append(problems, "exposure.start_budget must not be negative")
	}
	if s := c.CollisionMinScore; s < 0 || s > 1 || math.IsNaN(s) {
		problems = append(problems, fmt.Sprintf("exposure.collision_min_score must be between 0 and 1, got %v", s))
	}
	return problems
}

// EffectiveContextWindow is context_window, or the default when it is unset.
func (c *ExposureConfig) EffectiveContextWindow() int {
	if c == nil || c.ContextWindow <= 0 {
		return ExposureDefaultContextWindow
	}
	return c.ContextWindow
}

// EffectivePlugins is plugins, or "enabled" when it is unset.
func (c *ExposureConfig) EffectivePlugins() string {
	if c == nil || c.Plugins == "" {
		return ExposurePluginsEnabled
	}
	return c.Plugins
}
