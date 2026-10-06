package vspec

// Require is one predicate or combinator; exactly one field is set.
type Require struct {
	Regex      *RegexPred      `toml:"regex,omitempty" json:"regex,omitempty"`
	Forbid     *RegexPred      `toml:"forbid,omitempty" json:"forbid,omitempty"`
	FileExists *FileExistsPred `toml:"file_exists,omitempty" json:"file_exists,omitempty"`
	Paired     *PairedPred     `toml:"paired,omitempty" json:"paired,omitempty"`
	GlobCount  *GlobCountPred  `toml:"glob_count,omitempty" json:"glob_count,omitempty"`
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

// Example is one offline self-test of a verifier (`verifiers test`): a
// synthetic file set, the files that count as changed (and entirely added),
// and the expected outcome (pass, fail or not_applicable).
type Example struct {
	Name    string            `toml:"name" json:"name"`
	Files   map[string]string `toml:"files,omitempty" json:"files,omitempty"`
	Changed []string          `toml:"changed,omitempty" json:"changed,omitempty"`
	Expect  string            `toml:"expect" json:"expect"`
}
