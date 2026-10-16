package judge

// ReportSchemaVersion is the "schema_version" of the JSON report.
const ReportSchemaVersion = 2

// The modes of a report.
const (
	ModeJudge    = "judge"
	ModeEstimate = "estimate"
)

// The run decisions.
const (
	DecisionPass       = "pass"
	DecisionFail       = "fail"
	DecisionIncomplete = "incomplete"
)

// The origins of a finding.
const (
	OriginReviewer    = "reviewer"
	OriginAdjudicator = roleAdjudicator
	// OriginCriteria marks a JR-G06 finding raised from a failed criterion in criteria_results.
	OriginCriteria   = "criteria"
	OriginStaticOnly = "static-only"
)

// Adjudication records what the adjudicator did to a finding.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Adjudication struct {
	Decision           string  `json:"decision"`
	ReasonCode         string  `json:"reason_code,omitempty"`
	OriginalSeverity   string  `json:"original_severity,omitempty"`
	OriginalConfidence string  `json:"original_confidence,omitempty"`
	ChainID            *string `json:"chain_id"`
	Note               string  `json:"note,omitempty"`
}

// Verification records how the finding's evidence was checked.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Verification struct {
	QuoteVerified  bool `json:"quote_verified"`
	LocationValid  bool `json:"location_valid"`
	RepairAttempts int  `json:"repair_attempts"`
}

// SuppressionInfo says which suppression silenced a finding.
type SuppressionInfo struct {
	Reason  string `json:"reason"`
	Expires string `json:"expires"`
}

// Finding is a finding of the report.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Finding struct {
	ID                 string         `json:"id"`
	Code               string         `json:"code"`
	Category           Category       `json:"category"`
	Severity           Severity       `json:"severity"`
	Confidence         Confidence     `json:"confidence"`
	Verdict            Verdict        `json:"verdict"`
	Title              string         `json:"title"`
	Location           Location       `json:"location"`
	Quote              string         `json:"quote"`
	Chain              []Step         `json:"chain,omitempty"`
	Why                string         `json:"why,omitempty"`
	BenignReading      string         `json:"benign_reading,omitempty"`
	WhyNotBenign       string         `json:"why_not_benign,omitempty"`
	Convention         *ConventionRef `json:"convention,omitempty"`
	RubricID           *string        `json:"rubric_id,omitempty"`
	StaticRefs         []string       `json:"static_refs,omitempty"`
	AlsoAt             []AlsoAt       `json:"also_at,omitempty"`
	SuggestedFix       *string        `json:"suggested_fix,omitempty"`
	TokenWasteEstimate *int           `json:"token_waste_estimate,omitempty"`
	Origin             string         `json:"origin"`
	// Absence marks a finding about something missing (a heading a rubric requires): it quotes nothing.
	Absence      bool             `json:"absence,omitempty"`
	Adjudication Adjudication     `json:"adjudication"`
	Verification Verification     `json:"verification"`
	Fingerprint  string           `json:"fingerprint"`
	Suppressed   *SuppressionInfo `json:"suppressed,omitempty"`
}

