package config

// Verifier predicate types. Phase 2 adds a "command" type that runs a hardened
// command; it is deliberately absent so a config that names it is rejected
// until the runner exists.
const (
	VerifierFileExists      = "file_exists"
	VerifierFileAbsent      = "file_absent"
	VerifierGlobCount       = "glob_count"
	VerifierRegex           = "regex"
	VerifierForbid          = "forbid"
	VerifierKeyEquals       = "key_equals"
	VerifierGeneratedInSync = "generated_in_sync"
)

// VerifierTypes lists the accepted predicate types, in documentation order.
var VerifierTypes = []string{
	VerifierFileExists, VerifierFileAbsent, VerifierGlobCount, VerifierRegex,
	VerifierForbid, VerifierKeyEquals, VerifierGeneratedInSync,
}

// VerifierConfig is one deterministic repo check declared as [[verifiers]]
// and run by `ai-rulez verifiers run`. Which fields apply depends on Type.
type VerifierConfig struct {
	// Name identifies the verifier; it is unique and is the merge key of the
	// machine-local overlay.
	Name        string `yaml:"name" json:"name" toml:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	// Type selects the predicate; see VerifierTypes.
	Type string `yaml:"type" json:"type" toml:"type"`
	// Severity is error (default), warning or info. Only errors (and warnings
	// with --strict) fail the run.
	Severity string `yaml:"severity,omitempty" json:"severity,omitempty" toml:"severity,omitempty"`
	// Path is a repo-relative file for file_exists, file_absent and key_equals.
	Path string `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	// Glob selects files for glob_count, regex and forbid; Exclude removes matches.
	Glob    string   `yaml:"glob,omitempty" json:"glob,omitempty" toml:"glob,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
	// Pattern is the RE2 expression for regex and forbid.
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty" toml:"pattern,omitempty"`
	// Min and Max bound the match count of glob_count.
	Min *int `yaml:"min,omitempty" json:"min,omitempty" toml:"min,omitempty"`
	Max *int `yaml:"max,omitempty" json:"max,omitempty" toml:"max,omitempty"`
	// Key is a dotted path into a JSON, YAML or TOML document (key_equals);
	// Equals is the expected scalar, compared as text.
	Key    string  `yaml:"key,omitempty" json:"key,omitempty" toml:"key,omitempty"`
	Equals *string `yaml:"equals,omitempty" json:"equals,omitempty" toml:"equals,omitempty"`
	// Profile is the generate profile generated_in_sync renders ("" selects the default).
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty" toml:"profile,omitempty"`
}
