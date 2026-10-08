package review

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// Dimension statuses of a judged item.
const (
	// SemJudged is a verdict the votes agree on (or a single vote with nothing to compare).
	SemJudged = "judged"
	// SemUnstable is a flagged verdict whose votes disagree: it is reported at info and never gates.
	SemUnstable = "unstable"
	// SemPreempted is a dimension a twin error already answered: the judge was not asked.
	SemPreempted = "preempted"
	// SemSkipped is a dimension that was not judged (it needs the body, or there is no sibling).
	SemSkipped = "skipped"
	// SemError is a dimension the judge could not answer (never reported as a pass).
	SemError = "error"
)

// Origins of a finding.
const (
	OriginLintTwin = "lint-twin"
	OriginLLMJudge = "llm-judge"
	// OriginRun marks a note about the run itself (AR9G0 unjudged items, AR9G9).
	OriginRun = "run"
)

// maxConsecutiveFailures stops a run whose calls keep failing for a systematic reason.
const maxConsecutiveFailures = 5

// DefaultWorkers is how many items are judged at once.
const DefaultWorkers = 4

// SemDim is the judged result of one dimension for one item.
type SemDim struct {
	ID      string  `json:"id"`
	Code    string  `json:"code,omitempty"`
	Group   string  `json:"group"`
	Weight  float64 `json:"weight"`
	Status  string  `json:"status"`
	Verdict string  `json:"verdict,omitempty"`
	// Agreement is the share of votes equal to Verdict; it is the confidence.
	Agreement float64  `json:"agreement,omitempty"`
	Votes     []string `json:"votes,omitempty"`
	Evidence  []Quote  `json:"evidence,omitempty"`
	// Rationale and Suggestion come from the first vote that gave the final verdict.
	Rationale  string `json:"rationale,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	// DroppedVotes counts votes whose warn or fail was dropped for lack of a verbatim quote.
	DroppedVotes int `json:"dropped_votes,omitempty"`
	// PreemptedBy lists the twin codes that answered a preempted dimension.
	PreemptedBy []string `json:"preempted_by,omitempty"`
	Note        string   `json:"note,omitempty"`
	// Capped is true when the item was truncated: its verdicts are reported at info.
	Capped bool `json:"capped,omitempty"`

	severity string
}

// SemanticResult is the judge's result for one item.
type SemanticResult struct {
	// Score is the semantic score 0-100; nil when no dimension was judged.
	Score      *int     `json:"score"`
	Dimensions []SemDim `json:"dimensions"`
	Calls      int      `json:"calls"`
	Cached     int      `json:"cached"`
	Truncated  bool     `json:"truncated,omitempty"`
	// Incomplete is true when the spend cap cut the votes of this item short, or a dimension got no usable answer.
	Incomplete bool `json:"incomplete,omitempty"`
}

// RunUsage totals what a judged run used.
type RunUsage struct {
	Calls        int            `json:"calls"`
	Cached       int            `json:"cached"`
	Tokens       int            `json:"tokens"`
	CostUSD      float64        `json:"cost_usd"`
	Hallucinated int            `json:"hallucinated_evidence"`
	Models       map[string]int `json:"models,omitempty"`
}

// Add sums another run into u.
func (u *RunUsage) Add(o RunUsage) {
	u.Calls += o.Calls
	u.Cached += o.Cached
	u.Tokens += o.Tokens
	u.CostUSD += o.CostUSD
	u.Hallucinated += o.Hallucinated
	for m, n := range o.Models {
		if u.Models == nil {
			u.Models = map[string]int{}
		}
		u.Models[m] += n
	}
}

// ResolvedModels lists the model ids the provider reported, most used first.
func (u RunUsage) ResolvedModels() []string {
	ids := slices.Sorted(maps.Keys(u.Models))
	sort.SliceStable(ids, func(i, j int) bool { return u.Models[ids[i]] > u.Models[ids[j]] })
	return ids
}

// SemanticOptions configure a judge.
type SemanticOptions struct {
	Client llm.Client
	// Model overrides the client's default model per request ("" keeps it).
	Model string
	// K is the most votes for a flagged dimension; 0 uses the rubric's votes.max (at least 1).
	K       int
	Content string
	// Workers bounds how many items are judged at once; 0 uses DefaultWorkers.
	Workers int
	// NoCache bypasses the response cache (variance runs).
	NoCache bool
	// AllVotes asks every vote for every dimension instead of voting only on flagged ones, so
	// the consistency of the judge can be measured (review calibrate).
	AllVotes bool
	// ReverseSiblings presents the siblings in reverse order (the reorder probe).
	ReverseSiblings bool
}

// ErrFatal wraps an error that ends a judged run: a refused network, a bad key, a bad
// configuration, or failures that keep repeating.
var ErrFatal = errors.New("judged run stopped")

// Judge runs the model-judged review of items.
type Judge struct {
	rb     *Rubric
	opts   SemanticOptions
	system string

	mu     sync.Mutex
	usage  RunUsage
	failed int
}

// NewJudge returns a judge for rb.
func NewJudge(rb *Rubric, opts SemanticOptions) *Judge {
	if opts.Content == "" {
		opts.Content = config.ReviewContentDescriptions
	}
	if opts.K <= 0 {
		opts.K = max(rb.Votes.Max, 1)
	}
	if opts.Workers <= 0 {
		opts.Workers = DefaultWorkers
	}
	return &Judge{rb: rb, opts: opts, system: systemPrompt(rb)}
}

// Usage returns what the judge has used so far.
func (j *Judge) Usage() RunUsage {
	j.mu.Lock()
	defer j.mu.Unlock()
	u := j.usage
	u.Models = map[string]int{}
	for m, n := range j.usage.Models {
		u.Models[m] = n
	}
	return u
}

// itemUse counts the calls made for one item.
type itemUse struct{ calls, cached int }

func (j *Judge) account(resp llm.ChatResponse, use *itemUse) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if resp.Cached {
		j.usage.Cached++
		use.cached++
	} else {
		j.usage.Calls++
		use.calls++
		j.usage.Tokens += resp.Usage.Total()
		j.usage.CostUSD += resp.CostUSD
	}
	if resp.Model != "" {
		if j.usage.Models == nil {
			j.usage.Models = map[string]int{}
		}
		j.usage.Models[resp.Model]++
	}
}

// answered resets the failure streak once a reply parsed: a reply the contract rejects is a failure.
func (j *Judge) answered() {
	j.mu.Lock()
	j.failed = 0
	j.mu.Unlock()
}

func (j *Judge) hallucinated(n int) {
	if n == 0 {
		return
	}
	j.mu.Lock()
	j.usage.Hallucinated += n
	j.mu.Unlock()
}

// noteFailure counts a failed call; it reports whether the failures now keep repeating.
func (j *Judge) noteFailure() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.failed++
	return j.failed >= maxConsecutiveFailures
}

// classify turns a model error into what the run does about it: stop (fatal), stop
// because the spend cap is reached (budget), or carry on without that answer.
func classify(err error) (fatal, budget bool) {
	switch {
	case errors.Is(err, llm.ErrBudget):
		return false, true
	case errors.Is(err, llm.ErrNetworkDisabled), errors.Is(err, llm.ErrAuth), errors.Is(err, llm.ErrConfig), errors.Is(err, context.Canceled):
		return true, false
	}
	return false, false
}

// vote makes one call for one group and returns the checked verdicts of its dimensions.
func (j *Judge) vote(ctx context.Context, sp callSpec, temperature float64, use *itemUse) (verdicts map[string]DimVerdict, truncated bool, callErr error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		sp.retry = attempt
		built := buildCall(sp)
		truncated = truncated || built.Truncated
		resp, err := j.opts.Client.Chat(ctx, requestFor(sp, built, temperature, j.opts.NoCache, j.opts.Model))
		if err != nil {
			if errors.Is(err, llm.ErrContextLength) && sp.bodyTokens == 0 && sp.content == config.ReviewContentFull {
				// One more try with the body cut to half the limit; the verdicts stay capped at info.
				sp.bodyTokens = max(sp.rb.Limits.MaxItemTokens/2, 200)
				truncated = true
				attempt--
				lastErr = err
				continue
			}
			return nil, truncated, err
		}
		j.account(resp, use)
		parsed, perr := parseReply(resp.Text, sp.dims)
		if perr != nil {
			lastErr = perr
			continue
		}
		j.answered()
		out := map[string]DimVerdict{}
		halluc := 0
		for i := range sp.dims {
			d := &sp.dims[i]
			dv, h, dropped := checkEvidence(parsed[d.ID], *d, built.Corpus)
			halluc += h
			dv.dropped = dropped
			out[d.ID] = dv
		}
		j.hallucinated(halluc)
		return out, truncated, nil
	}
	return nil, truncated, oops.Wrapf(lastErr, "no usable reply after a retry")
}

// groupOutcome is the aggregated result of one group of dimensions.
type groupOutcome struct {
	dims      map[string]*SemDim
	truncated bool
	cut       bool
}

// judgeGroup judges the dimensions of one group: a first vote, then extra votes for
// the dimensions it flagged, up to K in all. An error is fatal to the run (or the
// spend cap); a failed answer only marks its dimensions.
func (j *Judge) judgeGroup(ctx context.Context, r *ItemResult, group string, dims []Dimension, sibs []Item, use *itemUse) (*groupOutcome, error) {
	out := &groupOutcome{dims: map[string]*SemDim{}}
	spec := j.groupSpec(r, group, dims, sibs)
	first, trunc, err := j.vote(ctx, spec(1), j.rb.Votes.FirstTemperature, use)
	out.truncated = trunc
	if err != nil {
		fatal, budget := classify(err)
		if fatal || (!budget && j.noteFailure()) {
			return nil, fmt.Errorf("%w: %w", ErrFatal, err)
		}
		if budget {
			return nil, err
		}
		for i := range dims {
			out.dims[dims[i].ID] = &SemDim{ID: dims[i].ID, Status: SemError, Note: "no answer: " + shortError(err)}
		}
		return out, nil
	}
	votes, err := j.extraVotes(ctx, spec, dims, first, out, use)
	if err != nil {
		return nil, err
	}
	for i := range dims {
		out.dims[dims[i].ID] = aggregateDim(dims[i], votes, j.rb.Votes.InstabilityThreshold)
	}
	return out, nil
}

// groupSpec returns the call of vote v for one group: the order of dimensions and siblings is
// derived from the item, so a vote is reproducible.
func (j *Judge) groupSpec(r *ItemResult, group string, dims []Dimension, sibs []Item) func(v int) callSpec {
	view := sendView(r.Item, r.Redacted)
	seed := r.Digest
	if seed == "" {
		seed = r.ID
	}
	return func(v int) callSpec {
		ordDims, ordSibs := orderFor(dims, sibs, v, seed)
		if j.opts.ReverseSiblings {
			for a, b := 0, len(ordSibs)-1; a < b; a, b = a+1, b-1 {
				ordSibs[a], ordSibs[b] = ordSibs[b], ordSibs[a]
			}
		}
		return callSpec{rb: j.rb, system: j.system, item: view, group: group, dims: ordDims, sibs: ordSibs, content: j.opts.Content, vote: v}
	}
}

// extraVotes asks the votes after the first, for the dimensions the first flagged (every one with
// AllVotes), up to K in all. It returns the votes that were cast; an error is fatal to the run.
func (j *Judge) extraVotes(ctx context.Context, spec func(v int) callSpec, dims []Dimension, first map[string]DimVerdict, out *groupOutcome, use *itemUse) ([]map[string]DimVerdict, error) {
	votes := []map[string]DimVerdict{first}
	flagged := flaggedIn(dims, votes)
	for v := 2; v <= j.opts.K && (len(flagged) > 0 || j.opts.AllVotes); v++ {
		next, t, err := j.vote(ctx, spec(v), j.rb.Votes.ExtraTemperature, use)
		out.truncated = out.truncated || t
		if err != nil {
			if fatal, budget := classify(err); fatal {
				return nil, fmt.Errorf("%w: %w", ErrFatal, err)
			} else if budget {
				out.cut = true
			}
			break
		}
		votes = append(votes, next)
		if !j.opts.AllVotes && len(votes) >= 2 && allAgree(flagged, votes) {
			break
		}
	}
	return votes, nil
}

func shortError(err error) string {
	s := err.Error()
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

// injectionDimension is the one dimension whose first-vote pass is never accepted alone: a
// single pass is the verdict an injected text is written to obtain, so it is always voted on.
const injectionDimension = "injection-intent"

// flaggedIn lists the dimensions the first vote gave something other than a pass, and the
// injection dimension whatever it gave.
func flaggedIn(dims []Dimension, votes []map[string]DimVerdict) []string {
	var out []string
	for i := range dims {
		d := &dims[i]
		if v, ok := votes[0][d.ID]; ok && (v.Verdict != VerdictPass || d.ID == injectionDimension) {
			out = append(out, d.ID)
		}
	}
	return out
}

// allAgree reports whether every vote gave each flagged dimension the same verdict.
func allAgree(flagged []string, votes []map[string]DimVerdict) bool {
	for _, id := range flagged {
		first := votes[0][id].Verdict
		for _, v := range votes[1:] {
			if got, ok := v[id]; ok && got.Verdict != first {
				return false
			}
		}
	}
	return true
}

// MedianVerdict is the median of verdicts ordered pass < warn < fail; with an even count
// a tie goes to the lower level.
func MedianVerdict(verdicts []string) string {
	ranks := make([]int, 0, len(verdicts))
	for _, v := range verdicts {
		if r := verdictRank(v); r >= 0 {
			ranks = append(ranks, r)
		}
	}
	if len(ranks) == 0 {
		return ""
	}
	sort.Ints(ranks)
	return verdictAt(ranks[(len(ranks)-1)/2])
}

// aggregateDim turns the votes of one dimension into its result. A first vote that
// passes is accepted as it is (precision over recall), except on the injection dimension; a flagged one takes the median
// of all votes, and a verdict the votes do not agree on (agreement below the rubric's
// instability threshold) is marked unstable.
func aggregateDim(d Dimension, votes []map[string]DimVerdict, threshold float64) *SemDim {
	sd := &SemDim{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, severity: d.Severity}
	var verdicts []string
	var series []DimVerdict
	for _, v := range votes {
		if dv, ok := v[d.ID]; ok {
			verdicts = append(verdicts, dv.Verdict)
			series = append(series, dv)
			if dv.dropped {
				sd.DroppedVotes++
			}
		}
	}
	if len(series) == 0 {
		sd.Status, sd.Note = SemError, "no vote answered this dimension"
		return sd
	}
	sd.Votes = verdicts
	final := verdicts[0]
	if final != VerdictPass || d.ID == injectionDimension {
		final = MedianVerdict(verdicts)
	}
	sd.Verdict = final
	same := 0
	for _, v := range verdicts {
		if v == final {
			same++
		}
	}
	sd.Agreement = math.Round(float64(same)/float64(len(verdicts))*1000) / 1000
	sd.Status = SemJudged
	if final != VerdictPass && sd.Agreement < threshold {
		sd.Status = SemUnstable
	}
	for _, dv := range series {
		if dv.Verdict == final {
			sd.Evidence, sd.Rationale, sd.Suggestion = dv.Evidence, dv.Rationale, dv.Suggestion
			break
		}
	}
	if sd.DroppedVotes > 0 {
		sd.Note = fmt.Sprintf("%d vote(s) dropped: a warn or fail without a verbatim quote", sd.DroppedVotes)
	}
	return sd
}

// ItemSemantic judges one scored item. pool holds the sendable views of every scored
// item (siblings are drawn from it). The error is nil, ErrFatal-wrapped, or the
// model layer's budget error (the spend cap was reached; the result so far is returned).
func (j *Judge) ItemSemantic(ctx context.Context, r *ItemResult, pool []Item) (*SemanticResult, error) {
	res := &SemanticResult{}
	use := &itemUse{}
	byID, firstErr := j.judgeGroups(ctx, r, pool, res, use)
	for _, sd := range byID {
		if sd.Status == SemError {
			// A dimension the judge could not answer leaves the item unvouched for.
			res.Incomplete = true
		}
	}
	for i := range j.rb.Dimensions {
		d := &j.rb.Dimensions[i]
		sd := byID[d.ID]
		switch {
		case sd != nil:
			sd.Code, sd.Group, sd.Weight, sd.severity = d.Code, d.Group, d.Weight, d.Severity
		case offlinePreempted(r, d.ID):
			sd = unjudgedDim(d, SemPreempted, "a lint rule already reports this: the judge was not asked")
			sd.Verdict, sd.PreemptedBy = VerdictFail, preemptedBy(r, d.ID)
		case d.NeedsBody && j.opts.Content != config.ReviewContentFull:
			sd = unjudgedDim(d, SemSkipped, "needs the body: use --content full")
		case firstErr != nil:
			sd = unjudgedDim(d, SemSkipped, "the run stopped before this dimension was judged")
		default:
			sd = unjudgedDim(d, SemSkipped, "not judged")
		}
		res.Dimensions = append(res.Dimensions, *sd)
	}
	res.Calls, res.Cached = use.calls, use.cached
	res.Score = semanticScore(res.Dimensions)
	return res, firstErr
}

// judgeGroups judges the intrinsic group, then the contextual one, and returns the answered
// dimensions by id. The first error stops the later groups; the flags of res are set as they arise.
func (j *Judge) judgeGroups(ctx context.Context, r *ItemResult, pool []Item, res *SemanticResult, use *itemUse) (map[string]*SemDim, error) {
	byID := map[string]*SemDim{}
	var firstErr error
	for _, group := range []string{GroupIntrinsic, GroupContextual} {
		dims := judgeDimensions(j.rb, r, group, j.opts.Content)
		var sibs []Item
		if group == GroupContextual && len(dims) > 0 {
			sibs = shortlist(pool, r.Item, j.rb.Limits.MaxSiblings)
			if len(sibs) == 0 {
				for i := range dims {
					byID[dims[i].ID] = unjudgedDim(&dims[i], SemSkipped, "no sibling to compare with")
				}
				dims = nil
			}
		}
		if len(dims) == 0 || firstErr != nil {
			continue
		}
		out, err := j.judgeGroup(ctx, r, group, dims, sibs, use)
		if err != nil {
			firstErr = err
			res.Incomplete = true
			continue
		}
		res.Truncated = res.Truncated || out.truncated
		res.Incomplete = res.Incomplete || out.cut
		for id, sd := range out.dims {
			if out.truncated {
				sd.Capped = true
			}
			byID[id] = sd
		}
	}
	return byID, firstErr
}

// unjudgedDim is the result of a dimension the judge gave no verdict.
func unjudgedDim(d *Dimension, status, note string) *SemDim {
	return &SemDim{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, severity: d.Severity, Status: status, Note: note}
}

func offlinePreempted(r *ItemResult, id string) bool {
	for i := range r.Dimensions {
		if r.Dimensions[i].ID == id && r.Dimensions[i].Preempted {
			return true
		}
	}
	return false
}

func preemptedBy(r *ItemResult, id string) []string {
	var out []string
	for i := range r.Dimensions {
		d := &r.Dimensions[i]
		if d.ID != id {
			continue
		}
		for _, e := range d.Evidence {
			if e.Severity == severityError {
				out = append(out, e.Code)
			}
		}
	}
	return out
}

// semanticScore applies the published formula to the dimensions the judge answered. A
// preempted, skipped or errored dimension is left out of both sums; an unstable verdict
// counts as a pass (it does not accuse). nil when nothing was judged.
func semanticScore(dims []SemDim) *int {
	sum, weights := 0.0, 0.0
	for i := range dims {
		d := &dims[i]
		switch d.Status {
		case SemJudged:
			sum += d.Weight * verdictValue[d.Verdict]
		case SemUnstable:
			sum += d.Weight
		default:
			continue
		}
		weights += d.Weight
	}
	if weights == 0 {
		return nil
	}
	s := int(math.Round(100 * sum / weights))
	return &s
}

// SemanticInput is what RunSemantic judges.
type SemanticInput struct {
	Rubric  *Rubric
	Results *Results
	Options SemanticOptions
}

// SemanticOutcome summarizes a judged run.
type SemanticOutcome struct {
	Usage RunUsage
	// Unjudged lists the scored items no call was made for (the run stopped first).
	Unjudged []string
	// Incomplete is true when the spend cap or a fatal error ended the run early, or cut votes short.
	Incomplete bool
	// Truncated lists the items whose body was cut before the judge saw it: the part it did not
	// see could hold anything, so such a run cannot gate.
	Truncated []string
	// StoppedBecause says why a run ended early: "budget", or the fatal error text.
	StoppedBecause string
}

// RunSemantic judges every scored item of in.Results, with bounded concurrency, and
// stores each result in the item. A reached spend cap stops the run without an error:
// the outcome is incomplete and lists what was left. A fatal error is returned with
// the partial outcome.
func RunSemantic(ctx context.Context, in SemanticInput) (*SemanticOutcome, error) {
	j := NewJudge(in.Rubric, in.Options)
	res := in.Results
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rs := &runStop{cancel: cancel}
	runPool(ctx, j.opts.Workers, len(res.Items), rs,
		func(i int) bool { return res.Items[i].Status == StatusScored },
		func(i int) {
			r := &res.Items[i]
			sem, err := j.ItemSemantic(ctx, r, res.pool)
			if err != nil {
				rs.record(err)
			}
			if sem != nil && (sem.Calls > 0 || sem.Cached > 0) {
				r.Semantic = sem
			}
		})

	out := &SemanticOutcome{Usage: j.Usage()}
	tallyOutcome(in.Rubric, res, j.opts.Content, out)
	switch {
	case rs.fatal != nil:
		out.Incomplete, out.StoppedBecause = true, strings.TrimPrefix(rs.fatal.Error(), ErrFatal.Error()+": ")
		return out, rs.fatal
	case rs.budget:
		out.Incomplete, out.StoppedBecause = true, "the spend cap was reached"
	}
	out.Incomplete = out.Incomplete || len(out.Unjudged) > 0
	return out, nil
}

// tallyOutcome lists which scored items were not judged, were cut short or were truncated.
func tallyOutcome(rb *Rubric, res *Results, content string, out *SemanticOutcome) {
	for i := range res.Items {
		r := &res.Items[i]
		if r.Status != StatusScored {
			continue
		}
		if r.Semantic == nil && hasJudgeableDims(rb, r, content, res.pool) {
			out.Unjudged = append(out.Unjudged, r.ID)
		} else if r.Semantic != nil && r.Semantic.Incomplete {
			out.Incomplete = true
		}
		if r.Semantic != nil && r.Semantic.Truncated {
			out.Truncated = append(out.Truncated, r.ID)
		}
	}
}

// runStop records what ends a judged run early; the workers of a run share it.
type runStop struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	// fatal is the first fatal error; budget is true once the spend cap was reached.
	fatal  error
	budget bool
}

// record notes a judge error and cancels the run. A fatal error is kept (unless it is the
// cancellation a spend-cap stop caused: a call cut short by the stop another worker asked for is
// not a second failure); any other error is the spend cap.
func (s *runStop) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !errors.Is(err, ErrFatal) {
		s.budget = true
		s.cancel()
		return
	}
	if s.fatal == nil && (!s.budget || !errors.Is(err, context.Canceled)) {
		s.fatal = err
		s.cancel()
	}
}

// stopped reports whether the run has been told to end.
func (s *runStop) stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fatal != nil || s.budget
}

// runPool calls work for each index below n that eligible accepts, on bounded workers, and stops
// handing out work once rs says the run ended or ctx is done.
func runPool(ctx context.Context, workers, n int, rs *runStop, eligible func(i int) bool, work func(i int)) {
	var wg sync.WaitGroup
	jobs := make(chan int)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				work(i)
			}
		}()
	}
	for i := range n {
		if !eligible(i) {
			continue
		}
		if rs.stopped() || ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

// hasJudgeableDims reports whether a call would be made for the item.
func hasJudgeableDims(rb *Rubric, r *ItemResult, content string, pool []Item) bool {
	for _, group := range []string{GroupIntrinsic, GroupContextual} {
		if len(judgeDimensions(rb, r, group, content)) == 0 {
			continue
		}
		if group == GroupContextual && len(shortlist(pool, r.Item, rb.Limits.MaxSiblings)) == 0 {
			continue
		}
		return true
	}
	return false
}

// VerdictTable lists the judged verdicts of a run: item id -> dimension id -> verdict.
func VerdictTable(res *Results) map[string]map[string]string {
	out := map[string]map[string]string{}
	for i := range res.Items {
		it := &res.Items[i]
		if it.Semantic == nil {
			continue
		}
		for j := range it.Semantic.Dimensions {
			d := &it.Semantic.Dimensions[j]
			if d.Status != SemJudged && d.Status != SemUnstable {
				continue
			}
			if out[it.ID] == nil {
				out[it.ID] = map[string]string{}
			}
			out[it.ID][d.ID] = d.Verdict
		}
	}
	return out
}

// CloneUnjudged copies the results without the judge's answers, so another model can judge the
// same items.
func (r *Results) CloneUnjudged() *Results {
	c := &Results{pool: r.pool, baselined: r.baselined}
	c.Items = make([]ItemResult, len(r.Items))
	copy(c.Items, r.Items)
	for i := range c.Items {
		c.Items[i].Semantic = nil
	}
	return c
}