// Dropped is a candidate that did not make it into the report, kept for --explain-drops.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Dropped struct {
	ID         string   `json:"id"`
	Code       string   `json:"code"`
	Category   Category `json:"category"`
	Severity   Severity `json:"severity,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	Location   Location `json:"location"`
	Quote      string   `json:"quote"`
	// RubricID is the criterion of a dropped JR-G06 candidate.
	RubricID string `json:"rubric_id,omitempty"`
	// Stage is verifier, adjudicator, merged, min_confidence or suppressed.
	Stage        string   `json:"stage"`
	ReasonCode   string   `json:"reason_code,omitempty"`
	CounterQuote string   `json:"counter_quote,omitempty"`
	Note         string   `json:"note,omitempty"`
	Problems     []string `json:"problems,omitempty"`
}

// RuleRef is a convention attached to a unit, as the report lists it.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type RuleRef struct {
	ID     string   `json:"id"`
	Paths  []string `json:"paths,omitempty"`
	Why    string   `json:"why"`
	SHA256 string   `json:"sha256"`
}

// Attached lists what was attached to a unit.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Attached struct {
	Rules            []RuleRef `json:"rules,omitempty"`
	InstructionFiles []RuleRef `json:"instruction_files,omitempty"`
}

// Scores are the per-category scores of a unit, null for a category that did not run.
type Scores struct {
	Security   *int `json:"security"`
	Quality    *int `json:"quality"`
	Governance *int `json:"governance"`
}

// CriterionReport is one rubric criterion outcome in the report.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type CriterionReport struct {
	RubricID string `json:"rubric_id"`
	Result   string `json:"result"`
	Note     string `json:"note,omitempty"`
}

// UnitReport is the outcome for one unit.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type UnitReport struct {
	ID            string            `json:"id"`
	Kind          Kind              `json:"kind"`
	Path          string            `json:"path"`
	ContentSHA256 string            `json:"content_sha256,omitempty"`
	Status        string            `json:"status"`
	Reason        string            `json:"reason,omitempty"`
	Complete      bool              `json:"complete"`
	Files         []string          `json:"files,omitempty"`
	Attached      *Attached         `json:"attached,omitempty"`
	Scores        Scores            `json:"scores"`
	Criteria      []CriterionReport `json:"criteria_results,omitempty"`
	Coverage      Coverage          `json:"coverage"`
	Gaps          []string          `json:"gaps,omitempty"`
	Findings      []*Finding        `json:"findings"`
	Suppressed    []*Finding        `json:"suppressed,omitempty"`
	Dropped       []Dropped         `json:"dropped,omitempty"`
	Notes         []string          `json:"notes,omitempty"`
	Calls         int               `json:"calls,omitempty"`
	CostUSD       float64           `json:"cost_usd,omitempty"`

	tally unitTally
}

// unitTally counts what happened to a unit's candidates, for the run summary.
type unitTally struct {
	candidates, verified, repaired int
	dropped                        map[string]int
}

// LoadedSkill is a rubric skill the run applied.
type LoadedSkill struct {
	Name       string   `json:"name"`
	Path       string   `json:"path,omitempty"`
	SHA256     string   `json:"sha256"`
	Tier       string   `json:"tier"`
	Categories []string `json:"categories"`
}

// Provenance is how the run was made.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Provenance struct {
	Temperature     *float64 `json:"temperature"`
	ReasoningEffort *string  `json:"reasoning_effort"`
	SchemaSHA256    string   `json:"schema_sha256"`
	PromptSHA256    string   `json:"prompt_sha256"`
	CacheHits       int      `json:"cache_hits"`
	Calls           int      `json:"calls"`
	InputTokens     int      `json:"input_tokens"`
	OutputTokens    int      `json:"output_tokens"`
	CostUSD         float64  `json:"cost_usd"`
}

// CoverageSummary counts units by how completely they were reviewed.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type CoverageSummary struct {
	UnitsTotal         int `json:"units_total"`
	UnitsFullyReviewed int `json:"units_fully_reviewed"`
	UnitsPartial       int `json:"units_partial"`
	UnitsWithheld      int `json:"units_withheld"`
	UnitsSkipped       int `json:"units_skipped"`
	UnitsErrored       int `json:"units_errored"`
}

// Estimate is the worst case of a run, from the plan.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Estimate struct {
	Units int `json:"units"`
	// Calls is every planned call: the reviewers' and one adjudicator call per unit with a review.
	Calls int `json:"calls"`
	// CallsFit is how many of Calls fit under the caps; the rest would not be made.
	CallsFit            int     `json:"calls_fit"`
	PromptTokens        int     `json:"prompt_tokens"`
	MaxCompletionTokens int     `json:"max_completion_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	ExpectedCostUSD     float64 `json:"expected_cost_usd"`
	CostKnown           bool    `json:"cost_known"`
}

// RunInfo describes the run.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type RunInfo struct {
	PromptVersion   string            `json:"prompt_version"`
	Categories      []Category        `json:"categories"`
	Models          map[string]string `json:"models"`
	EgressHosts     []string          `json:"egress_hosts"`
	FailOn          []string          `json:"fail_on"`
	MinConfidence   Confidence        `json:"min_confidence"`
	LoadedSkills    []LoadedSkill     `json:"loaded_skills,omitempty"`
	PromptOverrides []string          `json:"prompt_overrides,omitempty"`
	Provenance      Provenance        `json:"provenance"`
	Coverage        CoverageSummary   `json:"coverage"`
	Estimate        Estimate          `json:"estimate"`
	Changed         *ChangedInfo      `json:"changed,omitempty"`
	Notices         []string          `json:"notices,omitempty"`
}

// ChangedInfo records a --changed run.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type ChangedInfo struct {
	Base          string `json:"base"`
	Impact        bool   `json:"impact"`
	ChangedUnits  int    `json:"changed_units"`
	ImpactedUnits int    `json:"impacted_units"`
}

// Grounding counts how candidates fared against the verifier and the adjudicator.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Grounding struct {
	Candidates         int `json:"candidates"`
	QuoteVerified      int `json:"quote_verified"`
	DroppedUngrounded  int `json:"dropped_ungrounded"`
	DroppedByAdjud     int `json:"dropped_by_adjudicator"`
	Repaired           int `json:"repaired"`
	BelowMinConfidence int `json:"below_min_confidence"`
	Suppressed         int `json:"suppressed"`
}

// Summary totals the run.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Summary struct {
	Decision   string         `json:"decision"`
	BySeverity map[string]int `json:"by_severity"`
	ByCategory map[string]int `json:"by_category"`
	Grounding  Grounding      `json:"grounding"`
}

// Report is the document `judge --format json` prints (schema/judge-report.schema.json).
//
//nolint:tagliatelle // output keys are snake_case by project convention
type Report struct {
	SchemaVersion int          `json:"schema_version"`
	Mode          string       `json:"mode"`
	Run           RunInfo      `json:"run"`
	Units         []UnitReport `json:"units"`
	Summary       Summary      `json:"summary"`
}
