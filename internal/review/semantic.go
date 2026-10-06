package review

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	ids := sortedKeys(u.Models)
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
func (j *Judge) vote(ctx context.Context, sp callSpec, temperature float64, use *itemUse) (map[string]DimVerdict, bool, error) {
	var lastErr error
	truncated := false
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
		for _, d := range sp.dims {
			dv, h, dropped := checkEvidence(parsed[d.ID], d, built.Corpus)
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
	view := sendView(r.Item, r.Redacted)
	seed := r.Digest
	if seed == "" {
		seed = r.ID
	}
	spec := func(v int) callSpec {
		ordDims, ordSibs := orderFor(dims, sibs, v, seed)
		if j.opts.ReverseSiblings {
			for a, b := 0, len(ordSibs)-1; a < b; a, b = a+1, b-1 {
				ordSibs[a], ordSibs[b] = ordSibs[b], ordSibs[a]
			}
		}
		return callSpec{rb: j.rb, system: j.system, item: view, group: group, dims: ordDims, sibs: ordSibs, content: j.opts.Content, vote: v}
	}
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
		for _, d := range dims {
			out.dims[d.ID] = &SemDim{ID: d.ID, Status: SemError, Note: "no answer: " + shortError(err)}
		}
		return out, nil
	}
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
	for _, d := range dims {
		out.dims[d.ID] = aggregateDim(d, votes, j.rb.Votes.InstabilityThreshold)
	}
	return out, nil
}

func shortError(err error) string {
	s := err.Error()
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

// flaggedIn lists the dimensions the first vote gave something other than a pass.
func flaggedIn(dims []Dimension, votes []map[string]DimVerdict) []string {
	var out []string
	for _, d := range dims {
		if v, ok := votes[0][d.ID]; ok && v.Verdict != VerdictPass {
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
// passes is accepted as it is (precision over recall); a flagged one takes the median
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
	if final != VerdictPass {
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
	byID := map[string]*SemDim{}
	var firstErr error
	for _, group := range []string{GroupIntrinsic, GroupContextual} {
		dims := judgeDimensions(j.rb, r, group, j.opts.Content)
		var sibs []Item
		if group == GroupContextual && len(dims) > 0 {
			sibs = shortlist(pool, r.Item, j.rb.Limits.MaxSiblings)
			if len(sibs) == 0 {
				for _, d := range dims {
					byID[d.ID] = &SemDim{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, Status: SemSkipped, severity: d.Severity, Note: "no sibling to compare with"}
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
	for _, sd := range byID {
		if sd.Status == SemError {
			// A dimension the judge could not answer leaves the item unvouched for.
			res.Incomplete = true
		}
	}
	for _, d := range j.rb.Dimensions {
		sd := byID[d.ID]
		switch {
		case sd != nil:
			sd.Code, sd.Group, sd.Weight, sd.severity = d.Code, d.Group, d.Weight, d.Severity
		case offlinePreempted(r, d.ID):
			sd = &SemDim{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, severity: d.Severity, Status: SemPreempted, Verdict: VerdictFail, PreemptedBy: preemptedBy(r, d.ID), Note: "a lint rule already reports this: the judge was not asked"}
		case d.NeedsBody && j.opts.Content != config.ReviewContentFull:
			sd = &SemDim{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, severity: d.Severity, Status: SemSkipped, Note: "needs the body: use --content full"}
		case firstErr != nil:
			sd = &SemDim{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, severity: d.Severity, Status: SemSkipped, Note: "the run stopped before this dimension was judged"}
		default:
			sd = &SemDim{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, severity: d.Severity, Status: SemSkipped, Note: "not judged"}
		}
		res.Dimensions = append(res.Dimensions, *sd)
	}
	res.Calls, res.Cached = use.calls, use.cached
	res.Score = semanticScore(res.Dimensions)
	return res, firstErr
}

func offlinePreempted(r *ItemResult, id string) bool {
	for _, d := range r.Dimensions {
		if d.ID == id && d.Preempted {
			return true
		}
	}
	return false
}

func preemptedBy(r *ItemResult, id string) []string {
	var out []string
	for _, d := range r.Dimensions {
		if d.ID != id {
			continue
		}
		for _, e := range d.Evidence {
			if e.Severity == "error" {
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
	for _, d := range dims {
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

// SemanticOutcome summarises a judged run.
type SemanticOutcome struct {
	Usage RunUsage
	// Unjudged lists the scored items no call was made for (the run stopped first).
	Unjudged []string
	// Incomplete is true when the spend cap or a fatal error ended the run early, or cut votes short.
	Incomplete bool
	// StoppedBecause says why a run ended early: "budget", or the fatal error text.
	StoppedBecause string
}

// RunSemantic judges every scored item of in.Results, with bounded concurrency, and
// stores each result in the item. A reached spend cap stops the run without an error:
// the outcome is incomplete and lists what was left. A fatal error is returned with
// the partial outcome.
func RunSemantic(ctx context.Context, in SemanticInput) (*SemanticOutcome, error) {
	j := NewJudge(in.Rubric, in.Options)
	out := &SemanticOutcome{}
	res := in.Results
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		fatalErr error
		budget   bool
	)
	jobs := make(chan int)
	for range j.opts.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r := &res.Items[i]
				sem, err := j.ItemSemantic(ctx, r, res.pool)
				if err != nil {
					mu.Lock()
					if errors.Is(err, ErrFatal) {
						// A call cut short by the stop another worker asked for is not a second failure.
						if fatalErr == nil && (!budget || !errors.Is(err, context.Canceled)) {
							fatalErr = err
							cancel()
						}
					} else {
						budget = true
						cancel()
					}
					mu.Unlock()
				}
				if sem != nil && (sem.Calls > 0 || sem.Cached > 0) {
					r.Semantic = sem
				}
			}
		}()
	}
	for i := range res.Items {
		if res.Items[i].Status != StatusScored {
			continue
		}
		mu.Lock()
		stop := fatalErr != nil || budget
		mu.Unlock()
		if stop || ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	out.Usage = j.Usage()
	for i := range res.Items {
		r := &res.Items[i]
		if r.Status != StatusScored {
			continue
		}
		if r.Semantic == nil && hasJudgeableDims(in.Rubric, r, j.opts.Content, res.pool) {
			out.Unjudged = append(out.Unjudged, r.ID)
		} else if r.Semantic != nil && r.Semantic.Incomplete {
			out.Incomplete = true
		}
	}
	switch {
	case fatalErr != nil:
		out.Incomplete, out.StoppedBecause = true, strings.TrimPrefix(fatalErr.Error(), ErrFatal.Error()+": ")
		return out, fatalErr
	case budget:
		out.Incomplete, out.StoppedBecause = true, "the spend cap was reached"
	}
	out.Incomplete = out.Incomplete || len(out.Unjudged) > 0
	return out, nil
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
		for _, d := range it.Semantic.Dimensions {
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
