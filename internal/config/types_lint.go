package config

// LintConfig configures `ai-rulez validate --strict`. Every field is optional;
// the lint package applies defaults for anything left unset.
type LintConfig struct {
	// Profile selects a preset of severities and the failure threshold:
	// "default", "strict" or "permissive". Explicit severity, fail_on and
	// budget settings win over the preset.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty" toml:"profile,omitempty"`
	// FailOn is the lowest severity that makes the command exit non-zero:
	// "error" (default), "warning", or "none".
	FailOn string `yaml:"fail_on,omitempty" json:"fail_on,omitempty" toml:"fail_on,omitempty"` //nolint:tagliatelle
	// Analyzers is an allow-list of lint analyzers (security, references,
	// hooks, ...): only these run. `validate --analyzer` replaces it for one
	// invocation. Empty runs every analyzer.
	Analyzers []string `yaml:"analyzers,omitempty" json:"analyzers,omitempty" toml:"analyzers,omitempty"`
	// Ignore lists finding codes (AR401) or names (path-missing) to drop.
	Ignore []string `yaml:"ignore,omitempty" json:"ignore,omitempty" toml:"ignore,omitempty"`
	// IgnorePaths lists globs, relative to the config directory, of source
	// files whose findings are dropped.
	IgnorePaths []string `yaml:"ignore_paths,omitempty" json:"ignore_paths,omitempty" toml:"ignore_paths,omitempty"` //nolint:tagliatelle
	// Severity overrides the severity of a code or name: error, warning, info, off.
	Severity map[string]string `yaml:"severity,omitempty" json:"severity,omitempty" toml:"severity,omitempty"`
	// ExamplePaths lists globs, relative to the config directory or the repo
	// root, of files that document risky commands as examples: findings of the
	// command-shaped rules (AR005, AR006, AR008) in them are dropped.
	ExamplePaths []string `yaml:"example_paths,omitempty" json:"example_paths,omitempty" toml:"example_paths,omitempty"` //nolint:tagliatelle
	// AllowPaths lists globs of repo paths that may be referenced without
	// existing in the tracked tree (generated or machine-local outputs).
	AllowPaths []string `yaml:"allow_paths,omitempty" json:"allow_paths,omitempty" toml:"allow_paths,omitempty"` //nolint:tagliatelle
	// KnownNames lists skill, agent, rule, or command names provided outside
	// this tree (for example by an installed plugin).
	KnownNames []string `yaml:"known_names,omitempty" json:"known_names,omitempty" toml:"known_names,omitempty"` //nolint:tagliatelle
	// Description tunes the frontmatter description checks.
	Description *LintDescription `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	// Tolerate caps how many findings of a rule are tolerated, per rule code or
	// name: a rule whose unaccepted findings number at most N does not count
	// toward the exit code. Lower N over time to ratchet a rule down. Not to be
	// confused with Budgets, which caps the size of a content item.
	Tolerate map[string]int `yaml:"tolerate,omitempty" json:"tolerate,omitempty" toml:"tolerate,omitempty"`
	// Budget is the deprecated spelling of Tolerate; it still works and warns.
	Budget map[string]int `yaml:"budget,omitempty" json:"budget,omitempty" toml:"budget,omitempty"`
	// Risk sets the weights of the advisory risk score.
	Risk *LintRisk `yaml:"risk,omitempty" json:"risk,omitempty" toml:"risk,omitempty"`
	// Budgets maps a content kind (rule, context, skill, agent, command) to its size
	// limits (max_lines, max_tokens). Not to be confused with Tolerate.
	Budgets map[string]LintBudget `yaml:"budgets,omitempty" json:"budgets,omitempty" toml:"budgets,omitempty"`
	// RequireMetadata maps a content kind to frontmatter keys every item of that kind must set.
	RequireMetadata map[string][]string `yaml:"require_metadata,omitempty" json:"require_metadata,omitempty" toml:"require_metadata,omitempty"` //nolint:tagliatelle
	// AllowOverrides lists content names ("name") or domain-qualified names
	// ("domain/name") whose shadowing of another copy is intentional, so they
	// are not reported as collapsed duplicates.
	AllowOverrides []string `yaml:"allow_overrides,omitempty" json:"allow_overrides,omitempty" toml:"allow_overrides,omitempty"` //nolint:tagliatelle
	// AllowedKeys lists frontmatter keys accepted in addition to the built-in
	// Agent Skills, Claude Code and ai-rulez keys.
	AllowedKeys []string `yaml:"allowed_keys,omitempty" json:"allowed_keys,omitempty" toml:"allowed_keys,omitempty"` //nolint:tagliatelle
	// Metadata types and bounds the values of frontmatter keys (top-level, or
	// inside the Agent Skills `metadata` map), keyed by the frontmatter key.
	Metadata map[string]LintMetadataRule `yaml:"metadata,omitempty" json:"metadata,omitempty" toml:"metadata,omitempty"`
	// Security configures the security rule family (AR001...).
	Security *LintSecurity `yaml:"security,omitempty" json:"security,omitempty" toml:"security,omitempty"`
	// External lists third-party scanners whose findings are merged into the
	// report when `validate --strict --external` (or `scan --external`) runs.
	External []LintExternal `yaml:"external,omitempty" json:"external,omitempty" toml:"external,omitempty"`
	// Evals configures the evals-missing check (AR962).
	Evals *LintEvals `yaml:"evals,omitempty" json:"evals,omitempty" toml:"evals,omitempty"`
	// Traps configures the harness trap checks (AR9C1...).
	Traps *LintTraps `yaml:"traps,omitempty" json:"traps,omitempty" toml:"traps,omitempty"`
}

// LintTraps configures the checks for files a harness silently ignores.
type LintTraps struct {
	// ExtraHarnesses names harnesses whose traps run although no preset for them
	// is configured, for hand-written files such as .cursor/rules.
	ExtraHarnesses []string `yaml:"extra_harnesses,omitempty" json:"extra_harnesses,omitempty" toml:"extra_harnesses,omitempty"` //nolint:tagliatelle
	// MaxTableAgeDays reports (AR9C0) a trap or limits row whose verified_on is
	// older than this many days. 0, the default, turns the check off.
	MaxTableAgeDays int `yaml:"max_table_age_days,omitempty" json:"max_table_age_days,omitempty" toml:"max_table_age_days,omitempty"` //nolint:tagliatelle
}

// LintMetadataRule types one frontmatter key.
type LintMetadataRule struct {
	// Type is "string" (default), "date" or "enum".
	Type string `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"`
	// Values lists the accepted values of an enum.
	Values []string `yaml:"values,omitempty" json:"values,omitempty" toml:"values,omitempty"`
	// MaxAgeDays reports a date older than this many days as stale (date only).
	MaxAgeDays int `yaml:"max_age_days,omitempty" json:"max_age_days,omitempty" toml:"max_age_days,omitempty"` //nolint:tagliatelle
	// Kinds limits the rule to content kinds (rule, context, skill, agent, command); empty means all.
	Kinds []string `yaml:"kinds,omitempty" json:"kinds,omitempty" toml:"kinds,omitempty"`
	// Required reports an item of the configured kinds that lacks the key.
	Required bool `yaml:"required,omitempty" json:"required,omitempty" toml:"required,omitempty"`
}

