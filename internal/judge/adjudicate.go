package judge

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// windowRadius is how many lines the adjudicator sees around a cited range.
const windowRadius = 8

// maxEscalations is how many new findings the adjudicator may add.
const maxEscalations = 3

// cand is a verified candidate on its way through the adjudicator.
type cand struct {
	id      string
	cat     Category
	v       Verified
	repairs int
	stage   Category
}

// securityDropReason reports whether a reason code may drop a security candidate: only a concrete
// benign context, and only with a counter-quote, because the cost of missing a concealed
// exfiltration is higher than reviewing a false lead.
func securityDropReason(code string) bool {
	switch code {
	case "prohibition_context", "documentation_or_example", "test_fixture_or_placeholder", "expected_for_declared_purpose",
		"static_literal_data", "lint_option_wording", "declared_service_credential", "no_sink_or_no_untrusted_input":
		return true
	}
	return false
}

// validReasonCode reports whether code is one of the reason codes of the adjudicator's schema.
func validReasonCode(code string) bool {
	switch code {
	case reasonGrounded, "prohibition_context", "documentation_or_example", "test_fixture_or_placeholder", "expected_for_declared_purpose",
		"static_literal_data", "lint_option_wording", "declared_service_credential", "no_sink_or_no_untrusted_input", "unverifiable_reference",
		"guidance_not_binding", "convention_not_in_scope", "duplicate", "severity_inflated", "confidence_inflated", "quote_context_mismatch":
		return true
	}
	return false
}

// adjudication is the outcome of the adjudicator for a unit.
type adjudication struct {
	decisions map[string]Decision
	chains    []ChainDecision
	escalated []Verified
	notes     []string
	// valid[id] is false for a decision that broke the rules and was replaced by a keep.
	rejected map[string]string
	calls    int
	cost     float64
	err      error
	refused  bool
	dropped  int
}

// candidateJSON is the form a candidate takes in the adjudicator's prompt.
func candidateJSON(c *cand) string {
	k := c.v.Candidate
	m := map[string]any{
		"id": c.id, "code": k.Code, keyCategory: c.cat, "verdict": k.Verdict, "severity": k.Severity, "confidence": k.Confidence,
		"title": k.Title, "location": k.Location, "quote": k.Quote, "chain": k.Chain, "why": k.Why,
		"benign_reading": k.BenignReading, "why_not_benign": k.WhyNotBenign, "static_refs": k.StaticRefs,
		"also_at": k.AlsoAt, "convention": k.Convention, "rubric_id": k.RubricID,
	}
	return string(jsonText(m))
}

// windowsFor renders the source around every cited range, merged per file, and returns the texts
// (one per window) so the counter-quotes can be checked against them.
func windowsFor(u Unit, cands []*cand) (rendered string, texts []string) {
	type rng struct {
		start, end int
		ids        []string
	}
	byFile := map[string][]rng{}
	add := func(loc Location, id string) {
		f, ok := u.fileByPath(loc.Path)
		if !ok || len(f.Lines) == 0 {
			return
		}
		byFile[loc.Path] = append(byFile[loc.Path], rng{max(loc.StartLine-windowRadius, 1), min(loc.EndLine+windowRadius, len(f.Lines)), []string{id}})
	}
	for _, c := range cands {
		add(c.v.Candidate.Location, c.id)
		for _, s := range c.v.Candidate.Chain {
			add(s.Location, c.id)
		}
		for _, a := range c.v.Candidate.AlsoAt {
			add(a.Location, c.id)
		}
	}
	var b strings.Builder
	for _, p := range sortedKeys(byFile) {
		rs := byFile[p]
		sort.Slice(rs, func(i, j int) bool { return rs[i].start < rs[j].start })
		var merged []rng
		for _, r := range rs {
			if n := len(merged); n > 0 && r.start <= merged[n-1].end+1 {
				if r.end > merged[n-1].end {
					merged[n-1].end = r.end
				}
				merged[n-1].ids = appendUnique(merged[n-1].ids, r.ids...)
				continue
			}
			merged = append(merged, r)
		}
		f, _ := u.fileByPath(p)
		for _, r := range merged {
			var w strings.Builder
			for i := r.start; i <= r.end; i++ {
				fmt.Fprintf(&w, "L%d| %s\n", i, f.Lines[i-1])
			}
			fmt.Fprintf(&b, "=== WINDOW %s lines %d-%d (cited by %s)\n%s", p, r.start, r.end, strings.Join(r.ids, ", "), w.String())
			texts = append(texts, w.String())
		}
	}
	return strings.TrimRight(b.String(), "\n"), texts
}

