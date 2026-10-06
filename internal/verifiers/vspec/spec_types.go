package vspec

// Require is one predicate or combinator; exactly one field is set.
type Require struct {
	Regex      *RegexPred      `toml:"regex,omitempty" json:"regex,omitempty"`
	Forbid     *RegexPred      `toml:"forbid,omitempty" json:"forbid,omitempty"`
	FileExists *FileExistsPred `toml:"file_exists,omitempty" json:"file_exists,omitempty"`
	Paired     *PairedPred     `toml:"paired,omitempty" json:"paired,omitempty"`
	GlobCount  *GlobCountPred  `toml:"glob_count,omitempty" json:"glob_count,omitempty"`
	Command    *CommandPred    `toml:"command,omitempty" json:"command,omitempty"`
	LLM        *LLMPred        `toml:"llm,omitempty" json:"llm,omitempty"`
	All        []Require       `toml:"all,omitempty" json:"all,omitempty"`
	Any        []Require       `toml:"any,omitempty" json:"any,omitempty"`
	Not        *Require        `toml:"not,omitempty" json:"not,omitempty"`
}

// RegexPred is the body of `regex` (must match) and `forbid` (must not match).
type RegexPred struct {
	// Regex is an RE2 expression.
	Regex string `toml:"regex" json:"regex"`
	// In is same-file (default), diff-added or any-file.
	In string `toml:"in,omitempty" json:"in,omitempty"`
	// Files selects the files of an any-file predicate; it defaults to the scope.
	Files string `toml:"files,omitempty" json:"files,omitempty"`
}

// FileExistsPred holds when Path exists (or, with Exists = false, does not).
type FileExistsPred struct {
	Path   string `toml:"path" json:"path"`
	Exists *bool  `toml:"exists,omitempty" json:"exists,omitempty"`
}

// PairedPred requires, for every scoped file matching ForEach, that the path
// derived by the template was changed (RequiresChanged) or exists (RequiresExists).
type PairedPred struct {
	ForEach         string `toml:"for_each" json:"for_each"`
	RequiresChanged string `toml:"requires_changed,omitempty" json:"requires_changed,omitempty"`
	RequiresExists  string `toml:"requires_exists,omitempty" json:"requires_exists,omitempty"`
}

// GlobCountPred bounds how many files in the repository match Files.
type GlobCountPred struct {
	Files   string   `toml:"files" json:"files"`
	Exclude []string `toml:"exclude,omitempty" json:"exclude,omitempty"`
	Min     *int     `toml:"min,omitempty" json:"min,omitempty"`
	Max     *int     `toml:"max,omitempty" json:"max,omitempty"`
}

// CommandPred holds when a program exits with the expected status. It runs
// only with --allow-exec, without a shell, in the project root, with a scrubbed
// environment (see docs/verifiers.md).
type CommandPred struct {
	// Argv is the program and its arguments; there is no shell.
	Argv []string `toml:"argv" json:"argv"`
	// PassFiles hands the scoped files to the program: "args" appends them to
	// argv, "stdin0" writes them NUL-separated to its stdin.
	PassFiles string `toml:"pass_files,omitempty" json:"pass_files,omitempty"`
	// TimeoutS bounds the run; it is capped by [verifiers_settings] max_timeout_s.
	TimeoutS int `toml:"timeout_s,omitempty" json:"timeout_s,omitempty"`
	// ExpectExit is the exit status that means success (default 0).
	ExpectExit *int `toml:"expect_exit,omitempty" json:"expect_exit,omitempty"`
}

// LLMPred asks a model whether the changed lines satisfy a checklist. It is
// advisory (never above warning), needs `--allow-llm`, and sits only at the root
// of a require tree or directly under `all`, after the deterministic predicates.
type LLMPred struct {
	// Checklist is the list of statements the changes should satisfy.
	Checklist []string `toml:"checklist" json:"checklist"`
	// Model overrides the configured [llm] model for this verifier.
	Model string `toml:"model,omitempty" json:"model,omitempty"`
	// MaxDiffBytes bounds the changed text sent per call (default 24000);
	// larger changes are split into several calls.
	MaxDiffBytes int `toml:"max_diff_bytes,omitempty" json:"max_diff_bytes,omitempty"`
}

// Example is one offline self-test of a verifier (`verifiers test`): a
// synthetic file set, the files that count as changed (and entirely added),
// and the expected outcome (pass, fail or not_applicable).
type Example struct {
	Name    string            `toml:"name" json:"name"`
	Files   map[string]string `toml:"files,omitempty" json:"files,omitempty"`
	Changed []string          `toml:"changed,omitempty" json:"changed,omitempty"`
	Expect  string            `toml:"expect" json:"expect"`
}
