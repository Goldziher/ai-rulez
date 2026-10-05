package evals

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/tokens"
)

// Price is a model price in USD per million tokens.
type Price struct {
	InPerMTok  float64 `json:"in_per_mtok"`
	OutPerMTok float64 `json:"out_per_mtok"`
}

// PriceFor returns a rough price tier for a model name: haiku, sonnet and opus
// tiers, with sonnet for anything unrecognized. Prices change; these only feed
// the dry-run estimate and the --max-cost pre-flight check, and are overridden
// with --price-in and --price-out.
func PriceFor(model string) Price {
	name := strings.ToLower(model)
	switch {
	case strings.Contains(name, "haiku"):
		return Price{InPerMTok: 1, OutPerMTok: 5}
	case strings.Contains(name, "opus"):
		return Price{InPerMTok: 15, OutPerMTok: 75}
	default:
		return Price{InPerMTok: 3, OutPerMTok: 15}
	}
}

// Assumptions behind the estimate, per agent run. They are coarse on purpose.
const (
	// harnessOverheadTokens stands for the harness's own system prompt and tools.
	harnessOverheadTokens = 2000
	// assumedOutputTokens is the answer plus tool-call traffic of one run.
	assumedOutputTokens = 600
	judgeInputTokens    = 800
	judgeOutputTokens   = 100
)

// Estimate is the projected size and cost of a run.
type Estimate struct {
	AgentRuns    int     `json:"agent_runs"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// EstimateRun projects the cost of running req, repeating every case runsPerCase
// times per arm. It is deterministic and offline.
func EstimateRun(req *Request, skillTokens, runsPerCase int, price Price, counter tokens.Counter) Estimate {
	if runsPerCase < 1 {
		runsPerCase = 1
	}
	arms := 1
	if req.Ablation {
		arms = 2
	}
	var est Estimate
	for i := range req.Cases {
		c := &req.Cases[i]
		caseIn := harnessOverheadTokens + counter.Count(c.Prompt)
		for _, f := range c.Files {
			caseIn += counter.Count(f.Content)
		}
		for arm := 0; arm < arms; arm++ {
			in := caseIn
			if arm == 0 {
				in += skillTokens
			}
			est.AgentRuns += runsPerCase
			est.InputTokens += in * runsPerCase
			est.OutputTokens += assumedOutputTokens * runsPerCase
			if c.Rubric != "" {
				est.InputTokens += (judgeInputTokens + counter.Count(c.Rubric)) * runsPerCase
				est.OutputTokens += judgeOutputTokens * runsPerCase
			}
		}
	}
	est.CostUSD = round((float64(est.InputTokens)*price.InPerMTok + float64(est.OutputTokens)*price.OutPerMTok) / 1e6)
	return est
}

// Add sums two estimates.
func (e Estimate) Add(o Estimate) Estimate {
	return Estimate{
		AgentRuns:    e.AgentRuns + o.AgentRuns,
		InputTokens:  e.InputTokens + o.InputTokens,
		OutputTokens: e.OutputTokens + o.OutputTokens,
		CostUSD:      round(e.CostUSD + o.CostUSD),
	}
}
