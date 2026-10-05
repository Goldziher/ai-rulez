// Package evals defines a harness-neutral eval case format for skills, the
// scoring that turns runner results into per-skill numbers, and the pluggable
// runners that produce those results. It never makes a network call itself: a
// runner is either the `claude plugin eval` adapter or a user-supplied command.
package evals

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// CaseSchemaVersion is the version of the eval case file format.
const CaseSchemaVersion = 1

// Assertion types.
const (
	AssertContains    = "contains"
	AssertNotContains = "not_contains"
	AssertRegex       = "regex"
	AssertFileExists  = "file_exists"
	AssertCommandExit = "command_exit"
)

// DefaultRubricMinScore is the rubric score (0-1) a grader must reach.
const DefaultRubricMinScore = 0.7

// NearMissTag marks cases derived from a near_miss prompt.
const NearMissTag = "near-miss"

// CaseFileSuffixes are the file name suffixes that hold eval cases. Every other
// file under an evals directory (fixtures, graders, harness-native cases) is
// ignored by the parser.
var CaseFileSuffixes = []string{".eval.yaml", ".eval.yml", ".eval.json"}

var (
	idPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]*$`)
)

// Fixture is a file placed in the case's working directory before the run.
type Fixture struct {
	// Path is relative to the working directory.
	Path string `yaml:"path" json:"path"`
	// Content is the file's inline content. Exactly one of Content and Source is set
	// (an empty Content with no Source creates an empty file).
	Content string `yaml:"content,omitempty" json:"content,omitempty"`
	// Source is a file relative to the case file to copy.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

// Assertion is one deterministic check on a run.
type Assertion struct {
	Type string `yaml:"type" json:"type"`
	// Value is the text or pattern for contains, not_contains and regex.
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
	// Path names a file in the working directory: the subject of
	// contains/not_contains/regex when set (otherwise the final answer), and the
	// file file_exists looks for.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
	// Exists is for file_exists: false asserts the file is absent. Default true.
	Exists *bool `yaml:"exists,omitempty" json:"exists,omitempty"`
	// Command is for command_exit: run through the shell in the working directory.
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
	// ExitCode is the expected exit status of Command. Default 0.
	ExitCode *int `yaml:"exit_code,omitempty" json:"exit_code,omitempty"`
}

// Case is one eval case for a skill.
type Case struct {
	ID          string `yaml:"id,omitempty" json:"id,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Prompt is the user prompt. PromptFile names a file relative to the case file instead.
	Prompt     string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	PromptFile string `yaml:"prompt_file,omitempty" json:"prompt_file,omitempty"`
	// ExpectTrigger says whether the skill should fire for the prompt. Required.
	ExpectTrigger *bool `yaml:"expect_trigger,omitempty" json:"expect_trigger,omitempty"`
	// NearMiss lists prompts that look like they should trigger the skill but must
	// not. Each becomes a derived negative case.
	NearMiss   []string    `yaml:"near_miss,omitempty" json:"near_miss,omitempty"`
	Files      []Fixture   `yaml:"files,omitempty" json:"files,omitempty"`
	Assertions []Assertion `yaml:"assertions,omitempty" json:"assertions,omitempty"`
	// Rubric is graded by a model grader the runner provides.
	Rubric string `yaml:"rubric,omitempty" json:"rubric,omitempty"`
	// RubricMinScore is the pass mark (0-1) for Rubric. Default 0.7.
	RubricMinScore *float64 `yaml:"rubric_min_score,omitempty" json:"rubric_min_score,omitempty"`
	Model          string   `yaml:"model,omitempty" json:"model,omitempty"`
	Tags           []string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// Set by the loader, never part of a case file.
	File        string `yaml:"-" json:"-"`
	Line        int    `yaml:"-" json:"-"`
	NearMissOf  string `yaml:"-" json:"near_miss_of,omitempty"`
	ResolvedDir string `yaml:"-" json:"-"`
}

// CaseFile is the on-disk document: a list of cases, or a single case written at
// the top level.
type CaseFile struct {
	SchemaVersion int    `yaml:"schema_version,omitempty" json:"schema_version,omitempty"`
	Cases         []Case `yaml:"cases,omitempty" json:"cases,omitempty"`
	Case          `yaml:",inline"`
}

// Problem is one malformed thing in a case file.
type Problem struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	if p.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Message)
	}
	return fmt.Sprintf("%s: %s", p.File, p.Message)
}