// LintSecurity configures the security checks. Everything is optional.
type LintSecurity struct {
	// ScanImports also scans content imported through includes and installed
	// skills, and makes `generate` refuse to write it when it has an
	// error-level finding. Unset (the default) scans and keeps each finding's own
	// severity; "error" or "warn" replaces the severity of every finding in
	// imported content; "off" opts out.
	ScanImports string `yaml:"scan_imports,omitempty" json:"scan_imports,omitempty" toml:"scan_imports,omitempty"` //nolint:tagliatelle
	// AllowedHosts restricts the hosts URLs may point to ("example.com", "*.example.com").
	// Empty disables the outbound host check.
	AllowedHosts []string `yaml:"allowed_hosts,omitempty" json:"allowed_hosts,omitempty" toml:"allowed_hosts,omitempty"` //nolint:tagliatelle
	// AllowedTools lists allowed-tools entries that may be unrestricted (for example "Bash").
	AllowedTools []string `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty" toml:"allowed_tools,omitempty"` //nolint:tagliatelle
	// SecretPatterns adds regular expressions to the built-in secret detectors.
	SecretPatterns []LintSecretPattern `yaml:"secret_patterns,omitempty" json:"secret_patterns,omitempty" toml:"secret_patterns,omitempty"` //nolint:tagliatelle
	// InjectionPhrases adds case-insensitive phrases to the prompt-injection detector.
	InjectionPhrases []string `yaml:"injection_phrases,omitempty" json:"injection_phrases,omitempty" toml:"injection_phrases,omitempty"` //nolint:tagliatelle
}

// LintSecretPattern is a named secret detector.
type LintSecretPattern struct {
	Name  string `yaml:"name" json:"name" toml:"name"`
	Regex string `yaml:"regex" json:"regex" toml:"regex"`
}

