package evals

import (
	"context"
	"fmt"
)

// ProtocolVersion is the version of the request and response documents exchanged
// with a runner.
const ProtocolVersion = 1

// Arms of a run.
const (
	ArmWith    = "with"
	ArmWithout = "without"
)

// Runner executes the cases of one skill and reports what happened. Implementations
// are pluggable: the claude-plugin-eval adapter, the generic command runner, or a
// fake in tests.
type Runner interface {
	// Name identifies the runner in results.
	Name() string
	// Run executes req.Cases. With req.Ablation set it also runs every case without
	// the skill, so the response carries an Arm "without" result per case.
	Run(ctx context.Context, req *Request) (*Response, error)
}

// Fingerprinter is implemented by runners whose settings change what a run
// produces. The fingerprint joins the result cache key.
type Fingerprinter interface {
	Fingerprint() string
}

// SkillRef identifies the skill under test to a runner.
type SkillRef struct {
	ID string `json:"id"`
	// Dir is the absolute source directory of the skill (SKILL.md and resources;
	// its evals/ directory is not part of the skill a harness loads).
	Dir    string `json:"dir"`
	Digest string `json:"digest"`
	// Description is the skill's description. It is sent for the skills of an
	// activation request, where the description is what the model chooses by.
	Description string `json:"description,omitempty"`
}

// Request is the document a runner receives (on stdin, for the command runner).
type Request struct {
	Version int `json:"version"`
	// Mode is empty (or "cases") for a full case run, "activation" for an
	// activation request and "capabilities" for the handshake probe, which carries
	// no skill and no cases.
	Mode    string   `json:"mode,omitempty"`
	Surface string   `json:"surface,omitempty"`
	Skill   SkillRef `json:"skill"`
	// Skills is the installed set of an activation request: every skill the
	// harness must have available at once, the skill under test among them.
	Skills  []SkillRef `json:"skills,omitempty"`
	Harness string     `json:"harness"`
	Model   string     `json:"model,omitempty"`
	// Runs is how often each prompt of an activation request is repeated, and
	// MaxTurns bounds one run (the decision is in the first turn).
	Runs     int `json:"runs,omitempty"`
	MaxTurns int `json:"max_turns,omitempty"`
	// Ablation asks for an extra run of every case without the skill.
	Ablation bool `json:"ablation"`
	// MaxCostUSD is the budget left for this skill; 0 means no limit.
	MaxCostUSD float64 `json:"max_cost_usd,omitempty"`
	// Cases are self-contained: near misses are expanded and prompt files and
	// fixture sources are inlined.
	Cases []Case `json:"cases"`
}

