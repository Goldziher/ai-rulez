// Package judge is the engine of `ai-rulez judge`: it reviews agent configuration (skills, agents,
// commands, rules, CLAUDE.md and AGENTS.md files, MCP and settings files) with language models.
//
// A run is a pipeline. Static facts from the lint analyzers lead three category reviewers
// (security, quality, governance), each with its own prompt and output schema. A deterministic
// verifier checks that every finding quotes text that was really sent, with one repair round for the
// findings that fail. An independent adjudicator keeps, downgrades, merges or drops the survivors.
// Scores, statuses and the run decision are computed in code from the findings: a model never emits
// a score or a verdict.
//
// The package does no I/O of its own: file contents, repository listings, static facts and the model
// client come from the caller, so the whole pipeline is testable offline.
package judge

import (
	"strings"
)

// Names shared across the package: the rubric failure code, the "nothing here" text of a prompt
// block, the reason of a decision that keeps a finding, the role of the adjudicator and the JSON key
// of a finding's category.
const (
	codeRubricFailure = "JR-G06"
	noText            = "(none)"
	reasonGrounded    = "grounded"
	roleAdjudicator   = "adjudicator"
	keyCategory       = "category"
)

// PromptVersion is part of every request, so a changed prompt or schema never reuses a cached answer.
// Bump it whenever a file under prompts/ or schemas/ changes.
const PromptVersion = "judge/v3"

// Category is one of the three review concerns.
type Category string

// The categories, in the priority order a capped run spends its budget in.
const (
	CategorySecurity   Category = "security"
	CategoryGovernance Category = "governance"
	CategoryQuality    Category = "quality"
)

// AllCategories lists the categories in priority order.
func AllCategories() []Category {
	return []Category{CategorySecurity, CategoryGovernance, CategoryQuality}
}

// ParseCategory parses a category name.
func ParseCategory(s string) (Category, bool) {
	switch c := Category(strings.ToLower(strings.TrimSpace(s))); c {
	case CategorySecurity, CategoryGovernance, CategoryQuality:
		return c, true
	}
	return "", false
}

// codePrefix is the letter every code of the category carries (JR-S01, JR-Q01, JR-G01).
func (c Category) codePrefix() string {
	switch c {
	case CategorySecurity:
		return "S"
	case CategoryQuality:
		return "Q"
	default:
		return "G"
	}
}

// Severity is the impact of a finding.
type Severity string

// The severities, most serious first.
const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
	// SeverityNone is only a fail-on threshold: the category never fails a unit.
	SeverityNone Severity = "none"
)

// Rank orders severities: critical 4 ... info 0. Unknown values rank -1.
func (s Severity) Rank() int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	case SeverityInfo:
		return 0
	}
	return -1
}

// Valid reports whether s is a finding severity.
func (s Severity) Valid() bool { return s.Rank() >= 0 }

// Confidence is how firmly the quote alone establishes a finding.
type Confidence string

// The confidence levels.
const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Rank orders confidences: high 2, medium 1, low 0. Unknown values rank -1.
func (c Confidence) Rank() int {
	switch c {
	case ConfidenceHigh:
		return 2
	case ConfidenceMedium:
		return 1
	case ConfidenceLow:
		return 0
	}
	return -1
}

// Verdict is whether the quoted text establishes the behavior or only a risky capability.
type Verdict string

// The verdicts.
const (
	VerdictConfirmed      Verdict = "confirmed"
	VerdictContextualRisk Verdict = "contextual_risk"
)

// The limits of the evidence contract, enforced by the verifier.
const (
	maxLocationLines           = 12
	maxQuoteChars              = 240
	maxChainSteps              = 6
	minChainSteps              = 2
	maxStaticFactsPerUnit      = 64
	defaultMaxFindingsPerStage = 12
)

// Location is a line range of a file in the numbering the reviewer saw.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Location struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Step is one hop of a chain: a source, a transform, a trigger, a sink or a scope, with its own quote.
type Step struct {
	Role     string   `json:"role"`
	Location Location `json:"location"`
	Quote    string   `json:"quote"`
}

