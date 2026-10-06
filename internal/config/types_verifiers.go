package config

import "github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"

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
// and run by `ai-rulez verifiers run`. Two forms share the table. The flat form
// sets Type, and the fields that type uses. The spec form leaves Type empty and
// declares what a .ai-rulez/verifiers/*.toml entry does (the enforced rule,
// skill, agent or command, WhenChanged, Require, Fix, Examples); Name is its id.
type VerifierConfig struct {
	// Name identifies the verifier; it is unique and is the merge key of the
	// machine-local overlay.
	Name        string `yaml:"name" json:"name" toml:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty" toml:"description,omitempty"`
	// Type selects the predicate; see VerifierTypes. Empty selects the spec form.
	Type string `yaml:"type,omitempty" json:"type,omitempty" toml:"type,omitempty"`
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

	// Spec form (Type empty): exactly one of Rule, Skill, Agent and Command
	// names the enforced item, as `id` or `domain/id`.
	Rule    string `yaml:"rule,omitempty" json:"rule,omitempty" toml:"rule,omitempty"`
	Skill   string `yaml:"skill,omitempty" json:"skill,omitempty" toml:"skill,omitempty"`
	Agent   string `yaml:"agent,omitempty" json:"agent,omitempty" toml:"agent,omitempty"`
	Command string `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
	// Anchor is a heading of the enforced file; findings point at its line.
	Anchor  string `yaml:"anchor,omitempty" json:"anchor,omitempty" toml:"anchor,omitempty"`
	Message string `yaml:"message,omitempty" json:"message,omitempty" toml:"message,omitempty"`
	Fix     string `yaml:"fix,omitempty" json:"fix,omitempty" toml:"fix,omitempty"`
	// WhenChanged scopes the verifier to changed files matching these globs;
	// Exclude (shared with the flat form) removes matches.
	WhenChanged []string `yaml:"when_changed,omitempty" json:"when_changed,omitempty" toml:"when_changed,omitempty"`
	// Require is the predicate tree: one predicate or an all/any/not combinator.
	Require  *vspec.Require  `yaml:"require,omitempty" json:"require,omitempty" toml:"require,omitempty"`
	Examples []vspec.Example `yaml:"examples,omitempty" json:"examples,omitempty" toml:"examples,omitempty"`
}

// IsSpec reports whether the entry uses the spec form (no Type).
func (v *VerifierConfig) IsSpec() bool { return v.Type == "" }
