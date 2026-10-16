package judge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// RunMeta is what the caller knows about the run that the engine cannot: for the report.
type RunMeta struct {
	EgressHosts []string
	Skills      []LoadedSkill
	Notices     []string
	Changed     *ChangedInfo
}

// unitRun holds the results of one unit while the run is in flight.
type unitRun struct {
	up      *unitPlan
	results []*callResult // reviewer calls of the unit, in plan order
	cands   []*cand
	adj     *adjudication
}

// Run executes the plan: the reviewer calls (security first, in a deterministic order, at most
// Concurrency at a time), the verifier with its repair round, then one adjudicator call per unit with
// candidates, and finally the deterministic scoring.
func (e *Engine) Run(ctx context.Context, p *Plan, meta RunMeta) *Report {
	runs := make([]*unitRun, len(p.Units))
	byUnit := map[*unitPlan]*unitRun{}
	for i, up := range p.Units {
		runs[i] = &unitRun{up: up}
		byUnit[up] = runs[i]
	}
	results := make([]*callResult, len(p.Calls))
	mapParallel(ctx, e.Opts.concurrency(), len(p.Calls), func(i int) {
		results[i] = e.runReviewer(ctx, p.Calls[i])
		e.progress(p.Calls[i], results[i])
	})
	for i, c := range p.Calls {
		if results[i] == nil { // canceled before it started
			results[i] = &callResult{pc: c, outcome: callCapped, err: ctx.Err()}
		}
		ur := byUnit[c.unit]
		ur.results = append(ur.results, results[i])
	}
	for _, ur := range runs {
		ur.cands = e.mergeCandidates(ur)
	}
	var todo []*unitRun
	for _, ur := range runs {
		if len(ur.cands) > 0 {
			todo = append(todo, ur)
		}
	}
	mapParallel(ctx, e.Opts.concurrency(), len(todo), func(i int) {
		ur := todo[i]
		ur.adj = e.adjudicate(ctx, ur.up, ur.cands)
		if e.Opts.Progress != nil {
			e.Opts.Progress(fmt.Sprintf("adjudicated %s (%d candidates)", ur.up.unit.Path, len(ur.cands)))
		}
	})
	return e.finalize(p, runs, meta)
}

func (e *Engine) progress(pc *plannedCall, r *callResult) {
	if e.Opts.Progress == nil {
		return
	}
	status := "ok"
	switch {
	case r.err != nil && pc.skip != "":
		status = "skipped (" + pc.skip + ")"
	case r.err != nil:
		status = "failed"
	}
	e.Opts.Progress(fmt.Sprintf("%s review of %s: %s", pc.cat, pc.unit.unit.Path, status))
}

// mergeCandidates collects the verified candidates of a unit in a fixed order (security, governance,
// quality; chunk order; reply order), drops duplicates and gives each an id unique within the unit.
func (e *Engine) mergeCandidates(ur *unitRun) []*cand {
	var out []*cand
	seen := map[string]int{}
	counters := map[string]int{}
	for _, cat := range AllCategories() {
		for _, r := range ur.results {
			if r.pc.cat != cat {
				continue
			}
			for vi := range r.verified {
				vc := r.verified[vi]
				k := vc.v.Candidate
				key := FindingID(k.Code, k.Location.Path, k.Quote)
				if at, dup := seen[key]; dup {
					if better(k, out[at].v.Candidate) {
						out[at].v = vc.v
					}
					continue
				}
				catOfCode, _ := CategoryOfCode(k.Code)
				prefix := catOfCode.codePrefix()
				counters[prefix]++
				id := fmt.Sprintf("%s%d", prefix, counters[prefix])
				vc.v.Candidate.ID = id
				seen[key] = len(out)
				out = append(out, &cand{id: id, cat: catOfCode, v: vc.v, repairs: vc.repairs, stage: cat})
			}
		}
	}
	return out
}

func better(a, b Candidate) bool {
	if a.Severity.Rank() != b.Severity.Rank() {
		return a.Severity.Rank() > b.Severity.Rank()
	}
	return a.Confidence.Rank() > b.Confidence.Rank()
}