// IsCaseFile reports whether a file name holds eval cases.
func IsCaseFile(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range CaseFileSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// ParseFile parses and validates one case file's bytes. name is the file's path
// (used in problems and to default a single case's id). The returned cases are the
// authored ones, before near-miss expansion. Problems do not stop parsing: every
// case that could be read is returned next to them.
func ParseFile(name string, data []byte) ([]Case, []Problem) {
	doc, lines, problems := decodeCaseFile(name, data)
	if len(problems) > 0 {
		return nil, problems
	}
	fail := func(line int, format string, args ...any) {
		problems = append(problems, Problem{File: name, Line: line, Message: fmt.Sprintf(format, args...)})
	}

	cases := doc.Cases
	single := doc.ID != "" || doc.Prompt != "" || doc.PromptFile != "" || doc.ExpectTrigger != nil
	switch {
	case len(cases) > 0 && single:
		fail(1, "a file holds either a top-level case or a cases list, not both")
		return nil, problems
	case len(cases) == 0 && single:
		one := doc.Case
		if one.ID == "" {
			one.ID = defaultCaseID(name)
		}
		cases = []Case{one}
		lines.cases = []int{1}
	case len(cases) == 0:
		fail(1, "file holds no cases (add a cases list or write one case at the top level)")
		return nil, problems
	}

	out, more := validateCases(name, cases, lines)
	return out, append(problems, more...)
}

// validateCases stamps each case with its file and line and reports its problems.
func validateCases(name string, cases []Case, lines caseLines) ([]Case, []Problem) {
	var problems []Problem
	seen := map[string]bool{}
	out := make([]Case, 0, len(cases))
	for i := range cases {
		c := cases[i]
		c.File = name
		c.Line = 1
		if i < len(lines.cases) && lines.cases[i] > 0 {
			c.Line = lines.cases[i]
		}
		for _, message := range c.validate() {
			problems = append(problems, Problem{File: name, Line: c.Line, Message: fmt.Sprintf("case %q: %s", caseLabel(c, i), message)})
		}
		if c.ID != "" {
			if seen[c.ID] {
				problems = append(problems, Problem{File: name, Line: c.Line, Message: fmt.Sprintf("duplicate case id %q in this file", c.ID)})
			}
			seen[c.ID] = true
		}
		out = append(out, c)
	}
	return out, problems
}

// decodeCaseFile strictly decodes a YAML or JSON case file and checks its
// schema_version.
func decodeCaseFile(name string, data []byte) (doc CaseFile, lines caseLines, problems []Problem) {
	fail := func(line int, format string, args ...any) {
		problems = append(problems, Problem{File: name, Line: line, Message: fmt.Sprintf(format, args...)})
	}
	if strings.EqualFold(filepath.Ext(name), ".json") {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&doc); err != nil {
			fail(jsonLine(data, err), "invalid JSON: %v", err)
			return doc, lines, problems
		}
	} else {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				fail(1, "file is empty")
			} else {
				fail(yamlErrLine(err), "invalid YAML: %v", yamlErrMessage(err))
			}
			return doc, lines, problems
		}
		lines = yamlCaseLines(data)
	}
	if doc.SchemaVersion != 0 && doc.SchemaVersion != CaseSchemaVersion {
		fail(1, "unsupported schema_version %d (this ai-rulez reads %d)", doc.SchemaVersion, CaseSchemaVersion)
	}
	return doc, lines, problems
}

func caseLabel(c Case, index int) string {
	if c.ID != "" {
		return c.ID
	}
	return fmt.Sprintf("#%d", index+1)
}

func defaultCaseID(name string) string {
	base := filepath.Base(name)
	for _, suffix := range CaseFileSuffixes {
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			return strings.ToLower(base[:len(base)-len(suffix)])
		}
	}
	return strings.ToLower(base)
}

// validate returns the case's problems, in a stable order.
func (c *Case) validate() []string {
	out := c.validateIdentity()
	out = append(out, c.validateFixtures()...)
	for i := range c.Assertions {
		for _, message := range c.Assertions[i].validate() {
			out = append(out, fmt.Sprintf("assertions[%d]: %s", i, message))
		}
	}
	return append(out, c.validateGrading()...)
}

func (c *Case) validateIdentity() []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	switch {
	case c.ID == "":
		add("id is required")
	case !idPattern.MatchString(c.ID):
		add("id %q must be lowercase letters, digits, '.', '_' or '-'", c.ID)
	}
	out = append(out, c.validatePrompt()...)
	if c.ExpectTrigger == nil {
		add("expect_trigger is required (true or false)")
	}
	for i, prompt := range c.NearMiss {
		if strings.TrimSpace(prompt) == "" {
			add("near_miss[%d] is empty", i)
		}
	}
	if len(c.NearMiss) > 0 && c.ExpectTrigger != nil && !*c.ExpectTrigger {
		add("near_miss only makes sense on a case with expect_trigger: true")
	}
	for i, tag := range c.Tags {
		if !tagPattern.MatchString(tag) {
			add("tags[%d] %q must be lowercase letters, digits, '.', '_', ':' or '-'", i, tag)
		}
	}
	return out
}

