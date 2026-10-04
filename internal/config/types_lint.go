package config

// LintConfig configures `ai-rulez validate --strict`. Every field is optional;
// the lint package applies defaults for anything left unset.
type LintConfig struct {
	// FailOn is the lowest severity that makes the command exit non-zero:
	// "error" (default), "warning", or "none".
	FailOn string `yaml:"fail_on,omitempty" json:"fail_on,omitempty" toml:"fail_on,omitempty"` //nolint:tagliatelle
	// Ignore lists finding codes (AR401) or names (path-missing) to drop.
	Ignore []string `yaml:"ignore,omitempty" json:"ignore,omitempty" toml:"ignore,omitempty"`
	// IgnorePaths lists globs, relative to the config directory, of source
	// files whose findings are dropped.
	IgnorePaths []string `yaml:"ignore_paths,omitempty" json:"ignore_paths,omitempty" toml:"ignore_paths,omitempty"` //nolint:tagliatelle
	// Severity overrides the severity of a code or name: error, warning, info, off.
	Severity map[string]string `yaml:"severity,omitempty" json:"severity,omitempty" toml:"severity,omitempty"`
	// AllowPaths lists globs of repo paths that may be referenced without
	// existing in the tracked tree (generated or machine-local outputs).
	AllowPaths []string `yaml:"allow_paths,omitempty" json:"allow_paths,omitempty" toml:"allow_paths,omitempty"` //nolint:tagliatelle
	// KnownNames lists skill, agent, rule, or command names provided outside
	// this tree (for example by an installed plugin).
	KnownNames []string `yaml:"known_names,omitempty" json:"known_names,omitempty" toml:"known_names,omitempty"` //nolint:tagliatelle
	// Description tunes the frontmatter description checks.
	Description *LintDescription `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	// Budgets maps a content kind (rule, context, skill, agent, command) to its size limits.
	Budgets map[string]LintBudget `yaml:"budgets,omitempty" json:"budgets,omitempty" toml:"budgets,omitempty"`
	// RequireMetadata maps a content kind to frontmatter keys every item of that kind must set.
	RequireMetadata map[string][]string `yaml:"require_metadata,omitempty" json:"require_metadata,omitempty" toml:"require_metadata,omitempty"` //nolint:tagliatelle
}

// LintDescription tunes description quality checks.
type LintDescription struct {
	// MinLength and MaxLength bound the description in characters. Zero keeps the default.
	MinLength int `yaml:"min_length,omitempty" json:"min_length,omitempty" toml:"min_length,omitempty"` //nolint:tagliatelle
	MaxLength int `yaml:"max_length,omitempty" json:"max_length,omitempty" toml:"max_length,omitempty"` //nolint:tagliatelle
	// RequireUseWhen asks that a description state when to use the item ("Use when ...").
	RequireUseWhen bool `yaml:"require_use_when,omitempty" json:"require_use_when,omitempty" toml:"require_use_when,omitempty"` //nolint:tagliatelle
	// NearDuplicateThreshold is the word-set similarity (0-1) at which two descriptions are reported.
	NearDuplicateThreshold float64 `yaml:"near_duplicate_threshold,omitempty" json:"near_duplicate_threshold,omitempty" toml:"near_duplicate_threshold,omitempty"` //nolint:tagliatelle
}

// LintBudget caps the size of one content item. Zero keeps the default for the
// kind; a negative value removes the limit.
type LintBudget struct {
	MaxLines  int `yaml:"max_lines,omitempty" json:"max_lines,omitempty" toml:"max_lines,omitempty"`    //nolint:tagliatelle
	MaxTokens int `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty" toml:"max_tokens,omitempty"` //nolint:tagliatelle
}