// finalize builds the report from the run state: findings after the adjudicator, suppressions,
// minimum confidence, scores, statuses and the decision.
func (e *Engine) finalize(p *Plan, runs []*unitRun, meta RunMeta) *Report {
	now := e.Opts.Clock.Now()
	rep := &Report{SchemaVersion: ReportSchemaVersion, Mode: ModeJudge}
	rep.Run = e.runInfo(p, meta)
	rep.Run.Provenance.CacheHits, rep.Run.Provenance.Calls = e.usage.cacheHits, e.usage.calls
	rep.Run.Provenance.InputTokens, rep.Run.Provenance.OutputTokens, rep.Run.Provenance.CostUSD = e.usage.input, e.usage.output, e.usage.cost
	var expired []Suppression
	for _, ur := range runs {
		ureport, exp := e.finalizeUnit(ur, now)
		expired = append(expired, exp...)
		rep.Units = append(rep.Units, ureport)
	}
	for _, s := range dedupeSuppressions(expired) {
		rep.Run.Notices = append(rep.Run.Notices, fmt.Sprintf("suppression %s %s expired on %s and no longer applies", s.Code, s.Path, s.Expires.AddDate(0, 0, -1).Format("2006-01-02")))
	}
	e.summarize(rep)
	return rep
}

func dedupeSuppressions(in []Suppression) []Suppression {
	seen := map[string]bool{}
	var out []Suppression
	for _, s := range in {
		k := s.Code + "|" + s.Path + "|" + s.Quote
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

func (e *Engine) runInfo(p *Plan, meta RunMeta) RunInfo {
	cats := e.Opts.categories()
	run := RunInfo{
		PromptVersion: PromptVersion, Categories: cats, Models: e.Opts.Models.Map(), EgressHosts: meta.EgressHosts,
		FailOn: e.Opts.FailOn.Strings(), MinConfidence: e.minConfidence(), LoadedSkills: meta.Skills,
		PromptOverrides: e.Opts.Prompts.Overridden(), Notices: append([]string(nil), meta.Notices...), Changed: meta.Changed,
		Estimate: p.Estimate,
	}
	run.Provenance.SchemaSHA256 = SchemaSHA256(e.schemas[CategorySecurity]) + "+" + SchemaSHA256(e.adjSchema)[:12]
	run.Provenance.PromptSHA256 = e.Opts.Prompts.PromptFingerprint()
	if llm.OmitsTemperature(e.Opts.Models.Security) && llm.OmitsTemperature(e.Opts.Models.Adjudicator) {
		run.Provenance.Temperature = nil
	} else {
		zero := 0.0
		run.Provenance.Temperature = &zero
	}
	return run
}

func (e *Engine) minConfidence() Confidence {
	if e.Opts.MinConfidence.Rank() < 0 || e.Opts.MinConfidence == "" {
		return ConfidenceLow
	}
	return e.Opts.MinConfidence
}

// finalizeUnit builds one unit's report.
func (e *Engine) finalizeUnit(ur *unitRun, now time.Time) (UnitReport, []Suppression) {
	up := ur.up
	u := up.unit
	rep := UnitReport{ID: u.ID, Kind: u.Kind, Path: u.Path, ContentSHA256: u.Hash, Status: u.Status, Reason: SafeText(u.Reason), Findings: []*Finding{}}
	for _, f := range u.Files {
		rep.Files = append(rep.Files, f.Path)
	}
	if u.Status != "" {
		rep.Complete = u.Status == StatusSkipped
		rep.Coverage = Coverage{NotReviewed: []NotReviewed{}}
		return rep, nil
	}
	rep.Attached = attachedReport(up.att)
	rep.Notes = append(rep.Notes, up.notes...)
	gaps := e.unitGaps(ur, &rep)
	findings, dropped := e.unitFindings(ur, &rep)
	findings = append(findings, e.staticOnly(ur)...)
	// Minimum confidence, then suppressions.
	var kept []*Finding
	for _, f := range findings {
		if f.Confidence.Rank() < e.minConfidence().Rank() {
			dropped = append(dropped, Dropped{ID: f.ID, Code: f.Code, Category: f.Category, Severity: f.Severity, Confidence: string(f.Confidence), Location: f.Location, Quote: f.Quote, Stage: "min_confidence"})
			continue
		}
		kept = append(kept, f)
	}
	kept, suppressed, expired := ApplySuppressions(kept, e.Opts.Suppress, now)
	sortFindings(kept)
	sortFindings(suppressed)
	rep.Findings, rep.Suppressed = nonNilFindings(kept), suppressed
	if e.Opts.ExplainDrops {
		rep.Dropped = dropped
	}
	rep.Scores = e.scores(kept, ur)
	rep.Gaps = gaps
	rep.Complete = len(gaps) == 0
	rep.Status = e.unitStatus(&rep)
	for _, r := range ur.results {
		rep.Calls += r.calls
		rep.CostUSD += r.cost
	}
	if ur.adj != nil {
		rep.Calls += ur.adj.calls
		rep.CostUSD += ur.adj.cost
	}
	rep.tally = unitTally{dropped: map[string]int{}}
	for i := range dropped {
		rep.tally.dropped[dropped[i].Stage]++
	}
	for _, r := range ur.results {
		rep.tally.candidates += r.candidates
		rep.tally.verified += len(r.verified)
		rep.tally.repaired += r.repaired
	}
	return rep, expired
}

func nonNilFindings(f []*Finding) []*Finding {
	if f == nil {
		return []*Finding{}
	}
	return f
}

func attachedReport(att Attachment) *Attached {
	if len(att.Conventions) == 0 {
		return nil
	}
	out := &Attached{}
	for i := range att.Conventions {
		c := &att.Conventions[i]
		ref := RuleRef{ID: c.ID, Paths: c.Paths, Why: c.Why, SHA256: c.SHA256}
		if strings.HasPrefix(c.ID, "rule:") {
			out.Rules = append(out.Rules, ref)
		} else {
			out.InstructionFiles = append(out.InstructionFiles, ref)
		}
	}
	return out
}

// unitGaps lists, as sentences, everything that kept the unit from being reviewed completely, and
// fills the unit's coverage.
func (e *Engine) unitGaps(ur *unitRun, rep *UnitReport) []string {
	var gaps []string
	cov := []NotReviewed{}
	for _, f := range ur.up.unit.Files {
		if f.Omitted != "" {
			cov = append(cov, NotReviewed{Path: f.Path, Reason: f.Omitted})
			gaps = append(gaps, f.Path+" "+f.Omitted)
		}
	}
	for _, m := range ur.up.missed {
		cov = append(cov, m)
		gaps = append(gaps, fmt.Sprintf("%s lines %d-%d: %s", m.Path, m.StartLine, m.EndLine, m.Reason))
	}
	failedBy := map[Category][]string{}
	for _, r := range ur.results {
		if r.outcome != callOK && r.err != nil {
			reason := SafeText(llm.RedactSecrets(r.err.Error()))
			if r.outcome == callRefused {
				reason = "refused by the provider's content filter"
			}
			failedBy[r.pc.cat] = append(failedBy[r.pc.cat], fmt.Sprintf("chunk %d/%d: %s", r.pc.chunk+1, r.pc.nChunks, reason))
		}
		cov = append(cov, r.notReviewed...)
	}
	for _, cat := range AllCategories() {
		if f := failedBy[cat]; len(f) > 0 {
			gaps = append(gaps, fmt.Sprintf("%s review incomplete: %s", cat, strings.Join(f, "; ")))
		}
	}
	if ur.adj != nil && ur.adj.err != nil {
		rep.Notes = append(rep.Notes, "the adjudicator did not answer, so the findings are unreviewed: "+SafeText(llm.RedactSecrets(ur.adj.err.Error())))
	}
	rep.Coverage = Coverage{NotReviewed: cov}
	rep.Criteria = e.criteriaReport(ur)
	return gaps
}

// criteriaReport merges the rubric results of every call of the unit; a criterion nobody reported
// is not_evaluated, never silently passed.
func (e *Engine) criteriaReport(ur *unitRun) []CriterionReport {
	var want []string
	for _, cat := range e.Opts.categories() {
		want = append(want, e.rubricIDs(cat)...)
	}
	if len(want) == 0 {
		return nil
	}
	got := map[string]CriterionReport{}
	for _, r := range ur.results {
		for _, c := range r.criteria {
			prev, ok := got[c.RubricID]
			if !ok || c.Result == CriterionFail || prev.Result == CriterionNotApplicable {
				got[c.RubricID] = CriterionReport{RubricID: c.RubricID, Result: c.Result, Note: SafeText(c.Note)}
			}
		}
	}
	// A verified JR-G06 finding is a failed criterion whatever the criteria list says.
	for _, c := range ur.cands {
		if c.v.Candidate.Code == codeRubricFailure && c.v.Candidate.RubricID != nil {
			id := *c.v.Candidate.RubricID
			got[id] = CriterionReport{RubricID: id, Result: CriterionFail, Note: got[id].Note}
		}
	}
	var out []CriterionReport
	seen := map[string]bool{}
	for _, id := range want {
		if seen[id] {
			continue
		}
		seen[id] = true
		if c, ok := got[id]; ok {
			out = append(out, c)
		} else {
			out = append(out, CriterionReport{RubricID: id, Result: "not_evaluated", Note: "the reviewer did not report this criterion"})
		}
	}
	return out
}

// unitFindings turns the unit's candidates and the adjudicator's decisions into findings and the
// list of what was dropped and why.
func (e *Engine) unitFindings(ur *unitRun, rep *UnitReport) ([]*Finding, []Dropped) {
	var findings []*Finding
	var dropped []Dropped
	for _, r := range ur.results {
		dropped = append(dropped, r.dropped...)
	}
	adj := ur.adj
	chains := newChainIndex(adj)
	for _, c := range ur.cands {
		f, d := findingOf(c, adj, chains)
		if d != nil {
			dropped = append(dropped, *d)
			continue
		}
		findings = append(findings, f)
	}
	if adj != nil {
		findings = append(findings, escalatedFindings(adj)...)
		rep.Notes = append(rep.Notes, adj.notes...)
		for _, id := range sortedKeys(adj.rejected) {
			rep.Notes = append(rep.Notes, fmt.Sprintf("%s: %s", id, adj.rejected[id]))
		}
	}
	findings = append(findings, e.rubricAbsenceFindings(ur, findings, dropped)...)
	return dedupeFindings(findings), dropped
}

// rubricAbsenceFindings raises a JR-G06 finding for every criterion the reviewers reported as failed
// that has no finding: a failure that is an absence has no text to quote, and one whose quote did not
// verify must not vanish with it. The finding is unit-level (line 1 of the primary file) and carries no
// quote; when the reviewer's own finding was dropped by the verifier its severity and confidence are kept
// and the finding says the quote did not verify. A criterion the adjudicator dropped on purpose stays dropped.
func (e *Engine) rubricAbsenceFindings(ur *unitRun, have []*Finding, dropped []Dropped) []*Finding {
	covered := map[string]bool{}
	for _, f := range have {
		if f.RubricID != nil {
			covered[*f.RubricID] = true
		}
	}
	for i := range dropped {
		if d := &dropped[i]; d.Code == codeRubricFailure && (d.Stage == roleAdjudicator || d.Stage == "merged") {
			covered[d.RubricID] = true
		}
	}
	file := primaryFilePath(ur.up.unit)
	var out []*Finding
	for _, cat := range e.Opts.categories() {
		for _, id := range e.rubricIDs(cat) {
			note, failed := failedCriterion(ur, id)
			if !failed || covered[id] {
				continue
			}
			covered[id] = true
			out = append(out, e.absenceFinding(cat, id, file, note, droppedForRubric(dropped, id)))
		}
	}
	return out
}

// failedCriterion reports whether any reviewer call marked the criterion failed, with its note.
func failedCriterion(ur *unitRun, id string) (note string, failed bool) {
	for _, r := range ur.results {
		for _, c := range r.criteria {
			if c.RubricID == id && c.Result == CriterionFail {
				return c.Note, true
			}
		}
	}
	return "", false
}

// droppedForRubric is the verifier's record of the reviewer's own JR-G06 finding for a criterion, if any.
func droppedForRubric(dropped []Dropped, id string) *Dropped {
	for i := range dropped {
		d := &dropped[i]
		if d.Code == codeRubricFailure && d.Stage == "verifier" && d.RubricID == id {
			return d
		}
	}
	return nil
}

func primaryFilePath(u Unit) string {
	for _, f := range u.Files {
		if len(f.Lines) > 0 && f.Path == u.Primary {
			return f.Path
		}
	}
	for _, f := range u.Files {
		if len(f.Lines) > 0 {
			return f.Path
		}
	}
	return u.Path
}

func (e *Engine) absenceFinding(cat Category, id, file, note string, was *Dropped) *Finding {
	sev, conf := SeverityMedium, ConfidenceMedium
	why := "The rubric criterion failed; the failure is an absence or has no verifiable quote, so it is reported for the unit as a whole."
	verified := true
	if was != nil {
		if was.Severity.Valid() {
			sev = was.Severity
		}
		if Confidence(was.Confidence).Rank() >= 0 {
			conf = Confidence(was.Confidence)
		}
		why = "The reviewer's quote for this failed criterion did not verify against the file and was removed; the failure is kept for the unit as a whole."
		verified = false
	}
	if note != "" {
		why += " Reviewer's note: " + SafeText(note)
	}
	rid := id
	k := Candidate{ID: "A-" + id, Code: codeRubricFailure, Verdict: VerdictConfirmed, Severity: sev, Confidence: conf, Title: "Rubric criterion fails: " + id, Location: Location{Path: file, StartLine: 1, EndLine: 1}, Why: why, RubricID: &rid}
	c := &cand{id: k.ID, cat: cat, v: Verified{Candidate: k}}
	f := newFinding(c, OriginCriteria)
	f.ID = FindingID(k.Code, file, "absent:"+id)
	f.Fingerprint = FingerprintOf(k.Code, file, "absent:"+id)
	f.Absence = true
	f.Verification = Verification{QuoteVerified: verified, LocationValid: true}
	f.Adjudication = Adjudication{Decision: DecisionUnreviewed, OriginalSeverity: string(sev), OriginalConfidence: string(conf)}
	return f
}

// chainIndex maps a candidate to the chain the adjudicator put it in and the severity that chain
// sets as its floor.
type chainIndex struct {
	of    map[string]string
	floor map[string]Severity
}

func newChainIndex(adj *adjudication) chainIndex {
	ci := chainIndex{of: map[string]string{}, floor: map[string]Severity{}}
	if adj == nil {
		return ci
	}
	for i := range adj.chains {
		ch := &adj.chains[i]
		id := fmt.Sprintf("C%d", i+1)
		for _, m := range ch.Members {
			ci.of[m] = id
			if s := Severity(ch.CombinedSeverity); s.Rank() > ci.floor[m].Rank() {
				ci.floor[m] = s
			}
		}
	}
	return ci
}

// findingOf turns a candidate and the adjudicator's decision on it into a finding, or, when the
// decision removes it, the record of what was dropped.
func findingOf(c *cand, adj *adjudication, chains chainIndex) (*Finding, *Dropped) {
	k := &c.v.Candidate
	f := newFinding(c, OriginReviewer)
	f.Verification.RepairAttempts = c.repairs
	if adj == nil || adj.err != nil || adj.decisions[c.id].CandidateID == "" {
		f.Adjudication = Adjudication{Decision: DecisionUnreviewed, OriginalSeverity: string(k.Severity), OriginalConfidence: string(k.Confidence)}
		return f, nil
	}
	d := adj.decisions[c.id]
	switch d.Decision {
	case DecisionDrop, DecisionMerge:
		return nil, droppedByAdjudicator(k, c, f.ID, &d)
	case DecisionDowngrade:
		f.Adjudication = Adjudication{Decision: DecisionDowngrade, ReasonCode: d.ReasonCode, OriginalSeverity: string(k.Severity), OriginalConfidence: string(k.Confidence), Note: SafeText(d.Note)}
		f.Severity, f.Confidence = Severity(d.FinalSeverity), Confidence(d.FinalConfidence)
	default:
		f.Adjudication = Adjudication{Decision: DecisionKeep, ReasonCode: d.ReasonCode, Note: SafeText(d.Note)}
	}
	if cid, ok := chains.of[c.id]; ok {
		f.Adjudication.ChainID = &cid
		if floor := chains.floor[c.id]; floor.Rank() > f.Severity.Rank() {
			f.Adjudication.OriginalSeverity = string(f.Severity)
			f.Severity = floor
		}
	}
	return f, nil
}

// droppedByAdjudicator is the record of a candidate the adjudicator dropped or merged.
func droppedByAdjudicator(k *Candidate, c *cand, id string, d *Decision) *Dropped {
	dd := droppedOf(k, c.cat, roleAdjudicator, SafeText(d.Note), nil)
	dd.ID, dd.ReasonCode = id, d.ReasonCode
	if d.CounterQuote != nil {
		dd.CounterQuote = SafeText(*d.CounterQuote)
	}
	if d.Decision == DecisionMerge {
		dd.Stage = "merged"
		if d.MergeInto != nil {
			dd.Note = "merged into " + *d.MergeInto
		}
	}
	return &dd
}

// escalatedFindings are the findings the adjudicator raised itself, already verified.
func escalatedFindings(adj *adjudication) []*Finding {
	var out []*Finding
	for i := range adj.escalated {
		c := &cand{id: fmt.Sprintf("E%d", i+1), v: adj.escalated[i]}
		c.cat, _ = CategoryOfCode(c.v.Candidate.Code)
		f := newFinding(c, OriginAdjudicator)
		f.Adjudication = Adjudication{Decision: DecisionKeep, ReasonCode: reasonGrounded, Note: "raised by the adjudicator"}
		out = append(out, f)
	}
	return out
}

// dedupeFindings keeps one finding per stable id, the more severe one.
func dedupeFindings(in []*Finding) []*Finding {
	at := map[string]int{}
	var out []*Finding
	for _, f := range in {
		if i, dup := at[f.ID]; dup {
			if f.Severity.Rank() > out[i].Severity.Rank() {
				out[i] = f
			}
			continue
		}
		at[f.ID] = len(out)
		out = append(out, f)
	}
	return out
}

func newFinding(c *cand, origin string) *Finding {
	k := c.v.Candidate
	id := FindingID(k.Code, k.Location.Path, k.Quote)
	f := &Finding{
		ID: id, Code: k.Code, Category: c.cat, Severity: k.Severity, Confidence: k.Confidence, Verdict: k.Verdict,
		Title: SafeText(k.Title), Location: k.Location, Quote: SafeText(k.Quote), Chain: safeChain(k.Chain), Why: SafeText(k.Why),
		BenignReading: SafeText(k.BenignReading), WhyNotBenign: SafeText(k.WhyNotBenign), Convention: safeConvention(k.Convention),
		RubricID: k.RubricID, StaticRefs: k.StaticRefs, AlsoAt: safeAlsoAt(k.AlsoAt), TokenWasteEstimate: k.TokenWasteEstimate,
		Origin: origin, Verification: Verification{QuoteVerified: true, LocationValid: true},
		Fingerprint: FingerprintOf(k.Code, k.Location.Path, k.Quote),
	}
	if k.SuggestedFix != nil {
		s := SafeText(*k.SuggestedFix)
		f.SuggestedFix = &s
	}
	return f
}

func safeChain(in []Step) []Step {
	out := make([]Step, len(in))
	for i, s := range in {
		s.Quote = SafeText(s.Quote)
		out[i] = s
	}
	return out
}

func safeAlsoAt(in []AlsoAt) []AlsoAt {
	out := make([]AlsoAt, len(in))
	for i, a := range in {
		a.Quote = SafeText(a.Quote)
		out[i] = a
	}
	return out
}

func safeConvention(c *ConventionRef) *ConventionRef {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Quote = SafeText(cp.Quote)
	return &cp
}

// staticOnlyRule is a finding that is reported straight from a static rule.
type staticOnlyRule struct {
	code string
	sev  Severity
	conf Confidence
	text string
}

// staticOnlyFor maps the static rules that are certain enough to report without a model to the
// finding they become. Hidden characters in an instruction file have no benign reading in an agent
// instruction, so the adjudicator never sees them and can never drop them.
func staticOnlyFor(code string) (staticOnlyRule, bool) {
	if code == "AR002" {
		return staticOnlyRule{"JR-S02", SeverityHigh, ConfidenceMedium, "Hidden characters in an instruction file (static analyzer AR002)"}, true
	}
	return staticOnlyRule{}, false
}

func (e *Engine) staticOnly(ur *unitRun) []*Finding {
	if !e.Opts.hasCategory(CategorySecurity) {
		return nil
	}
	var out []*Finding
	for _, fact := range ur.up.facts {
		m, ok := staticOnlyFor(fact.Code)
		if !ok {
			continue
		}
		f, ok := ur.up.unit.fileByPath(fact.Path)
		if !ok || fact.Line < 1 || fact.Line > len(f.Lines) {
			continue
		}
		quote := truncateRunes(strings.TrimSpace(f.Lines[fact.Line-1]), maxQuoteChars)
		if quote == "" {
			quote = "(line " + itoa(fact.Line) + ")"
		}
		k := Candidate{ID: fact.ID, Code: m.code, Verdict: VerdictConfirmed, Severity: m.sev, Confidence: m.conf, Title: m.text,
			Location: Location{Path: fact.Path, StartLine: fact.Line, EndLine: fact.Line}, Quote: quote, StaticRefs: []string{fact.ID}, Why: fact.Message}
		found := newFinding(&cand{id: fact.ID, cat: CategorySecurity, v: Verified{Candidate: k}}, OriginStaticOnly)
		found.Adjudication = Adjudication{Decision: DecisionUnreviewed}
		found.Verification = Verification{QuoteVerified: true, LocationValid: true}
		out = append(out, found)
	}
	return out
}

// scores computes the category scores; a category that did not run on this unit is null.
func (e *Engine) scores(findings []*Finding, ur *unitRun) Scores {
	var s Scores
	ran := map[Category]bool{}
	for _, r := range ur.results {
		if r.outcome == callOK {
			ran[r.pc.cat] = true
		}
	}
	for _, cat := range e.Opts.categories() {
		if !ran[cat] {
			continue
		}
		var in []*Finding
		for _, f := range findings {
			if f.Category == cat {
				in = append(in, f)
			}
		}
		v := CategoryScore(in)
		switch cat {
		case CategorySecurity:
			s.Security = &v
		case CategoryQuality:
			s.Quality = &v
		case CategoryGovernance:
			s.Governance = &v
		}
	}
	return s
}

// unitStatus: fail when a fail-on rule is met, partial when the review was incomplete, warn when
// there are findings below the threshold, else pass.
func (e *Engine) unitStatus(rep *UnitReport) string {
	switch {
	case e.Opts.FailOn.Fails(rep.Findings, rep.Scores):
		return StatusFail
	case !rep.Complete:
		return StatusPartial
	case len(rep.Findings) > 0:
		return StatusWarn
	}
	return StatusPass
}

// summarize fills the run summary and the decision from the units.
func (e *Engine) summarize(rep *Report) {
	s := &rep.Summary
	s.BySeverity, s.ByCategory = map[string]int{}, map[string]int{}
	cov := &rep.Run.Coverage
	cov.UnitsTotal = len(rep.Units)
	failed, incomplete := false, false
	g := &s.Grounding
	for ui := range rep.Units {
		u := &rep.Units[ui]
		switch u.Status {
		case StatusWithheld:
			cov.UnitsWithheld++
			incomplete = true
		case StatusSkipped:
			cov.UnitsSkipped++
		case StatusError:
			cov.UnitsErrored++
			incomplete = true
		default:
			if u.Complete {
				cov.UnitsFullyReviewed++
			} else {
				cov.UnitsPartial++
				incomplete = true
			}
		}
		failed = failed || u.Status == StatusFail
		for _, f := range u.Findings {
			s.BySeverity[string(f.Severity)]++
			s.ByCategory[string(f.Category)]++
		}
		g.Candidates += u.tally.candidates
		g.QuoteVerified += u.tally.verified
		g.Repaired += u.tally.repaired
		g.DroppedUngrounded += u.tally.dropped["verifier"]
		g.DroppedByAdjud += u.tally.dropped[roleAdjudicator] + u.tally.dropped["merged"]
		g.BelowMinConfidence += u.tally.dropped["min_confidence"]
		g.Suppressed += len(u.Suppressed)
	}
	switch {
	case failed:
		s.Decision = DecisionFail
	case incomplete:
		s.Decision = DecisionIncomplete
	default:
		s.Decision = DecisionPass
	}
}

// EstimateReport describes the plan without calling a model: which units would be sent (and what
// is attached to them), which are held back, and the worst-case cost.
func (e *Engine) EstimateReport(p *Plan, meta RunMeta) *Report {
	rep := &Report{SchemaVersion: ReportSchemaVersion, Mode: ModeEstimate}
	rep.Run = e.runInfo(p, meta)
	if n := p.Skipped(); n > 0 {
		rep.Run.Notices = append(rep.Run.Notices, fmt.Sprintf("%d of the %d planned calls fit under the caps; the other %d would not be made, so the units they belong to would be partial", p.Estimate.CallsFit, p.Estimate.Calls, n))
	}
	for _, up := range p.Units {
		u := up.unit
		ur := UnitReport{ID: u.ID, Kind: u.Kind, Path: u.Path, ContentSHA256: u.Hash, Status: u.Status, Reason: SafeText(u.Reason), Findings: []*Finding{}, Coverage: Coverage{NotReviewed: []NotReviewed{}}}
		for _, f := range u.Files {
			ur.Files = append(ur.Files, f.Path)
		}
		if u.Status == "" {
			ur.Status = StatusWouldSend
			ur.Attached = attachedReport(up.att)
			for _, f := range u.Files {
				if f.Omitted != "" {
					ur.Coverage.NotReviewed = append(ur.Coverage.NotReviewed, NotReviewed{Path: f.Path, Reason: f.Omitted})
				}
			}
			ur.Coverage.NotReviewed = append(ur.Coverage.NotReviewed, up.missed...)
			for _, c := range up.calls {
				if c.skip != "" {
					ur.Gaps = appendUnique(ur.Gaps, fmt.Sprintf("%s review not run: %s", c.cat, c.skip))
				}
			}
			ur.Complete = len(ur.Gaps) == 0 && len(ur.Coverage.NotReviewed) == 0
		}
		rep.Units = append(rep.Units, ur)
	}
	cov := &rep.Run.Coverage
	cov.UnitsTotal = len(rep.Units)
	for ui := range rep.Units {
		u := &rep.Units[ui]
		switch u.Status {
		case StatusWithheld:
			cov.UnitsWithheld++
		case StatusSkipped:
			cov.UnitsSkipped++
		default:
			if u.Complete {
				cov.UnitsFullyReviewed++
			} else {
				cov.UnitsPartial++
			}
		}
	}
	rep.Summary.BySeverity, rep.Summary.ByCategory = map[string]int{}, map[string]int{}
	rep.Summary.Decision = DecisionPass
	return rep
}
