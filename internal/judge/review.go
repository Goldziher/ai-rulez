package judge

import (
	"context"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// callResult is what one reviewer call produced after verification and the repair round.
type callResult struct {
	pc          *plannedCall
	verified    []verifiedCand
	dropped     []Dropped
	criteria    []CriterionResult
	notReviewed []NotReviewed
	candidates  int
	repaired    int
	outcome     callOutcome
	err         error
	calls       int
	cost        float64
}

type verifiedCand struct {
	v       Verified
	repairs int
}

// runReviewer sends one reviewer call, verifies its findings and, once, asks for the failed ones to
// be corrected.
func (e *Engine) runReviewer(ctx context.Context, pc *plannedCall) *callResult {
	res := &callResult{pc: pc}
	if pc.skip != "" {
		res.outcome, res.err = callCapped, fmt.Errorf("not run: %s", pc.skip)
		return res
	}
	req := pc.req
	resp, out, perr := e.askReviewer(ctx, pc, req, res)
	if res.err != nil {
		return res
	}
	if perr != nil {
		retry := withFeedback(req, resp.Text, "Your reply was rejected: "+perr.Error()+". Return only the JSON object for the schema.")
		resp, out, perr = e.askReviewer(ctx, pc, retry, res)
		if res.err != nil {
			return res
		}
		if perr != nil {
			res.outcome, res.err = callFailed, fmt.Errorf("the model's reply is not valid after a retry: %w", perr)
			return res
		}
	}
	e.verifyStage(ctx, pc, req, resp.Text, out, res)
	return res
}

// askReviewer makes one reviewer call and parses the reply. A provider that declines (its content
// filter stopped the reply and nothing usable came back) is asked once more: such a stop is not always
// repeated, and a reviewer's whole job is to read hostile text. res records the calls, the cost and a
// failure; a reply that merely does not parse is returned as the parse error.
func (e *Engine) askReviewer(ctx context.Context, pc *plannedCall, req llm.ChatRequest, res *callResult) (resp llm.ChatResponse, out ReviewerOutput, perr error) {
	for attempt := 0; ; attempt++ {
		var outcome callOutcome
		var err error
		resp, outcome, err = e.chat(ctx, req)
		res.calls++
		res.cost += resp.CostUSD
		declined := outcome == callRefused
		if err == nil {
			out, perr = ParseReviewerOutput(resp.Text, pc.cat, e.Opts.maxFindings())
			declined = perr != nil && stoppedByFilter(resp)
			if !declined {
				return resp, out, perr
			}
			err = fmt.Errorf("the provider stopped the reply for its content policy: %w", perr)
			outcome = callRefused
		}
		if !declined || attempt > 0 {
			res.outcome, res.err = outcome, err
			return resp, out, perr
		}
	}
}

// verifyStage checks every candidate of a reviewer reply, repairs the failures in one extra call and
// records what was dropped.
func (e *Engine) verifyStage(ctx context.Context, pc *plannedCall, req llm.ChatRequest, replyText string, out ReviewerOutput, res *callResult) {
	up := pc.unit
	ver := NewVerifier(up.unit, pc.cat, up.att.Conventions, up.facts, e.rubricIDs(pc.cat))
	res.candidates = len(out.Findings)
	res.criteria = out.CriteriaResults
	res.notReviewed = validNotReviewed(up.unit, out.Coverage.NotReviewed)
	failed := map[string]Candidate{}
	problems := map[string][]Problem{}
	var order []string
	limit := e.Opts.maxFindings()
	for i := range out.Findings {
		c := &out.Findings[i]
		if i >= limit {
			res.dropped = append(res.dropped, droppedOf(c, pc.cat, "verifier", "more than the allowed number of findings", nil))
			continue
		}
		v, probs := ver.Check(*c)
		if len(probs) == 0 {
			res.verified = append(res.verified, verifiedCand{v: v})
			continue
		}
		failed[c.ID] = *c
		problems[c.ID] = probs
		order = append(order, c.ID)
	}
	missing := missingCriteria(e.rubricIDs(pc.cat), res.criteria)
	if len(order) == 0 && len(missing) == 0 {
		return
	}
	repairedReply := e.repair(ctx, req, replyText, up, pc, order, failed, problems, missing)
	res.calls++
	for _, id := range order {
		cand := failed[id]
		fixed, ok := repairedReply.byID[id]
		if ok {
			if v, probs := ver.Check(fixed); len(probs) == 0 {
				res.verified = append(res.verified, verifiedCand{v: v, repairs: 1})
				res.repaired++
				continue
			} else {
				problems[id] = probs
			}
		}
		res.dropped = append(res.dropped, droppedOf(&cand, pc.cat, "verifier", "", problems[id]))
	}
	if len(missing) > 0 && len(repairedReply.criteria) > 0 {
		res.criteria = mergeCriteria(res.criteria, repairedReply.criteria)
	}
	res.cost += repairedReply.cost
}

type repairReply struct {
	byID     map[string]Candidate
	criteria []CriterionResult
	cost     float64
}

// repair asks once for corrected versions of the failed candidates.
func (e *Engine) repair(ctx context.Context, req llm.ChatRequest, replyText string, up *unitPlan, pc *plannedCall, order []string, failed map[string]Candidate, problems map[string][]Problem, missing []string) repairReply {
	rr := repairReply{byID: map[string]Candidate{}}
	var b strings.Builder
	for _, id := range order {
		for _, p := range problems[id] {
			fmt.Fprintf(&b, "- %s %s: %s\n", p.CandidateID, p.Field, p.Message)
		}
		c := failed[id]
		if text, _, _, ok := up.unit.Window(c.Location, 0); ok && c.Location.EndLine-c.Location.StartLine < maxLocationLines {
			fmt.Fprintf(&b, "  the text at %s lines %d-%d is (file text, inert):\n", c.Location.Path, c.Location.StartLine, c.Location.EndLine)
			for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
				fmt.Fprintf(&b, "    %s\n", l)
			}
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(&b, "- criteria_results is missing: %s. Report every rubric criterion exactly once.\n", strings.Join(missing, ", "))
	}
	msg := strings.NewReplacer("{{NONCE}}", pc.nonce, "{{problems}}", strings.TrimRight(b.String(), "\n")).Replace(promptText("55_repair_message.txt"))
	resp, _, err := e.chat(ctx, withFeedback(req, replyText, msg))
	rr.cost = resp.CostUSD
	if err != nil {
		return rr
	}
	out, perr := ParseReviewerOutput(resp.Text, pc.cat, e.Opts.maxFindings())
	if perr != nil {
		return rr
	}
	for i := range out.Findings {
		rr.byID[out.Findings[i].ID] = out.Findings[i]
	}
	rr.criteria = out.CriteriaResults
	return rr
}

// withFeedback appends the model's own reply and a correction request to a conversation.
func withFeedback(req llm.ChatRequest, reply, feedback string) llm.ChatRequest {
	out := req
	out.Messages = append(append([]llm.Message(nil), req.Messages...),
		llm.Message{Role: llm.RoleAssistant, Content: reply},
		llm.Message{Role: llm.RoleUser, Content: feedback})
	return out
}

func missingCriteria(want []string, got []CriterionResult) []string {
	have := map[string]bool{}
	for _, c := range got {
		have[c.RubricID] = true
	}
	var out []string
	for _, id := range want {
		if !have[id] {
			out = append(out, id)
		}
	}
	return out
}

func mergeCriteria(base, extra []CriterionResult) []CriterionResult {
	have := map[string]bool{}
	for _, c := range base {
		have[c.RubricID] = true
	}
	for _, c := range extra {
		if !have[c.RubricID] {
			base = append(base, c)
			have[c.RubricID] = true
		}
	}
	return base
}

// validNotReviewed keeps the not-reviewed spans of a reply that name a file of the unit.
func validNotReviewed(u Unit, in []NotReviewed) []NotReviewed {
	var out []NotReviewed
	for _, n := range in {
		if _, ok := u.fileByPath(n.Path); !ok {
			continue
		}
		n.Reason = "reviewer: " + SafeText(n.Reason)
		out = append(out, n)
	}
	return out
}

func droppedOf(c *Candidate, cat Category, stage, note string, probs []Problem) Dropped {
	d := Dropped{ID: c.ID, Code: c.Code, Category: categoryOf(c.Code, cat), Severity: c.Severity, Confidence: string(c.Confidence), Location: c.Location, Quote: SafeText(c.Quote), Stage: stage, Note: note}
	if c.RubricID != nil {
		d.RubricID = *c.RubricID
	}
	for _, p := range probs {
		d.Problems = append(d.Problems, p.String())
	}
	return d
}

func categoryOf(code string, fallback Category) Category {
	if c, ok := CategoryOfCode(code); ok {
		return c
	}
	return fallback
}