// LintExternal is an external scanner. Command is an argv, run from the
// project root with the paths of the scanned files appended; it must print
// SARIF 2.1.0 or the ai-rulez JSON list to stdout. It never runs through a
// shell, and every run is bounded by a timeout, an output cap and a process
// group kill.
type LintExternal struct {
	Name    string   `yaml:"name" json:"name" toml:"name"`
	Command []string `yaml:"command" json:"command" toml:"command"`
	// Format is "sarif" (default) or "json".
	Format string `yaml:"format,omitempty" json:"format,omitempty" toml:"format,omitempty"`
	// Egress declares whether content derived from the scanned files can leave
	// the machine (or a credential is used to call a network service). Unset keeps
	// the legacy behaviour (full inherited environment) and is reported as AR9E1.
	// false scrubs the environment and rejects known egress flags; true runs the
	// scanner only when its name is passed to --allow-egress.
	Egress *bool `yaml:"egress,omitempty" json:"egress,omitempty" toml:"egress,omitempty"`
	// Timeout is a Go duration such as "90s"; default 2m, at most 15m.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty" toml:"timeout,omitempty"`
	// EnvPass lists extra environment variable names passed through the scrubbed
	// environment. Proxy and credential-like names are rejected when egress is false.
	EnvPass []string `yaml:"env_pass,omitempty" json:"env_pass,omitempty" toml:"env_pass,omitempty"` //nolint:tagliatelle
	// Inputs turns staging on: the scanner runs in a scratch directory that holds
	// a read-only copy of exactly these kinds of content (rules, context, skills,
	// agents, commands, checks, hooks, mcp, imports) and nothing else. The
	// placeholders {stage}, {root}, {files}, {skill_dirs}, {out} and {tmp} in
	// command are expanded as argv elements; no file path is appended.
	Inputs []string `yaml:"inputs,omitempty" json:"inputs,omitempty" toml:"inputs,omitempty"`
	// SeverityMap maps a scanner rule id (glob) to a severity: critical, high,
	// medium, low, info (or error, warning). It is applied before any other mapping.
	SeverityMap map[string]string `yaml:"severity_map,omitempty" json:"severity_map,omitempty" toml:"severity_map,omitempty"` //nolint:tagliatelle
	// MaxSeverity caps the severity of this scanner's findings (same values).
	MaxSeverity string `yaml:"max_severity,omitempty" json:"max_severity,omitempty" toml:"max_severity,omitempty"` //nolint:tagliatelle
}

// LintEvals configures the check for skills that have no eval cases. A skill
// has cases when its evals/ directory, or .ai-rulez/evals/<skill-name>/, holds
// at least one file.
type LintEvals struct {
	// Require turns the evals-missing check on at warning severity. An explicit
	// [lint.severity] entry for evals-missing wins.
	Require bool `yaml:"require,omitempty" json:"require,omitempty" toml:"require,omitempty"`
	// Allow lists skill names or globs exempt from the check.
	Allow []string `yaml:"allow,omitempty" json:"allow,omitempty" toml:"allow,omitempty"`
	// MinPassRate turns the eval-score-low check (AR998) on: a skill whose recorded
	// pass rate (.ai-rulez/eval-results.json) is below it is reported. Range 0-1;
	// also the pass mark of "ai-rulez eval run" when --threshold is not given.
	MinPassRate float64 `yaml:"min_pass_rate,omitempty" json:"min_pass_rate,omitempty" toml:"min_pass_rate,omitempty"` //nolint:tagliatelle
	// RequireFresh turns the eval-stale check (AR997) on: "warn" or "error" report a
	// skill edited after its last recorded passing eval; "off" or empty disables it.
	RequireFresh string `yaml:"require_fresh,omitempty" json:"require_fresh,omitempty" toml:"require_fresh,omitempty"` //nolint:tagliatelle
	// MinActivationRecall and MinActivationPrecision turn the activation-low check
	// (AR9A1) on: a skill whose recorded activation recall or precision
	// ("ai-rulez eval run --mode activation") is below it is reported. Range 0-1.
	MinActivationRecall    float64 `yaml:"min_activation_recall,omitempty" json:"min_activation_recall,omitempty" toml:"min_activation_recall,omitempty"`          //nolint:tagliatelle
	MinActivationPrecision float64 `yaml:"min_activation_precision,omitempty" json:"min_activation_precision,omitempty" toml:"min_activation_precision,omitempty"` //nolint:tagliatelle
	// ConfusionThreshold turns the skill-confusable check (AR9A2) on: a sibling that
	// won at least this share (0-1) of a skill's positive activation prompts is reported.
	ConfusionThreshold float64 `yaml:"confusion_threshold,omitempty" json:"confusion_threshold,omitempty" toml:"confusion_threshold,omitempty"` //nolint:tagliatelle
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

// LintRisk weights the advisory risk score: the points one finding of each
// severity adds (capped at 100 per item or bundle). Unset keeps the default
// (error 25, warning 8, info 1); 0 is a valid weight.
type LintRisk struct {
	Error   *int `yaml:"error,omitempty" json:"error,omitempty" toml:"error,omitempty"`
	Warning *int `yaml:"warning,omitempty" json:"warning,omitempty" toml:"warning,omitempty"`
	Info    *int `yaml:"info,omitempty" json:"info,omitempty" toml:"info,omitempty"`
}

// Tolerated returns the per-rule tolerated finding counts: [lint.tolerate]
// together with the deprecated [lint.budget], with [lint.tolerate] winning when a
// rule is in both.
func (l *LintConfig) Tolerated() map[string]int {
	if l == nil || len(l.Tolerate)+len(l.Budget) == 0 {
		return nil
	}
	out := make(map[string]int, len(l.Tolerate)+len(l.Budget))
	for k, v := range l.Budget {
		out[k] = v
	}
	for k, v := range l.Tolerate {
		out[k] = v
	}
	return out
}