func (c *Case) validatePrompt() []string {
	switch {
	case c.Prompt == "" && c.PromptFile == "":
		return []string{"one of prompt and prompt_file is required"}
	case c.Prompt != "" && c.PromptFile != "":
		return []string{"prompt and prompt_file are mutually exclusive"}
	case c.PromptFile != "":
		if err := checkRelPath(c.PromptFile); err != "" {
			return []string{"prompt_file " + err}
		}
	}
	return nil
}

func (c *Case) validateFixtures() []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	for i, f := range c.Files {
		if err := checkRelPath(f.Path); err != "" {
			add("files[%d].path %s", i, err)
		}
		if f.Content != "" && f.Source != "" {
			add("files[%d]: content and source are mutually exclusive", i)
		}
		if f.Source != "" {
			if err := checkRelPath(f.Source); err != "" {
				add("files[%d].source %s", i, err)
			}
		}
	}
	return out
}

func (c *Case) validateGrading() []string {
	var out []string
	if c.RubricMinScore != nil {
		if c.Rubric == "" {
			out = append(out, "rubric_min_score needs a rubric")
		}
		if *c.RubricMinScore < 0 || *c.RubricMinScore > 1 {
			out = append(out, "rubric_min_score must be between 0 and 1")
		}
	}
	return out
}

func (a *Assertion) validate() []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	forbid := func(field string, set bool) {
		if set {
			add("%s is not valid for type %q", field, a.Type)
		}
	}
	switch a.Type {
	case AssertContains, AssertNotContains:
		if a.Value == "" {
			add("value is required for type %q", a.Type)
		}
		forbid("command", a.Command != "")
		forbid("exit_code", a.ExitCode != nil)
		forbid("exists", a.Exists != nil)
	case AssertRegex:
		if a.Value == "" {
			add("value is required for type %q", a.Type)
		} else if _, err := regexp.Compile(a.Value); err != nil {
			add("value is not a valid regular expression: %v", err)
		}
		forbid("command", a.Command != "")
		forbid("exit_code", a.ExitCode != nil)
		forbid("exists", a.Exists != nil)
	case AssertFileExists:
		if a.Path == "" {
			add("path is required for type %q", a.Type)
		}
		forbid("value", a.Value != "")
		forbid("command", a.Command != "")
		forbid("exit_code", a.ExitCode != nil)
	case AssertCommandExit:
		if strings.TrimSpace(a.Command) == "" {
			add("command is required for type %q", a.Type)
		}
		forbid("value", a.Value != "")
		forbid("path", a.Path != "")
		forbid("exists", a.Exists != nil)
	case "":
		add("type is required (%s)", strings.Join(assertionTypes(), ", "))
	default:
		add("unknown type %q (use %s)", a.Type, strings.Join(assertionTypes(), ", "))
	}
	if a.Path != "" {
		if err := checkRelPath(a.Path); err != "" {
			add("path %s", err)
		}
	}
	return out
}

func assertionTypes() []string {
	return []string{AssertContains, AssertNotContains, AssertRegex, AssertFileExists, AssertCommandExit}
}

// checkRelPath returns a problem description for a path that is not a clean
// relative path inside its base, or "".
func checkRelPath(p string) string {
	switch {
	case p == "":
		return "is empty"
	case path.IsAbs(filepath.ToSlash(p)) || filepath.IsAbs(p) || strings.HasPrefix(p, "~"):
		return fmt.Sprintf("%q must be relative", p)
	}
	clean := path.Clean(filepath.ToSlash(p))
	if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return fmt.Sprintf("%q must stay inside its directory", p)
	}
	return ""
}

// Expand returns the cases with every near_miss prompt turned into its own
// negative case, in a stable order (a case, then its near misses).
func Expand(cases []Case) []Case {
	out := make([]Case, 0, len(cases))
	for i := range cases {
		c := cases[i]
		parent := c
		parent.NearMiss = nil
		out = append(out, parent)
		for n, prompt := range c.NearMiss {
			nm := Case{
				ID:            fmt.Sprintf("%s.near-miss-%d", c.ID, n+1),
				Description:   "near miss of " + c.ID,
				Prompt:        prompt,
				ExpectTrigger: boolPtr(false),
				Model:         c.Model,
				Tags:          appendUnique(c.Tags, NearMissTag),
				File:          c.File,
				Line:          c.Line,
				NearMissOf:    c.ID,
				ResolvedDir:   c.ResolvedDir,
			}
			out = append(out, nm)
		}
	}
	return out
}

func boolPtr(b bool) *bool { return &b }

func appendUnique(list []string, extra string) []string {
	out := append([]string(nil), list...)
	for _, item := range out {
		if item == extra {
			return out
		}
	}
	return append(out, extra)
}

// Expects reports whether the case expects the skill to trigger.
func (c *Case) Expects() bool { return c.ExpectTrigger != nil && *c.ExpectTrigger }
