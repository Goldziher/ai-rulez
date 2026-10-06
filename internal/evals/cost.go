package evals

import (
	"math"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/pricing"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// Price is a model price in USD per million tokens (the shared table in
// internal/pricing, which internal/llm prices model calls with too).
type Price = pricing.Price

// PriceFor returns the built-in price of a model. An empty model (the harness's
// own default) is priced at the sonnet tier and counts as known. known is false
// for a model name the table does not list; the returned price is then the sonnet
// tier, good enough to show an estimate but not to enforce --max-cost with.
func PriceFor(model string) (price Price, known bool) {
	if strings.TrimSpace(model) == "" || strings.EqualFold(model, "default") {
		p, _ := pricing.Lookup("sonnet")
		return p, true
	}
	if p, ok := pricing.Lookup(model); ok {
		return p, true
	}
	p, _ := pricing.Lookup("sonnet")
	return p, false
}

// Assumptions behind the estimate, per agent run. They are coarse on purpose.
const (
	// harnessOverheadTokens stands for the harness's own system prompt and tools.
	harnessOverheadTokens = 2000
	// assumedOutputTokens is the answer plus tool-call traffic of one run.
	assumedOutputTokens = 600
	judgeInputTokens    = 800
	judgeOutputTokens   = 100
	// activationOutputTokens is the output of one activation decision.
	activationOutputTokens = 150
)

// The range of an activation estimate is tighter than a case run's: one turn, no
// fixtures and no tool loop, so the input is the overhead, the installed set's
// descriptions and the prompt.
const (
	activationLowInputFactor   = 0.9
	activationHighInputFactor  = 1.25
	activationLowOutputFactor  = 0.5
	activationHighOutputFactor = 2.0
)

// The range around the expected figure. Low assumes the prompt and answer come
// in under the assumptions; high assumes a larger context, a tool loop that
// re-reads the skill and fixtures (toolLoopFactor times the tokens beyond the
// harness overhead) and a longer answer. They are initial defaults, documented in
// docs/evals.md, and the estimate tests pin them.
const (
	lowInputFactor   = 0.8
	lowOutputFactor  = 0.5
	highInputFactor  = 1.3
	highOutputFactor = 3.0
	toolLoopFactor   = 1.0
)

// Estimate is the projected size and cost of a run. The unsuffixed fields are the
// expected figure; the Low and High fields bound it.
type Estimate struct {
	AgentRuns        int     `json:"agent_runs"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	InputTokensLow   int     `json:"input_tokens_low"`
	InputTokensHigh  int     `json:"input_tokens_high"`
	OutputTokensLow  int     `json:"output_tokens_low"`
	OutputTokensHigh int     `json:"output_tokens_high"`
	CostLowUSD       float64 `json:"cost_low_usd"`
	CostHighUSD      float64 `json:"cost_high_usd"`
}

// EstimateParams are the assumptions behind an estimate. The defaults are the
// constants above; `ai-rulez eval calibrate-estimate` proposes measured values and
// [lint.evals.estimate] (or --overhead-tokens and friends) applies them.
type EstimateParams struct {
	// OverheadTokens is the harness's own input per agent run (system prompt and tools).
	OverheadTokens int
	// AssumedOutputTokens is the output per agent run.
	AssumedOutputTokens int
	// ToolLoopFactor scales the tool-loop re-read in the high figure.
	ToolLoopFactor float64
	// ActivationOutputTokens is the output of one activation decision (a skill
	// call or a short answer).
	ActivationOutputTokens int
}

// DefaultEstimateParams returns the built-in assumptions.
func DefaultEstimateParams() EstimateParams {
	return EstimateParams{OverheadTokens: harnessOverheadTokens, AssumedOutputTokens: assumedOutputTokens, ToolLoopFactor: toolLoopFactor,
		ActivationOutputTokens: activationOutputTokens}
}

// withDefaults fills the zero fields with the built-in assumptions.
func (p EstimateParams) withDefaults() EstimateParams {
	d := DefaultEstimateParams()
	if p.OverheadTokens <= 0 {
		p.OverheadTokens = d.OverheadTokens
	}
	if p.AssumedOutputTokens <= 0 {
		p.AssumedOutputTokens = d.AssumedOutputTokens
	}
	if p.ToolLoopFactor <= 0 {
		p.ToolLoopFactor = d.ToolLoopFactor
	}
	if p.ActivationOutputTokens <= 0 {
		p.ActivationOutputTokens = d.ActivationOutputTokens
	}
	return p
}

// EstimateRun projects the cost of running req, repeating every case runsPerCase
// times per arm. It is deterministic and offline.
func EstimateRun(req *Request, skillTokens, runsPerCase int, price Price, counter tokens.Counter) Estimate {
	return EstimateRunWith(DefaultEstimateParams(), req, skillTokens, runsPerCase, price, counter)
}

// EstimateRunWith is EstimateRun under explicit assumptions.
func EstimateRunWith(params EstimateParams, req *Request, skillTokens, runsPerCase int, price Price, counter tokens.Counter) Estimate {
	params = params.withDefaults()
	overhead, output := params.OverheadTokens, params.AssumedOutputTokens
	if runsPerCase < 1 {
		runsPerCase = 1
	}
	arms := 1
	if req.Ablation {
		arms = 2
	}
	var est Estimate
	var inLow, inHigh, outLow, outHigh float64
	for i := range req.Cases {
		c := &req.Cases[i]
		caseIn := overhead + counter.Count(c.Prompt)
		for _, f := range c.Files {
			caseIn += counter.Count(f.Content)
		}
		judgeIn, judgeOut := 0, 0
		if c.HasRubric() {
			judgeIn, judgeOut = judgeInputTokens+counter.Count(c.RubricText()), judgeOutputTokens
		}
		for arm := 0; arm < arms; arm++ {
			in := caseIn
			if arm == 0 {
				in += skillTokens
			}
			n := float64(runsPerCase)
			est.AgentRuns += runsPerCase
			est.InputTokens += (in + judgeIn) * runsPerCase
			est.OutputTokens += (output + judgeOut) * runsPerCase
			inLow += n * (lowInputFactor*float64(in) + float64(judgeIn))
			inHigh += n * (highInputFactor*float64(in) + params.ToolLoopFactor*float64(in-overhead) + float64(judgeIn))
			outLow += n * (lowOutputFactor*float64(output) + float64(judgeOut))
			outHigh += n * (highOutputFactor*float64(output) + float64(judgeOut))
		}
	}
	est.InputTokensLow, est.InputTokensHigh = int(math.Round(inLow)), int(math.Round(inHigh))
	est.OutputTokensLow, est.OutputTokensHigh = int(math.Round(outLow)), int(math.Round(outHigh))
	est.CostUSD = priceTokens(price, est.InputTokens, est.OutputTokens)
	est.CostLowUSD = priceTokens(price, est.InputTokensLow, est.OutputTokensLow)
	est.CostHighUSD = priceTokens(price, est.InputTokensHigh, est.OutputTokensHigh)
	return est
}

func priceTokens(price Price, in, out int) float64 {
	return round((float64(in)*price.InPerMTok + float64(out)*price.OutPerMTok) / 1e6)
}

// Add sums two estimates.
func (e Estimate) Add(o Estimate) Estimate {
	return Estimate{
		AgentRuns:        e.AgentRuns + o.AgentRuns,
		InputTokens:      e.InputTokens + o.InputTokens,
		OutputTokens:     e.OutputTokens + o.OutputTokens,
		CostUSD:          round(e.CostUSD + o.CostUSD),
		InputTokensLow:   e.InputTokensLow + o.InputTokensLow,
		InputTokensHigh:  e.InputTokensHigh + o.InputTokensHigh,
		OutputTokensLow:  e.OutputTokensLow + o.OutputTokensLow,
		OutputTokensHigh: e.OutputTokensHigh + o.OutputTokensHigh,
		CostLowUSD:       round(e.CostLowUSD + o.CostLowUSD),
		CostHighUSD:      round(e.CostHighUSD + o.CostHighUSD),
	}
}

// EstimateRecord sets the projected cost of a run next to the cost it reported.
// It is stored with the skill's result, so the history shows how far off the
// estimate tends to be.
type EstimateRecord struct {
	LowUSD         float64 `json:"low_usd"`
	ExpectedUSD    float64 `json:"expected_usd"`
	HighUSD        float64 `json:"high_usd"`
	ExpectedTokens int     `json:"expected_tokens"`
	// AgentRuns and the input/output split of the expected and the reported tokens
	// are what `eval calibrate-estimate` fits the assumptions to. The reported
	// split is absent when the runner reported none.
	AgentRuns            int `json:"agent_runs,omitempty"`
	ExpectedInputTokens  int `json:"expected_input_tokens,omitempty"`
	ExpectedOutputTokens int `json:"expected_output_tokens,omitempty"`
	ActualInputTokens    int `json:"actual_input_tokens,omitempty"`
	ActualOutputTokens   int `json:"actual_output_tokens,omitempty"`
	// ActualUSD and ActualTokens are what the runner reported (cost, and input plus
	// output tokens); both are absent when it reported neither.
	ActualUSD    float64 `json:"actual_usd,omitempty"`
	ActualTokens int     `json:"actual_tokens,omitempty"`
	// Error is actual/expected - 1 over USD; nil when there is no actual cost.
	Error *float64 `json:"error,omitempty"`
	// InRange says the actual cost fell within [low, high]; nil when there is no actual cost.
	InRange *bool `json:"in_range,omitempty"`
}

// WithUsage adds the run count and the input/output split of the estimate and of
// what the runner reported, and returns the record.
func (r *EstimateRecord) WithUsage(est *Estimate, inputTokens, outputTokens int) *EstimateRecord {
	r.AgentRuns, r.ExpectedInputTokens, r.ExpectedOutputTokens = est.AgentRuns, est.InputTokens, est.OutputTokens
	r.ActualInputTokens, r.ActualOutputTokens = inputTokens, outputTokens
	return r
}

// NewEstimateRecord pairs an estimate with the cost and tokens a run reported.
func NewEstimateRecord(est *Estimate, actualUSD float64, actualTokens int) *EstimateRecord {
	rec := &EstimateRecord{
		LowUSD: est.CostLowUSD, ExpectedUSD: est.CostUSD, HighUSD: est.CostHighUSD,
		ExpectedTokens: est.InputTokens + est.OutputTokens,
	}
	if actualUSD <= 0 {
		return rec
	}
	rec.ActualUSD, rec.ActualTokens = round(actualUSD), actualTokens
	inRange := actualUSD >= est.CostLowUSD && actualUSD <= est.CostHighUSD
	rec.InRange = &inRange
	if est.CostUSD > 0 {
		errRatio := round(actualUSD/est.CostUSD - 1)
		rec.Error = &errRatio
	}
	return rec
}

// EstimateActivation projects the cost of an activation request: every case runs
// req.Runs times (runs when unset), each run costing the harness overhead, the
// descriptions of the installed set and the prompt in, and one short decision out.
// It is deterministic and offline.
func EstimateActivation(params EstimateParams, req *Request, runs int, price Price, counter tokens.Counter) Estimate {
	params = params.withDefaults()
	if req.Runs > 0 {
		runs = req.Runs
	}
	if runs < 1 {
		runs = 1
	}
	listing := 0
	for i := range req.Skills {
		listing += counter.Count(req.Skills[i].ID + ": " + req.Skills[i].Description)
	}
	var est Estimate
	var inLow, inHigh, outLow, outHigh float64
	for i := range req.Cases {
		in := params.OverheadTokens + listing + counter.Count(req.Cases[i].Prompt)
		out := params.ActivationOutputTokens
		est.AgentRuns += runs
		est.InputTokens += in * runs
		est.OutputTokens += out * runs
		inLow += float64(runs) * activationLowInputFactor * float64(in)
		inHigh += float64(runs) * activationHighInputFactor * float64(in)
		outLow += float64(runs) * activationLowOutputFactor * float64(out)
		outHigh += float64(runs) * activationHighOutputFactor * float64(out)
	}
	est.InputTokensLow, est.InputTokensHigh = int(math.Round(inLow)), int(math.Round(inHigh))
	est.OutputTokensLow, est.OutputTokensHigh = int(math.Round(outLow)), int(math.Round(outHigh))
	est.CostUSD = priceTokens(price, est.InputTokens, est.OutputTokens)
	est.CostLowUSD = priceTokens(price, est.InputTokensLow, est.OutputTokensLow)
	est.CostHighUSD = priceTokens(price, est.InputTokensHigh, est.OutputTokensHigh)
	return est
}