func appendUnique(list []string, add ...string) []string {
	for _, a := range add {
		found := false
		for _, l := range list {
			if l == a {
				found = true
				break
			}
		}
		if !found {
			list = append(list, a)
		}
	}
	return list
}

// conventionsFor renders the lines of the conventions the candidates cite, with a little context.
func conventionsFor(att Attachment, cands []*cand) string {
	cited := map[string][]int{}
	for _, c := range cands {
		if cv := c.v.Candidate.Convention; cv != nil {
			cited[cv.ID] = append(cited[cv.ID], cv.Line)
		}
	}
	if len(cited) == 0 {
		return noText
	}
	var b strings.Builder
	for ci := range att.Conventions {
		conv := &att.Conventions[ci]
		lines, ok := cited[conv.ID]
		if !ok || len(conv.Lines) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[convention id=%s  attached=%s]\n", conv.ID, conv.Why)
		seen := map[int]bool{}
		sort.Ints(lines)
		for _, l := range lines {
			for i := max(l-3, 1); i <= min(l+3, len(conv.Lines)); i++ {
				if !seen[i] {
					seen[i] = true
					fmt.Fprintf(&b, "L%d| %s\n", i, conv.Lines[i-1])
				}
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// adjudicate asks the adjudicator about the candidates of a unit and checks its answer.
func (e *Engine) adjudicate(ctx context.Context, up *unitPlan, cands []*cand) *adjudication {
	adj := &adjudication{decisions: map[string]Decision{}, rejected: map[string]string{}}
	windows, texts := windowsFor(up.unit, cands)
	var cj []string
	for _, c := range cands {
		cj = append(cj, candidateJSON(c))
	}
	probe := strings.Join(cj, "\n") + windows + renderFacts(up.facts)
	nonce := deriveNonce([]string{up.unit.Hash, roleAdjudicator, e.Opts.Prompts.PromptFingerprint(), PromptVersion}, append(unitTexts(up.unit), probe)...)
	user := strings.NewReplacer(
		"{{unit_id}}", up.unit.ID, "{{kind}}", string(up.unit.Kind), "{{unit_path}}", up.unit.Path, "{{unit_sha256}}", up.unit.Hash[:16],
		"{{prompt_version}}", PromptVersion, "{{NONCE}}", nonce,
		"{{facts}}", renderFacts(up.facts), "{{conventions}}", conventionsFor(up.att, cands),
		"{{candidates}}", strings.Join(cj, "\n"), "{{windows}}", windows,
	).Replace(promptText("56_adjudicator_user_template.txt"))
	req := llm.ChatRequest{
		Model: e.Opts.Models.Adjudicator,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: e.Opts.Prompts.AdjudicatorSystem(nonce)},
			{Role: llm.RoleUser, Content: user},
		},
		MaxTokens:      e.Opts.maxCompletion(),
		ResponseFormat: format("judge_adjudicator", e.adjSchema),
		PromptVersion:  PromptVersion + "/adjudicator",
		AcceptReply: func(text string) error {
			_, err := ParseAdjudicatorOutput(text)
			return err
		},
	}
	resp, out, perr := e.askAdjudicator(ctx, req, adj)
	if adj.err != nil {
		return adj
	}
	if perr == nil {
		if probs := checkAdjudication(&out, cands, texts); len(probs) > 0 {
			perr = fmt.Errorf("%s", strings.Join(probs, "; "))
		}
	}
	if perr != nil {
		msg := strings.NewReplacer("{{NONCE}}", nonce, "{{problems}}", "- "+strings.ReplaceAll(perr.Error(), "; ", "\n- ")).Replace(promptText("57_adjudicator_feedback.txt"))
		resp2, _, err2 := e.chat(ctx, withFeedback(req, resp.Text, msg))
		adj.calls++
		adj.cost += resp2.CostUSD
		if err2 == nil {
			if out2, perr2 := ParseAdjudicatorOutput(resp2.Text); perr2 == nil {
				out, perr = out2, nil
				if probs := checkAdjudication(&out, cands, texts); len(probs) > 0 {
					adj.notes = append(adj.notes, "the adjudicator's answer still broke consistency rules after one correction: "+strings.Join(probs, "; "))
				}
			}
		}
	}
	if perr != nil && len(out.Decisions) == 0 {
		adj.err = fmt.Errorf("the adjudicator's reply is not usable: %w", perr)
		return adj
	}
	e.applyAdjudication(up, cands, &out, texts, adj)
	return adj
}

// askAdjudicator makes the adjudicator call and parses the reply; like a reviewer call it is made
// once more when the provider declines. adj records the calls, the cost and a refusal.
func (e *Engine) askAdjudicator(ctx context.Context, req llm.ChatRequest, adj *adjudication) (resp llm.ChatResponse, out AdjudicatorOutput, perr error) {
	for attempt := 0; ; attempt++ {
		var outcome callOutcome
		var err error
		resp, outcome, err = e.chat(ctx, req)
		adj.calls++
		adj.cost += resp.CostUSD
		declined := outcome == callRefused
		if err == nil {
			out, perr = ParseAdjudicatorOutput(resp.Text)
			if declined = perr != nil && stoppedByFilter(resp); !declined {
				return resp, out, perr
			}
			err = fmt.Errorf("the provider stopped the reply for its content policy: %w", perr)
		}
		if !declined || attempt > 0 {
			adj.err, adj.refused = err, declined
			return resp, out, perr
		}
	}
}

// checkAdjudication lists the consistency rules the answer breaks.
func checkAdjudication(out *AdjudicatorOutput, cands []*cand, windowTexts []string) []string {
	byID := candidatesByID(cands)
	probs := decisionCountProblems(out.Decisions, cands)
	for i := range out.Decisions {
		d := &out.Decisions[i]
		c, ok := byID[d.CandidateID]
		if !ok {
			probs = append(probs, "decision for unknown candidate "+d.CandidateID)
			continue
		}
		probs = append(probs, decisionProblems(d, c, byID, windowTexts)...)
	}
	return probs
}

func candidatesByID(cands []*cand) map[string]*cand {
	byID := make(map[string]*cand, len(cands))
	for _, c := range cands {
		byID[c.id] = c
	}
	return byID
}

// decisionCountProblems reports a candidate with no decision or more than one.
func decisionCountProblems(decisions []Decision, cands []*cand) []string {
	seen := map[string]int{}
	for i := range decisions {
		seen[decisions[i].CandidateID]++
	}
	var probs []string
	for _, c := range cands {
		switch seen[c.id] {
		case 0:
			probs = append(probs, "candidate "+c.id+" has no decision")
		case 1:
		default:
			probs = append(probs, "candidate "+c.id+" has more than one decision")
		}
	}
	return probs
}

// decisionProblems checks one decision against the rules of its kind.
func decisionProblems(d *Decision, c *cand, byID map[string]*cand, windowTexts []string) []string {
	switch d.Decision {
	case DecisionKeep:
		return nil
	case DecisionDrop:
		var probs []string
		if !validReasonCode(d.ReasonCode) {
			probs = append(probs, c.id+": drop needs a reason_code")
		}
		if d.CounterQuote == nil || !VerbatimIn(*d.CounterQuote, windowTexts) {
			probs = append(probs, c.id+": drop needs a counter_quote taken verbatim from the windows")
		}
		return probs
	case DecisionDowngrade:
		if !downgradeOK(&c.v.Candidate, d) {
			return []string{c.id + ": downgrade needs final_severity and final_confidence lower than the candidate's"}
		}
		return nil
	case DecisionMerge:
		if d.MergeInto == nil || *d.MergeInto == c.id || byID[*d.MergeInto] == nil {
			return []string{c.id + ": merge_into must name another candidate"}
		}
		return nil
	}
	return []string{c.id + ": decision " + d.Decision + " is not keep, downgrade, drop or merge"}
}

func downgradeOK(k *Candidate, d *Decision) bool {
	sev, conf := Severity(d.FinalSeverity), Confidence(d.FinalConfidence)
	if !sev.Valid() || conf.Rank() < 0 {
		return false
	}
	return sev.Rank() <= k.Severity.Rank() && conf.Rank() <= k.Confidence.Rank() &&
		(sev.Rank() < k.Severity.Rank() || conf.Rank() < k.Confidence.Rank())
}

// applyAdjudication turns the decisions into the adjudication result, enforcing the asymmetric rule in
// code: a security candidate is dropped only with a valid concrete reason and a verbatim counter-quote.
func (e *Engine) applyAdjudication(up *unitPlan, cands []*cand, out *AdjudicatorOutput, windowTexts []string, adj *adjudication) {
	byID := candidatesByID(cands)
	for i := range out.Decisions {
		d := out.Decisions[i]
		c, ok := byID[d.CandidateID]
		if !ok {
			continue
		}
		if _, dup := adj.decisions[d.CandidateID]; dup {
			continue
		}
		adj.decisions[d.CandidateID] = vetDecision(&d, c, byID, windowTexts, adj)
	}
	settleMerges(adj)
	collectChains(out.Chains, adj)
	e.collectEscalations(up, out.Escalations, adj)
	adj.notes = append(adj.notes, out.CoverageNotes...)
}

// vetDecision returns the decision to apply: the model's, or a keep with the reason when the model's
// breaks a rule (a security drop without a concrete context and counter-quote, a downgrade that
// lowers nothing, a merge into nothing).
func vetDecision(d *Decision, c *cand, byID map[string]*cand, windowTexts []string, adj *adjudication) Decision {
	keep := func(why string) Decision {
		adj.rejected[c.id] = why
		return Decision{CandidateID: c.id, Decision: DecisionKeep, ReasonCode: reasonGrounded, Note: why}
	}
	switch d.Decision {
	case DecisionDrop:
		counterOK := d.CounterQuote != nil && VerbatimIn(*d.CounterQuote, windowTexts)
		if c.cat == CategorySecurity && (!securityDropReason(d.ReasonCode) || !counterOK) {
			return keep("a security drop needs a concrete benign context and a verbatim counter-quote; kept")
		}
		if !validReasonCode(d.ReasonCode) {
			d.ReasonCode = "unverifiable_reference"
		}
	case DecisionDowngrade:
		if !downgradeOK(&c.v.Candidate, d) {
			return keep("downgrade did not lower severity or confidence; kept")
		}
	case DecisionMerge:
		if target, ok := byID[valueOr(d.MergeInto)]; d.MergeInto == nil || !ok || target.id == c.id {
			return keep("merge target is not another candidate; kept")
		}
	}
	return *d
}

// settleMerges turns a merge into a candidate that is itself dropped or merged into a keep.
func settleMerges(adj *adjudication) {
	for id, d := range adj.decisions {
		if d.Decision != DecisionMerge {
			continue
		}
		if t := adj.decisions[*d.MergeInto]; t.Decision == DecisionDrop || t.Decision == DecisionMerge {
			adj.decisions[id] = Decision{CandidateID: id, Decision: DecisionKeep, ReasonCode: reasonGrounded, Note: "merge target did not survive; kept"}
		}
	}
}

// collectChains keeps the chains of at least two surviving members.
func collectChains(chains []ChainDecision, adj *adjudication) {
	for i := range chains {
		ch := &chains[i]
		var members []string
		for _, m := range ch.Members {
			if d, ok := adj.decisions[m]; ok && (d.Decision == DecisionKeep || d.Decision == DecisionDowngrade) {
				members = append(members, m)
			}
		}
		if len(members) >= minChainMembers && Severity(ch.CombinedSeverity).Valid() {
			adj.chains = append(adj.chains, ChainDecision{Members: members, CombinedSeverity: ch.CombinedSeverity, Story: SafeText(ch.Story)})
		}
	}
}

// minChainMembers is the fewest candidates that make a chain.
const minChainMembers = 2

// collectEscalations verifies the findings the adjudicator raised itself with the full evidence
// contract; at most maxEscalations are taken.
func (e *Engine) collectEscalations(up *unitPlan, escalations []Candidate, adj *adjudication) {
	for i := range escalations {
		if i >= maxEscalations {
			break
		}
		esc := &escalations[i]
		catOfCode, ok := CategoryOfCode(esc.Code)
		if !ok {
			adj.dropped++
			continue
		}
		ver := NewVerifier(up.unit, catOfCode, up.att.Conventions, up.facts, e.rubricIDs(catOfCode))
		v, probs := ver.Check(*esc)
		if len(probs) > 0 {
			adj.dropped++
			continue
		}
		adj.escalated = append(adj.escalated, v)
	}
}

func valueOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
