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
}

// Request is the document a runner receives (on stdin, for the command runner).
type Request struct {
	Version int      `json:"version"`
	Skill   SkillRef `json:"skill"`
	Harness string   `json:"harness"`
	Model   string   `json:"model,omitempty"`
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
	// (assertions and rubric); ai-rulez then does not grade the case itself.
	Passed *bool `json:"passed,omitempty"`
	// Output is the final answer, graded by the case's assertions when Passed is
	// not set.
	Output string `json:"output,omitempty"`
	// WorkDir is the directory the case ran in, for file assertions.
	WorkDir string `json:"work_dir,omitempty"`
	// RubricScore is the grader's score (0-1) for the case's rubric.
	RubricScore  *float64 `json:"rubric_score,omitempty"`
	InputTokens  int      `json:"input_tokens,omitempty"`
	OutputTokens int      `json:"output_tokens,omitempty"`
	CostUSD      float64  `json:"cost_usd,omitempty"`
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
		if !known[result.Case] {
			return fmt.Errorf("results[%d]: unknown case %q", i, result.Case)
		}
		if result.Arm != ArmWith && result.Arm != ArmWithout {
			return fmt.Errorf("results[%d]: arm must be %q or %q, got %q", i, ArmWith, ArmWithout, result.Arm)
		}
		if err := checkMoney("cost_usd", result.CostUSD); err != nil {
			return fmt.Errorf("results[%d]: %w", i, err)
		}
		if result.InputTokens < 0 || result.OutputTokens < 0 {
			return fmt.Errorf("results[%d]: token counts must be >= 0", i)
		}
		if result.RubricScore != nil && (*result.RubricScore < 0 || *result.RubricScore > 1) {
			return fmt.Errorf("results[%d]: rubric_score must be between 0 and 1", i)
		}
	}
	return nil
}
