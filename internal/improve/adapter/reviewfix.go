package adapter

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// ClientFactory builds the model client of the adapter; tests replace it with a fake backend.
type ClientFactory func(lc llm.Config, opts llm.Options) (llm.Client, error)

// ReviewFixOptions configure the review-fix adapter.
type ReviewFixOptions struct {
	// LLM is the resolved model configuration (never a key: api_key_env names the variable).
	LLM llm.Config
	// FixerModel writes the fix; JudgeModel judges and verifies it (empty: the LLM model). They
	// must differ unless AllowSameModel is set: a model verifying its own edit is biased toward it.
	FixerModel     string
	JudgeModel     string
	AllowSameModel bool
	// Votes is the judge's votes per flagged dimension (0: the rubric's maximum).
	Votes int
	// MaxGrowthPercent bounds how much the fix may grow SKILL.md (0: the review default).
	MaxGrowthPercent int
	// Rubric is the rubric id (empty: the default builtin:skill-quality).
	Rubric string
	// Factory builds the client; nil uses llm.New.
	Factory ClientFactory
}

// RunReviewFix judges the workspace copy of SKILL.md with the review rubric and, when the judge
// finds stable problems, asks the fixer for edits that pass the review fix checks (they apply
// exactly, only the description and body change, no new link or credential, no new security
// finding, and a different judge model rates the targeted dimensions better and no other worse).
// A verified fix is written to the workspace; the gate decides whether it is kept. When it fails after calling the
// model it returns the error together with a response carrying the cost spent, which Serve prints.
func RunReviewFix(ctx context.Context, req *improve.OptimizerRequest, workspace string, o *ReviewFixOptions) (*improve.OptimizerResponse, error) {
	if err := o.Check(); err != nil {
		return nil, err
	}
	if strings.ContainsAny(req.Skill.Dir, `/\`) || req.Skill.Dir == "" || req.Skill.Dir == "." || req.Skill.Dir == ".." {
		return nil, fmt.Errorf("the request names a skill directory %q outside the workspace", req.Skill.Dir)
	}
	skillPath := filepath.Join(workspace, req.Skill.Dir, "SKILL.md")
	data, mode, err := safefs.ReadRegularKeepMode(skillPath)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	rb, err := loadRubric(o.Rubric)
	if err != nil {
		return nil, err
	}
	item := rv.Item{
		ID: rv.KindSkill + ":" + req.Skill.ID, Kind: rv.KindSkill, Name: req.Skill.ID, Path: req.Skill.Dir + "/SKILL.md", Owned: true, Abs: skillPath,
	}.WithText(string(data))
	res := rv.Run(rv.Input{Rubric: rb, Items: []rv.Item{item}, Content: config.ReviewContentFull})

	// The judge and the fixer share one client; the model of each call is set per request.
	lc := o.LLM
	if req.Budget.MaxCostUSD > 0 && (lc.MaxCostUSD <= 0 || req.Budget.MaxCostUSD < lc.MaxCostUSD) {
		lc.MaxCostUSD = req.Budget.MaxCostUSD // never spend more than the run has left
	}
	client, err := newClient(lc, o)
	if err != nil {
		return nil, err
	}
	defer client.Close() //nolint:errcheck // nothing to flush
	k := o.Votes
	if k <= 0 {
		k = max(rb.Votes.Max, 1)
	}
	judge := rv.SemanticOptions{Client: client, Model: o.JudgeModel, K: k, Content: config.ReviewContentFull, Workers: 1, NoCache: true}
	outcome, err := rv.RunSemantic(ctx, rv.SemanticInput{Rubric: rb, Results: res, Options: judge})
	if err != nil {
		return spentResponse(client, 0), fmt.Errorf("the judged run stopped: %w", err)
	}
	resp := &improve.OptimizerResponse{Version: improve.ProtocolVersion, Changed: []string{}, CostUSD: outcome.Usage.CostUSD}
	if outcome.Incomplete {
		resp.Summary = "the judged run was incomplete (" + outcome.StoppedBecause + "): no change"
		return resp, nil
	}
	findings := stableFindings(res, rb)
	if len(findings) == 0 || len(res.Items) == 0 {
		resp.Summary = "the judge found no stable problem in SKILL.md: no change"
		return resp, nil
	}
	growth := o.MaxGrowthPercent
	if growth <= 0 {
		growth = config.DefaultReviewFixMaxGrowthPercent
	}
	ir := &res.Items[0]
	prop, err := rv.ProposeFix(ctx, rv.FixInput{
		Rubric: rb, Item: *ir, Findings: findings, Pool: res.Pool(), Fixer: client, FixerModel: o.FixerModel,
		Verifier: rv.NewJudge(rb, judge), MaxGrowthPercent: growth, LintCheck: withTokenLimit(scanDelta(ir), req.Constraints.MaxSkillTokens),
		Feedback: previousRoundFeedback(req.Previous),
	})
	if prop != nil {
		resp.CostUSD += prop.Usage.CostUSD
	}
	if err != nil {
		return spentResponse(client, resp.CostUSD), fmt.Errorf("propose a fix: %w", err)
	}
	if !prop.Verified {
		resp.Summary = "no safe fix: " + prop.Reason
		return resp, nil
	}
	if err := safefs.WriteFileAtomicMode(skillPath, []byte(prop.Patched), mode); err != nil {
		return spentResponse(client, resp.CostUSD), fmt.Errorf("write SKILL.md: %w", err)
	}
	resp.Changed = []string{"SKILL.md"}
	resp.Summary = "review-fix: " + prop.Note
	resp.Notes = fmt.Sprintf("%s %s before %v, after %v, %d attempt(s)", prop.Code, prop.Dimension, prop.Before, prop.After, prop.Attempts)
	return resp, nil
}

// previousRoundFeedback tells the fixer how the loop's last round ended. An adapter is stateless (each round is a
// new process), so the request's previous field is the only memory it has. The loop already limits it: reasons are
// given only for rounds decided before the held-out set was consulted, so nothing here carries held-out data.
func previousRoundFeedback(prev *improve.PreviousRound) string {
	if prev == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Round %d ended: %s.", prev.Round, prev.Decision)
	if prev.WorkspaceKept {
		b.WriteString(" The file below still holds that attempt.")
	} else {
		b.WriteString(" The file below was reset to the state before it.")
	}
	if len(prev.Reasons) > 0 {
		b.WriteString("\nReasons:")
		for _, r := range prev.Reasons {
			fmt.Fprintf(&b, "\n- %s", r)
		}
	}
	if prev.Summary != "" {
		fmt.Fprintf(&b, "\nIts own summary: %s", prev.Summary)
	}
	return b.String()
}

// Check refuses settings the adapter cannot run with (AR9J9): no model, no network opt-in, no fixer
// model, or a fixer equal to the judge. The command checks it before asking for consent.
func (o *ReviewFixOptions) Check() error {
	judge := o.JudgeModel
	if judge == "" {
		judge = o.LLM.FullModel()
	}
	switch {
	case o.LLM.FullModel() == "" && o.FixerModel == "":
		return &RefusedError{Reason: "no model: set [llm] model in the user config, or pass --adapter-model"}
	case !o.LLM.AllowNetwork:
		return &RefusedError{Reason: "the review-fix adapter calls a model, which needs [llm] allow_network = true in the user config: " + llm.NetworkDisabledMessage}
	case o.FixerModel == "" && !o.AllowSameModel:
		return &RefusedError{Reason: "no fixer model: pass --adapter-model or set [review.fix] model; it must differ from the judge model " + judge}
	case o.FixerModel != "" && rv.SameModel(o.FixerModel, judge) && !o.AllowSameModel:
		return &RefusedError{Reason: fmt.Sprintf("the fixer and the judge are both %s: a model verifying its own edit is biased toward it; use a different --adapter-model or --allow-same-model", judge)}
	}
	return nil
}

func loadRubric(ref string) (*rv.Rubric, error) {
	if ref == "" {
		ref = config.DefaultReviewRubric
	}
	id, ok := strings.CutPrefix(ref, config.BuiltinRubricPrefix)
	if !ok {
		return nil, &RefusedError{Reason: fmt.Sprintf("rubric %q is not built in: the adapter runs in a throwaway workspace with no project rubrics (use builtin:<id>)", ref)}
	}
	rb, err := rv.LoadBuiltin(id)
	if err != nil {
		return nil, fmt.Errorf("load the rubric: %w", err)
	}
	return rb, nil
}

// spentResponse is the answer of a round that failed after it called the model: no change, and the cost spent so
// far (the client's own count when it keeps one, else known), so improve charges what the round actually cost.
func spentResponse(client llm.Client, known float64) *improve.OptimizerResponse {
	cost := known
	if s, ok := client.(interface{ Spent() llm.Spent }); ok {
		cost = max(cost, s.Spent().CostUSD)
	}
	return &improve.OptimizerResponse{Version: improve.ProtocolVersion, Changed: []string{}, CostUSD: cost, Summary: "the review-fix adapter failed: no change"}
}

func newClient(lc llm.Config, o *ReviewFixOptions) (llm.Client, error) {
	factory := o.Factory
	if factory == nil {
		factory = func(lc llm.Config, opts llm.Options) (llm.Client, error) {
			m, err := llm.New(lc, opts)
			if err != nil {
				return nil, err //nolint:wrapcheck // contextual below
			}
			return m, nil
		}
	}
	// No cache: a repeated round must ask again, and the throwaway home has no cache directory.
	client, err := factory(lc, llm.Options{NoCache: true})
	if err != nil {
		return nil, fmt.Errorf("build the model client: %w", err)
	}
	return client, nil
}

// stableFindings are the judged findings of the item that a fix may target: verdicts below pass
// that the votes agree on.
func stableFindings(res *rv.Results, rb *rv.Rubric) []rv.Finding {
	var out []rv.Finding
	for _, f := range res.Findings(rb) {
		if f.Origin == rv.OriginLLMJudge && f.Status == rv.SemJudged && !f.Baselined && f.ItemID != "" {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out
}

// withTokenLimit adds the run's SKILL.md token limit (constraints.max_skill_tokens) to a fix's checks, so
// a fix that the diff policy would reject is sent back to the fixer instead of being proposed.
func withTokenLimit(next func(string) ([]string, error), limit int) func(string) ([]string, error) {
	counter, err := tokens.New("")
	if limit <= 0 || err != nil {
		return next
	}
	return func(patched string) ([]string, error) {
		added, nerr := next(patched)
		if nerr != nil {
			return nil, nerr
		}
		if n := counter.Count(patched); n > limit {
			added = append(added, fmt.Sprintf("SKILL.md would be %d tokens, the limit is %d: make the edit shorter", n, limit))
		}
		return added, nil
	}
}

// scanDelta is the security gate of a fix: the patched text must not add a finding of any
// security rule the original did not have.
func scanDelta(ir *rv.ItemResult) func(string) ([]string, error) {
	return func(patched string) ([]string, error) {
		count := func(text string) map[string]int {
			m := map[string]int{}
			for _, f := range lint.ScanText(ir.Path, text) {
				m[f.Code]++
			}
			return m
		}
		before, after := count(ir.Raw), count(patched)
		var added []string
		for code, n := range after {
			if n > before[code] {
				added = append(added, fmt.Sprintf("%s x%d", code, n-before[code]))
			}
		}
		sort.Strings(added)
		return added, nil
	}
}
