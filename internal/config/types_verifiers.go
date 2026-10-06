package config

import "github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"

// Verifier predicate types of the flat form. The command and llm predicates
// exist only in the spec form (`[verifiers.require.command]`, `.llm`), where
// they are gated by `--allow-exec` and `--allow-llm`.
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

// VerifiersSettings is the [verifiers_settings] table: limits and policy of
// `ai-rulez verifiers run`. (The name differs from the [[verifiers]] array
// because TOML cannot use one key as both a table and an array.)
type VerifiersSettings struct {
	// MaxTimeoutS caps the timeout_s of a command predicate (default 300).
	MaxTimeoutS int `yaml:"max_timeout_s,omitempty" json:"max_timeout_s,omitempty" toml:"max_timeout_s,omitempty"`
	// MaxFileBytes bounds how much of a file a content predicate reads
	// (default 5 MiB); a larger file is skipped with a note, never half-read.
	MaxFileBytes int `yaml:"max_file_bytes,omitempty" json:"max_file_bytes,omitempty" toml:"max_file_bytes,omitempty"`
	// RequireExamples reports a spec verifier without self-test examples (AR9H6).
	RequireExamples bool `yaml:"require_examples,omitempty" json:"require_examples,omitempty" toml:"require_examples,omitempty"`
	// WarnDead reports a verifier whose when_changed matches no file (AR9H5)
	// on every run, as --strict-applicability does.
	WarnDead bool `yaml:"warn_dead,omitempty" json:"warn_dead,omitempty" toml:"warn_dead,omitempty"`
	// TrustExecFrom names includes whose verifiers may use the command
	// predicate. The include must also be pinned in ai-rulez.lock.
	TrustExecFrom []string `yaml:"trust_exec_from,omitempty" json:"trust_exec_from,omitempty" toml:"trust_exec_from,omitempty"`
	// CommandEnv lists extra environment variable NAMES passed to command
	// predicates. Everything else is scrubbed; credential-looking names are refused.
	CommandEnv []string `yaml:"command_env,omitempty" json:"command_env,omitempty" toml:"command_env,omitempty"`
}
