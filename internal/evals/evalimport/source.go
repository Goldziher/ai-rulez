// Package evalimport turns eval scenarios written for another tool into ai-rulez
// eval case files. It is offline: it reads local files only, never contacts a
// service, runs nothing it reads, and treats every input as untrusted text. A
// format is a Source (Detect, Load, Map); tessl is the one that exists.
package evalimport

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
)

// Limits bounds what the importer reads.
type Limits struct {
	// MaxBytes caps any one input file. Default 2 MiB.
	MaxBytes int64
	// MaxDepth caps the nesting of a JSON document. Default 32.
	MaxDepth int
	// MaxScenarios caps the scenarios of one run. Default 500.
	MaxScenarios int
}

// Default limits.
const (
	DefaultMaxBytes     int64 = 2 << 20
	DefaultMaxDepth           = 32
	DefaultMaxScenarios       = 500
)

func (l Limits) withDefaults() Limits {
	if l.MaxBytes <= 0 {
		l.MaxBytes = DefaultMaxBytes
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = DefaultMaxDepth
	}
	if l.MaxScenarios <= 0 {
		l.MaxScenarios = DefaultMaxScenarios
	}
	return l
}

// Rubric modes of MapOptions.
const (
	// RubricSingle puts the checklist in one free-text rubric, weights as prose.
	RubricSingle = "single"
	// RubricItems maps each criterion to a rubric_items entry with its weight.
	RubricItems = "items"
)

// MapOptions configures Map.
type MapOptions struct {
	// RubricMode is RubricSingle (default) or RubricItems.
	RubricMode string
	// ID, when set, replaces the case id derived from the scenario name (the
	// importer uses it to keep ids unique).
	ID string
	// LiftAssertions converts clearly mechanical criteria into deterministic
	// assertions (the criterion stays in the rubric).
	LiftAssertions bool
}

// Source is one importable format.
type Source interface {
	// Name is the --from value.
	Name() string
	// Detect says whether path looks like input of this format.
	Detect(path string) bool
	// Load reads the scenarios under path (a scenario directory, its criteria
	// file, or a directory of scenario directories).
	Load(path string, limits Limits) ([]Scenario, error)
	// Map converts a scenario into a case and the report of what happened to its fields.
	Map(sc *Scenario, opts MapOptions) (*Mapped, error)
}

// Scenario is a loaded scenario, still in the source's shape.
type Scenario struct {
	// Name is the scenario's own name (or its directory name).
	Name string
	// Dir is where the scenario's files are; fixture sources resolve against it.
	Dir string
	// Task is the prompt text.
	Task string
	// Raw is the decoded criteria document.
	Raw any
	// SHA256 is the digest of the input files (criteria and task), for provenance.
	SHA256 string
	// Origin is the path the scenario was read from, as given.
	Origin string
}

// FixtureFile is a starting file of a scenario.
type FixtureFile struct {
	// Path is where the case creates it (relative, checked).
	Path    string
	Content []byte
}

// Mapped is the result of mapping one scenario.
type Mapped struct {
	// ID is the case id before collision handling.
	ID string
	// Case is the case in its on-disk form.
	Case evals.Case
	// TaskFile is the prompt file written next to the case, and its content.
	TaskFile    string
	TaskContent string
	// Fixtures are the files to copy beside the case.
	Fixtures []FixtureFile
	Report   Report
}

// Report states what happened to the fields of one scenario, so nothing
// disappears silently.
type Report struct {
	Scenario     string `json:"scenario"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"source_sha256"`
	// Mapped, Assumed and Ignored are one line each.
	Mapped  []string `json:"mapped,omitempty"`
	Assumed []string `json:"assumed,omitempty"`
	// Unmapped lists every input field that has no counterpart (AR9A5).
	Unmapped []Unmapped `json:"unmapped,omitempty"`
	Lifted   []Lift     `json:"lifted,omitempty"`
	Warnings []string   `json:"warnings,omitempty"`
	// Written lists the files written (or, in a dry run, that would be).
	Written []string `json:"written,omitempty"`
}

// Unmapped is one input field that could not be mapped.
type Unmapped struct {
	// Path is the JSON path of the field, for example $.baseline.arm.
	Path string `json:"path"`
	// Value is a short rendering of the value (never more than 80 characters).
	Value string `json:"value"`
	// Hint says where the information belongs in ai-rulez, when it has a place.
	Hint string `json:"hint,omitempty"`
}

// Lift is one criterion converted into an assertion.
type Lift struct {
	Criterion string `json:"criterion"`
	Assertion string `json:"assertion"`
}

// FindingCode is the code the importer's report carries for unmapped fields. It is
// informational: validate does not report it.
const FindingCode = "AR9A5"

// summarize renders a value for the report in at most 80 characters.
func summarize(v any) string {
	text := fmt.Sprintf("%v", v)
	switch t := v.(type) {
	case string:
		text = t
	case []any, map[string]any:
		text = fmt.Sprintf("%d item(s)", lenOf(t))
	}
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	runes := []rune(text)
	if len(runes) > 80 {
		text = string(runes[:77]) + "..."
	}
	return text
}

func lenOf(v any) int {
	switch t := v.(type) {
	case []any:
		return len(t)
	case map[string]any:
		return len(t)
	}
	return 0
}