// Result is what a runner reports for one case in one arm.
type Result struct {
	Case string `json:"case"`
	Arm  string `json:"arm"`
	// Triggered says whether the skill fired. Required for the "with" arm.
	Triggered *bool `json:"triggered,omitempty"`
	// Passed, when set, is the runner's own verdict on the outcome checks
	// (assertions and rubric); ai-rulez then does not grade the case itself,
	// except that a rubric graded by the built-in grader must pass as well.
	Passed *bool `json:"passed,omitempty"`
	// Output is the final answer, graded by the case's assertions when Passed is
	// not set.
	Output string `json:"output,omitempty"`
	// WorkDir is the directory the case ran in, for file assertions.
	WorkDir string `json:"work_dir,omitempty"`
	// RubricScore is the grader's score (0-1) for the case's rubric, and
	// RubricRationale the grader's one-line reason (set by the built-in grader).
	RubricScore     *float64 `json:"rubric_score,omitempty"`
	RubricRationale string   `json:"rubric_rationale,omitempty"`
	// builtinRubric is set by the built-in grader: it owns the rubric verdict, so
	// RubricScore is checked even when the runner gave its own Passed verdict
	// (which then covers only the runner's other checks).
	builtinRubric bool
	InputTokens   int     `json:"input_tokens,omitempty"`
	OutputTokens  int     `json:"output_tokens,omitempty"`
	CostUSD       float64 `json:"cost_usd,omitempty"`
	// Fired, FiredCounts and Runs answer an activation request: how often each skill
	// of the installed set loaded over Runs repetitions of the prompt (a skill that
	// never loaded is absent). Triggered says whether the case's target loaded in
	// most of them and is informational; ai-rulez scores from FiredCounts.
	Fired       []string       `json:"fired,omitempty"`
	FiredCounts map[string]int `json:"fired_counts,omitempty"`
	Runs        int            `json:"runs,omitempty"`
	// ErroredRuns is how many repetitions failed and are not in Runs; ai-rulez
	// treats a prompt whose runs mostly errored as having no result.
	ErroredRuns int `json:"errored_runs,omitempty"`
	// Skipped marks a case the runner cannot run (for example an assertion type it
	// does not support); Reason says why. Skipped cases are not scored.
	Skipped bool   `json:"skipped,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Error marks a run that failed to produce a result. It counts as a failure.
	Error string `json:"error,omitempty"`
}

// Response is the document a runner returns.
type Response struct {
	Version int      `json:"version"`
	Results []Result `json:"results"`
	// CostUSD is the total cost when the runner reports it per response only.
	CostUSD float64 `json:"cost_usd,omitempty"`
	// Capabilities and Surfaces answer the handshake probe (Request.Mode
	// "capabilities"): what the runner can do and, for activation, on which
	// surfaces. A response to any other request leaves them out.
	Capabilities []string `json:"capabilities,omitempty"`
	Surfaces     []string `json:"surfaces,omitempty"`
}

// Validate checks a response against the request it answers.
func (r *Response) Validate(req *Request) error {
	if r.Version != ProtocolVersion {
		return fmt.Errorf("runner answered protocol version %d, want %d", r.Version, ProtocolVersion)
	}
	if err := checkMoney("cost_usd", r.CostUSD); err != nil {
		return err
	}
	known := map[string]bool{}
	for i := range req.Cases {
		known[req.Cases[i].ID] = true
	}
	for i := range r.Results {
		result := &r.Results[i]
		if err := result.validateCommon(i, known); err != nil {
			return err
		}
		if req.Mode == ModeActivation {
			if err := result.validateActivation(i, req); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateCommon checks the fields every result carries, whatever the mode.
func (r *Result) validateCommon(i int, known map[string]bool) error {
	if !known[r.Case] {
		return fmt.Errorf("results[%d]: unknown case %q", i, r.Case)
	}
	if r.Arm != ArmWith && r.Arm != ArmWithout {
		return fmt.Errorf("results[%d]: arm must be %q or %q, got %q", i, ArmWith, ArmWithout, r.Arm)
	}
	if err := checkMoney("cost_usd", r.CostUSD); err != nil {
		return fmt.Errorf("results[%d]: %w", i, err)
	}
	if r.InputTokens < 0 || r.OutputTokens < 0 {
		return fmt.Errorf("results[%d]: token counts must be >= 0", i)
	}
	if r.RubricScore != nil && (*r.RubricScore < 0 || *r.RubricScore > 1) {
		return fmt.Errorf("results[%d]: rubric_score must be between 0 and 1", i)
	}
	return nil
}

// validateActivation checks the activation fields of one result against the
// request: the arm, the run count and that only installed skills fired.
func (r *Result) validateActivation(index int, req *Request) error {
	if r.Arm != ArmWith {
		return fmt.Errorf("results[%d]: an activation response has only the %q arm", index, ArmWith)
	}
	if r.Skipped || r.Error != "" {
		return nil
	}
	if r.Runs < 1 {
		return fmt.Errorf("results[%d]: runs must be >= 1 in an activation response", index)
	}
	if r.ErroredRuns < 0 {
		return fmt.Errorf("results[%d]: errored_runs must be >= 0, got %d", index, r.ErroredRuns)
	}
	installed := map[string]bool{}
	for i := range req.Skills {
		installed[req.Skills[i].ID] = true
	}
	total := 0
	for id, n := range r.FiredCounts {
		if !installed[id] && id != firedNone {
			return fmt.Errorf("results[%d]: fired_counts names %q, which is not in the installed set", index, id)
		}
		if n < 0 || n > r.Runs {
			return fmt.Errorf("results[%d]: fired_counts[%q] is %d, outside 0..runs (%d)", index, id, n, r.Runs)
		}
		total += n
	}
	if total > r.Runs*len(installed) {
		return fmt.Errorf("results[%d]: fired_counts sums to %d over %d runs", index, total, r.Runs)
	}
	return nil
}

// ValidateProbe checks the answer to the handshake probe: the protocol version and
// no results (a runner that answers a probe with results ran something).
func (r *Response) ValidateProbe() error {
	if r.Version != ProtocolVersion {
		return fmt.Errorf("runner answered protocol version %d, want %d", r.Version, ProtocolVersion)
	}
	if len(r.Results) > 0 {
		return fmt.Errorf("runner answered the capabilities probe with %d results; it does not implement the handshake", len(r.Results))
	}
	return nil
}