// AlsoAt is another place of the same root cause.
type AlsoAt struct {
	Location Location `json:"location"`
	Quote    string   `json:"quote"`
}

// ConventionRef points at the line of an attached convention a governance finding rests on.
type ConventionRef struct {
	ID    string `json:"id"`
	Line  int    `json:"line"`
	Quote string `json:"quote"`
}

// Candidate is a finding as a reviewer (or the adjudicator, as an escalation) proposes it.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Candidate struct {
	ID                 string         `json:"id"`
	Code               string         `json:"code"`
	Verdict            Verdict        `json:"verdict"`
	Severity           Severity       `json:"severity"`
	Confidence         Confidence     `json:"confidence"`
	Title              string         `json:"title"`
	Location           Location       `json:"location"`
	Quote              string         `json:"quote"`
	Chain              []Step         `json:"chain"`
	Why                string         `json:"why"`
	BenignReading      string         `json:"benign_reading"`
	WhyNotBenign       string         `json:"why_not_benign"`
	StaticRefs         []string       `json:"static_refs"`
	AlsoAt             []AlsoAt       `json:"also_at"`
	Convention         *ConventionRef `json:"convention"`
	RubricID           *string        `json:"rubric_id"`
	TokenWasteEstimate *int           `json:"token_waste_estimate"`
	SuggestedFix       *string        `json:"suggested_fix"`
}

// CriterionResult is the outcome of one rubric criterion.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type CriterionResult struct {
	RubricID string `json:"rubric_id"`
	Result   string `json:"result"`
	Note     string `json:"note"`
}

// The results of a rubric criterion.
const (
	CriterionPass          = "pass"
	CriterionFail          = "fail"
	CriterionNotApplicable = "not_applicable"
)

// NotReviewed is a span the reviewer could not see.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type NotReviewed struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Reason    string `json:"reason"`
}

// Coverage is what a unit's review did not see.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Coverage struct {
	NotReviewed []NotReviewed `json:"not_reviewed"`
}

// ReviewerOutput is the reply of a category reviewer.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type ReviewerOutput struct {
	Findings        []Candidate       `json:"findings"`
	CriteriaResults []CriterionResult `json:"criteria_results"`
	Coverage        Coverage          `json:"coverage"`
}

// The adjudicator's decisions and reason codes.
const (
	DecisionKeep      = "keep"
	DecisionDowngrade = "downgrade"
	DecisionDrop      = "drop"
	DecisionMerge     = "merge"
	// DecisionUnreviewed marks a finding no adjudicator saw (static-only, or the adjudicator did not answer).
	DecisionUnreviewed = "unreviewed"
)

// Decision is the adjudicator's verdict on one candidate.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Decision struct {
	CandidateID     string  `json:"candidate_id"`
	Decision        string  `json:"decision"`
	ReasonCode      string  `json:"reason_code"`
	FinalSeverity   string  `json:"final_severity"`
	FinalConfidence string  `json:"final_confidence"`
	CounterQuote    *string `json:"counter_quote"`
	MergeInto       *string `json:"merge_into"`
	Note            string  `json:"note"`
}

// ChainDecision is an attack the adjudicator assembled from kept candidates.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type ChainDecision struct {
	Members          []string `json:"members"`
	CombinedSeverity string   `json:"combined_severity"`
	Story            string   `json:"story"`
}

// AdjudicatorOutput is the reply of the adjudicator.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type AdjudicatorOutput struct {
	Decisions     []Decision      `json:"decisions"`
	Chains        []ChainDecision `json:"chains"`
	Escalations   []Candidate     `json:"escalations"`
	CoverageNotes []string        `json:"coverage_notes"`
}

// Fact is a deterministic lead from the static analyzers (validate and scan). It never makes a
// finding on its own: reviewers verify it against the content.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Fact struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Quote       string `json:"quote,omitempty"`
	Message     string `json:"message,omitempty"`
	ContextKind string `json:"context_kind,omitempty"`
}
