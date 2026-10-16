package judge

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// Defaults of a run.
const (
	DefaultConcurrency = 4
	// DefaultMaxCompletion is the completion budget of one call. It includes the reasoning tokens of
	// a thinking model, which spend the same budget before the first byte of the answer.
	DefaultMaxCompletion = 6144
	// expectedReviewerCompletion and expectedAdjudicatorCompletion are the typical completions the
	// plan budgets for; the hard cap of the model layer still reserves the worst case per call.
	expectedReviewerCompletion    = 800
	expectedAdjudicatorCompletion = 600
	// adjudicatorShare is the fraction of units the plan expects to need an adjudicator call.
	adjudicatorShare = 0.5
	// adjudicatorBase is the candidates and the source windows an adjudicator call carries on top of its
	// system prompt and schema (measured at about 750 tokens for two candidates; the rest is slack).
	adjudicatorBase = 1000
)

// Models names the model of each role. A bare name is routed to the provider of the run.
type Models struct {
	Security, Quality, Governance, Adjudicator string
}

// For returns the reviewer model of a category.
func (m Models) For(c Category) string {
	switch c {
	case CategorySecurity:
		return m.Security
	case CategoryQuality:
		return m.Quality
	default:
		return m.Governance
	}
}

// Map lists the models by role, for the report.
func (m Models) Map() map[string]string {
	return map[string]string{"security": m.Security, "quality": m.Quality, "governance": m.Governance, roleAdjudicator: m.Adjudicator}
}

// Options configure an engine.
type Options struct {
	Categories    []Category
	FailOn        FailOn
	MinConfidence Confidence
	Attach        AttachOptions
	// Rubrics are the rubric skills each category's reviewer evaluates.
	Rubrics     map[Category][]Rubric
	Prompts     PromptSet
	Models      Models
	Concurrency int
	// RPM limits the calls started per minute (0 = no limit).
	RPM           int
	MaxFindings   int
	InputTokens   int
	MaxCompletion int
	ExplainDrops  bool
	Suppress      []Suppression
	// Caps are the plan-time limits; the model client enforces its own hard limits as well.
	Caps Caps
	// Clock is the time source (nil is the wall clock).
	Clock ambient.Clock
	// Progress, when set, receives one line per finished call.
	Progress func(string)
}

// Caps are the spend limits of a run. Zero means unlimited.
type Caps struct {
	MaxCostUSD float64
	MaxTokens  int
	MaxCalls   int
}

func (o Options) concurrency() int {
	if o.Concurrency > 0 {
		return o.Concurrency
	}
	return DefaultConcurrency
}

func (o Options) maxFindings() int {
	if o.MaxFindings > 0 {
		return o.MaxFindings
	}
	return defaultMaxFindingsPerStage
}

func (o Options) inputTokens() int {
	if o.InputTokens > 0 {
		return o.InputTokens
	}
	return DefaultInputTokens
}

func (o Options) maxCompletion() int {
	if o.MaxCompletion > 0 {
		return o.MaxCompletion
	}
	return DefaultMaxCompletion
}

func (o Options) categories() []Category {
	if len(o.Categories) == 0 {
		return AllCategories()
	}
	// priority order, whatever order the caller listed them in
	var out []Category
	for _, c := range AllCategories() {
		for _, w := range o.Categories {
			if w == c {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func (o Options) hasCategory(c Category) bool {
	for _, w := range o.categories() {
		if w == c {
			return true
		}
	}
	return false
}

// Input is what a run judges.
type Input struct {
	Units []Unit
	// Facts are the static leads of each unit, by unit id.
	Facts map[string][]Fact
	// Repo is the project, for attaching conventions.
	Repo Repo
	// Notices are carried into the report.
	Notices []string
	Changed *ChangedInfo
	// EgressHosts are the hosts content would be sent to.
	EgressHosts []string
}

// Engine runs the pipeline.
type Engine struct {
	Client  llm.Client
	Pricing llm.Pricing
	Opts    Options

	schemas   map[Category]map[string]any
	adjSchema map[string]any
	mu        sync.Mutex
	usage     usageTotals
	stopped   atomic.Value // string: why no further call is made
	// jsonMode is set once a provider has refused the structured-output request: later calls ask for
	// the schema in the prompt and rely on the strict validation in Go.
	jsonMode atomic.Bool
	limiter  rateLimiter
}

type usageTotals struct {
	calls, cacheHits, input, output int
	cost                            float64
}

// NewEngine returns an engine over a model client.
func NewEngine(c llm.Client, pricing llm.Pricing, opts Options) *Engine {
	e := &Engine{Client: c, Pricing: pricing, Opts: opts, schemas: map[Category]map[string]any{}}
	for _, cat := range AllCategories() {
		e.schemas[cat] = reviewerSchema(cat)
	}
	e.adjSchema = adjudicatorSchema()
	e.limiter = rateLimiter{interval: rpmInterval(opts.RPM), clock: opts.Clock}
	return e
}

func rpmInterval(rpm int) time.Duration {
	if rpm <= 0 {
		return 0
	}
	return time.Minute / time.Duration(rpm)
}

// rateLimiter spaces call starts. Waiting is deterministic in count, not in timing.
type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
	clock    ambient.Clock
}

func (r *rateLimiter) wait(ctx context.Context) error {
	if r.interval <= 0 {
		return nil
	}
	r.mu.Lock()
	now := r.clock.Now()
	at := r.next
	if at.Before(now) {
		at = now
	}
	r.next = at.Add(r.interval)
	r.mu.Unlock()
	d := at.Sub(now)
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("rate limit wait: %w", ctx.Err())
	}
}

// plannedCall is one reviewer call, built but not sent.
type plannedCall struct {
	unit     *unitPlan
	cat      Category
	chunk    int
	nChunks  int
	req      llm.ChatRequest
	nonce    string
	prompt   int
	skip     string // why the call will not be made (cap reached)
	expected float64
	worst    float64
}

// unitPlan is a unit with everything its calls need.
type unitPlan struct {
	idx      int
	unit     Unit
	facts    []Fact
	att      Attachment
	siblings string
	calls    []*plannedCall
	missed   []NotReviewed
	// notes are facts about the plan the report carries (a reviewer that did not apply).
	notes []string
	// rubrics by tier for this unit, per category
}

// Plan is a run before any call: the units, their requests and the estimate.
type Plan struct {
	Units    []*unitPlan
	Calls    []*plannedCall
	Estimate Estimate
	input    Input
}

// Plan builds every reviewer request of the run and its estimate, and applies the caps to the
// order of work: calls are admitted by category priority (security first), then unit order, until
// the expected cost, tokens or calls reach a cap. The calls left over are reported, not sent.
func (e *Engine) Plan(in Input) *Plan {
	p := &Plan{input: in}
	siblings := siblingIndex(in.Units)
	for i := range in.Units {
		u := in.Units[i]
		up := &unitPlan{idx: i, unit: u, facts: limitFacts(in.Facts[u.ID])}
		if u.Status == "" {
			up.att = Attach(in.Repo, u, e.attachOptions())
			up.siblings = siblings[u.ID]
			e.buildCalls(up)
		}
		p.Units = append(p.Units, up)
	}
	// Priority order across units: all security calls, then governance, then quality.
	for _, cat := range e.Opts.categories() {
		for _, up := range p.Units {
			for _, c := range up.calls {
				if c.cat == cat {
					p.Calls = append(p.Calls, c)
				}
			}
		}
	}
	e.estimate(p)
	e.admit(p)
	p.Estimate.CallsFit = p.Estimate.Calls - p.Skipped()
	return p
}

func (e *Engine) attachOptions() AttachOptions { return e.Opts.Attach }

func limitFacts(f []Fact) []Fact {
	if len(f) > maxStaticFactsPerUnit {
		return f[:maxStaticFactsPerUnit]
	}
	return f
}

// siblingIndex lists, for each unit, the other units of its kind in the same place with their
// descriptions: the quality reviewer needs them to judge overlap, and only them.
func siblingIndex(units []Unit) map[string]string {
	group := func(u *Unit) string {
		dir := path.Dir(u.Path)
		if u.Kind == KindSkill {
			dir = path.Dir(dir)
		}
		return string(u.Kind) + "|" + dir
	}
	by := map[string][]*Unit{}
	for i := range units {
		u := &units[i]
		if u.Status == "" && (u.Kind == KindSkill || u.Kind == KindAgent || u.Kind == KindCommand) {
			by[group(u)] = append(by[group(u)], u)
		}
	}
	out := map[string]string{}
	for _, list := range by {
		for _, u := range list {
			var parts []string
			for _, o := range list {
				if o.ID == u.ID {
					continue
				}
				desc := truncateRunes(o.Description(), 200)
				if desc == "" {
					desc = "(no description)"
				}
				parts = append(parts, o.ID+": "+desc)
				if len(parts) >= maxSiblings {
					break
				}
			}
			out[u.ID] = strings.Join(parts, " | ")
		}
	}
	return out
}

// instructions renders the T1 reviewer instructions of a category (prompt extension and T1 rubrics)
// and the T2 rubric blocks, which travel with the conventions.
func (e *Engine) instructions(cat Category) (t1, t2 string) {
	var one, two []string
	if ext := strings.TrimSpace(e.Opts.Prompts.Extend[cat]); ext != "" {
		one = append(one, "[prompt extension]\n"+defuseMarkers(llm.RedactSecrets(ext)))
	}
	for _, r := range e.Opts.Rubrics[cat] {
		if r.Tier == TierConfigured {
			one = append(one, r.Block())
		} else {
			two = append(two, r.Block())
		}
	}
	return strings.Join(one, "\n\n"), strings.Join(two, "\n\n")
}

func (e *Engine) rubricIDs(cat Category) []string {
	var ids []string
	for _, r := range e.Opts.Rubrics[cat] {
		ids = append(ids, r.CriterionIDs()...)
	}
	return ids
}

func (e *Engine) buildCalls(up *unitPlan) {
	for _, cat := range e.Opts.categories() {
		if cat == CategoryGovernance && len(up.att.Conventions) == 0 && len(e.Opts.Rubrics[cat]) == 0 && up.unit.Kind != KindRule {
			// The governance reviewer enforces only written conventions; with none attached and no
			// rubric there is nothing to enforce, and a call would only pay for the prompt.
			up.notes = append(up.notes, "governance review skipped: no convention or rubric applies to this unit")
			continue
		}
		t1, t2 := e.instructions(cat)
		mode := convModeOmit
		switch cat {
		case CategoryGovernance:
			mode = convModeFull
		case CategoryQuality:
			mode = convModeIndex
		}
		convs := up.att.Conventions
		// Fixed overhead of every chunk: the system prompt, the facts, the conventions, the rubric.
		fixed := llm.EstimatePromptTokens(llm.ChatRequest{Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: e.Opts.Prompts.ReviewerSystem(cat, "0", e.Opts.maxFindings())},
			{Role: llm.RoleUser, Content: renderFacts(up.facts) + renderConventions(convs, mode) + t1 + t2},
		}})
		chunks, missed := PlanChunks(up.unit, max(e.Opts.inputTokens()-fixed, 4000))
		if up.missed == nil {
			up.missed = missed // the same spans are out of reach of every category
		}
		for _, ch := range chunks {
			in := reviewerInput{Unit: up.unit, Category: cat, Chunk: ch, ChunkCount: len(chunks), Facts: up.facts, Conv: convs, ConvMode: mode, Instr: t1, ExtraConv: t2, Siblings: up.siblings}
			pc := e.reviewerCall(up, in)
			up.calls = append(up.calls, pc)
		}
	}
}

func (e *Engine) reviewerCall(up *unitPlan, in reviewerInput) *plannedCall {
	probe := in
	probe.Nonce = ""
	text := probe.render()
	nonce := deriveNonce([]string{up.unit.Hash, string(in.Category), itoa(in.Chunk.Index), e.Opts.Prompts.PromptFingerprint(), PromptVersion}, append(unitTexts(up.unit), text)...)
	in.Nonce = nonce
	user := in.render()
	cat := in.Category
	req := llm.ChatRequest{
		Model: e.Opts.Models.For(cat),
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: e.Opts.Prompts.ReviewerSystem(cat, nonce, e.Opts.maxFindings())},
			{Role: llm.RoleUser, Content: user},
		},
		MaxTokens:      e.Opts.maxCompletion(),
		ResponseFormat: format("judge_"+string(cat), e.schemas[cat]),
		PromptVersion:  PromptVersion + "/" + string(cat),
		AcceptReply: func(text string) error {
			_, err := ParseReviewerOutput(text, cat, e.Opts.maxFindings())
			return err
		},
	}
	return &plannedCall{unit: up, cat: cat, chunk: in.Chunk.Index, nChunks: in.ChunkCount, req: req, nonce: nonce, prompt: llm.EstimatePromptTokens(req)}
}

// estimate fills the plan's estimate: the worst case (every call may use its whole completion
// budget, every unit with a candidate is adjudicated) and the expected cost, which the caps use.
func (e *Engine) estimate(p *Plan) {
	est := Estimate{CostKnown: true}
	price := func(model string, prompt, completion int) (float64, bool) {
		return e.Pricing.Cost(e.fullModel(model), llm.Usage{PromptTokens: prompt, CompletionTokens: completion})
	}
	for _, c := range p.Calls {
		est.Calls++
		est.PromptTokens += c.prompt
		est.MaxCompletionTokens += c.req.MaxTokens
		worst, ok := price(c.req.Model, c.prompt, c.req.MaxTokens)
		exp, _ := price(c.req.Model, c.prompt, expectedReviewerCompletion)
		c.worst, c.expected = worst, exp
		est.CostUSD += worst
		est.ExpectedCostUSD += exp
		est.CostKnown = est.CostKnown && ok
	}
	n := 0
	for _, up := range p.Units {
		if up.unit.Status == "" && len(up.calls) > 0 {
			n++
		}
	}
	est.Units = n
	if n > 0 {
		prompt := adjudicatorBase + e.adjudicatorFixedTokens()
		worst, ok := price(e.Opts.Models.Adjudicator, prompt*2, e.Opts.maxCompletion())
		exp, _ := price(e.Opts.Models.Adjudicator, prompt, expectedAdjudicatorCompletion)
		est.Calls += n
		est.PromptTokens += prompt * n
		est.MaxCompletionTokens += e.Opts.maxCompletion() * n
		est.CostUSD += worst * float64(n)
		est.ExpectedCostUSD += exp * float64(n) * adjudicatorShare
		est.CostKnown = est.CostKnown && ok
	}
	p.Estimate = est
}

// adjudicatorFixedTokens is what every adjudicator call carries: its system prompt and its schema.
func (e *Engine) adjudicatorFixedTokens() int {
	return llm.EstimatePromptTokens(llm.ChatRequest{
		Messages:       []llm.Message{{Role: llm.RoleSystem, Content: e.Opts.Prompts.AdjudicatorSystem("0")}},
		ResponseFormat: format("judge_adjudicator", e.adjSchema),
	})
}

// fullModel is the model as the price table knows it.
func (e *Engine) fullModel(m string) string { return m }

// admit applies the caps to the plan: calls are kept in priority order while the expected cost,
// tokens and call count stay inside them. The adjudicators' expected share is held back first.
func (e *Engine) admit(p *Plan) {
	caps := e.Opts.Caps
	if caps.MaxCostUSD <= 0 && caps.MaxTokens <= 0 && caps.MaxCalls <= 0 {
		return
	}
	adjCalls := int(float64(p.Estimate.Units)*adjudicatorShare + 0.5)
	adjCost := 0.0
	adjTokens := 0
	if p.Estimate.Units > 0 {
		perCost, _ := e.Pricing.Cost(e.Opts.Models.Adjudicator, llm.Usage{PromptTokens: adjudicatorBase + e.adjudicatorFixedTokens(), CompletionTokens: expectedAdjudicatorCompletion})
		adjCost = perCost * float64(adjCalls)
		adjTokens = (adjudicatorBase + e.adjudicatorFixedTokens() + expectedAdjudicatorCompletion) * adjCalls
	}
	var cost float64
	tok, calls := adjTokens, adjCalls
	cost = adjCost
	for _, c := range p.Calls {
		t := c.prompt + expectedReviewerCompletion
		switch {
		case caps.MaxCostUSD > 0 && cost+c.expected > caps.MaxCostUSD:
			c.skip = "the spend cap would be exceeded"
		case caps.MaxTokens > 0 && tok+t > caps.MaxTokens:
			c.skip = "the token cap would be exceeded"
		case caps.MaxCalls > 0 && calls >= caps.MaxCalls:
			c.skip = "the call cap would be exceeded"
		}
		if c.skip != "" {
			continue
		}
		cost += c.expected
		tok += t
		calls++
	}
}

// Skipped counts the calls the caps left out of the plan.
func (p *Plan) Skipped() int {
	n := 0
	for _, c := range p.Calls {
		if c.skip != "" {
			n++
		}
	}
	return n
}

// stop records why no further call is made; the first reason wins.
func (e *Engine) stop(reason string) {
	e.stopped.CompareAndSwap(nil, reason)
}

func (e *Engine) stopReason() string {
	if v, ok := e.stopped.Load().(string); ok {
		return v
	}
	return ""
}

// callOutcome classifies a failed model call.
type callOutcome int

const (
	callOK callOutcome = iota
	callRefused
	callFailed
	callCapped
)

// chat makes one model call under the rate limit and records its usage. A refusal (the provider
// stopped for its content policy) comes back as callRefused: the run reports it, never retries it.
func (e *Engine) chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, callOutcome, error) {
	if reason := e.stopReason(); reason != "" {
		return llm.ChatResponse{}, callCapped, errors.New(reason)
	}
	if err := e.limiter.wait(ctx); err != nil {
		return llm.ChatResponse{}, callFailed, err
	}
	if e.jsonMode.Load() {
		req = jsonModeRequest(req)
	}
	req = e.screenRequest(req)
	resp, err := e.Client.Chat(ctx, req)
	if err != nil && rejectsStructuredOutput(err) && req.ResponseFormat != nil {
		// A gateway or model without json_schema support: ask for the schema in the prompt instead.
		e.jsonMode.Store(true)
		req = jsonModeRequest(req)
		resp, err = e.Client.Chat(ctx, req)
	}
	if err != nil {
		var le *llm.Error
		if errors.As(err, &le) {
			switch le.Kind {
			case llm.KindBudget:
				e.stop("spend cap reached: " + llm.RedactSecrets(err.Error()))
				return resp, callCapped, err
			case llm.KindAuth, llm.KindConfig, llm.KindNetworkDisabled:
				e.stop("the model could not be called: " + llm.RedactSecrets(err.Error()))
			}
		}
		return resp, callFailed, err
	}
	if resp.CostUSD == 0 && !resp.Cached {
		// Without a spend cap there is no accounting layer; price the usage here.
		resp.CostUSD, _ = e.Pricing.Cost(firstNonEmptyStr(req.Model, resp.Model), resp.Usage)
	}
	e.mu.Lock()
	e.usage.calls++
	if resp.Cached {
		e.usage.cacheHits++
	}
	e.usage.input += resp.Usage.PromptTokens
	e.usage.output += resp.Usage.CompletionTokens
	e.usage.cost += resp.CostUSD
	e.mu.Unlock()
	if refused(resp) {
		return resp, callRefused, errors.New("the provider refused the request (content filter)")
	}
	return resp, callOK, nil
}

// screenRequest covers inputs that bypass unit preparation: rubrics, prompt overrides,
// traps and retry feedback. It leaves the caller's message slice untouched.
func (e *Engine) screenRequest(req llm.ChatRequest) llm.ChatRequest {
	req.Messages = append([]llm.Message(nil), req.Messages...)
	for i := range req.Messages {
		req.Messages[i].Content = llm.RedactForReview(maskSecretValues(req.Messages[i].Content, e.Opts.Attach.Secrets))
	}
	return req
}

// stoppedByFilter reports whether the provider stopped the reply for its content policy. On Bedrock
// a reply that is whole and valid can still carry this stop reason, so it alone is not a refusal.
func stoppedByFilter(r llm.ChatResponse) bool {
	switch strings.ToLower(r.FinishReason) {
	case "content_filter", "refusal", "safety":
		return true
	}
	return false
}

// refused reports whether the provider declined to answer: stopped by its filter with nothing to read.
func refused(r llm.ChatResponse) bool {
	return stoppedByFilter(r) && strings.TrimSpace(r.Text) == ""
}

// mapParallel runs fn for every index with at most n goroutines, handing the indexes out in order.
func mapParallel(ctx context.Context, n, count int, fn func(i int)) {
	if count == 0 {
		return
	}
	idx := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < min(n, count); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				if ctx.Err() != nil {
					continue
				}
				fn(i)
			}
		}()
	}
	for i := 0; i < count; i++ {
		idx <- i
	}
	close(idx)
	wg.Wait()
}

// sortFindings orders findings deterministically: category priority, severity, path, line, code.
func sortFindings(fs []*Finding) {
	prio := map[Category]int{CategorySecurity: 0, CategoryGovernance: 1, CategoryQuality: 2}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		switch {
		case prio[a.Category] != prio[b.Category]:
			return prio[a.Category] < prio[b.Category]
		case a.Severity.Rank() != b.Severity.Rank():
			return a.Severity.Rank() > b.Severity.Rank()
		case a.Location.Path != b.Location.Path:
			return a.Location.Path < b.Location.Path
		case a.Location.StartLine != b.Location.StartLine:
			return a.Location.StartLine < b.Location.StartLine
		}
		return a.Code < b.Code
	})
}

// rejectsStructuredOutput reports whether err is a provider's HTTP 400 about the response format.
func rejectsStructuredOutput(err error) bool {
	var le *llm.Error
	if !errors.As(err, &le) || le.Status != 400 {
		return false
	}
	msg := strings.ToLower(le.Message)
	return strings.Contains(msg, "response_format") || strings.Contains(msg, "json_schema") || strings.Contains(msg, "structured output")
}

// jsonModeRequest turns a structured-output request into one that carries its schema in the system
// prompt: the provider enforces nothing, and the reply is validated strictly in Go either way.
func jsonModeRequest(req llm.ChatRequest) llm.ChatRequest {
	if req.ResponseFormat == nil {
		return req
	}
	out := req
	out.Messages = append([]llm.Message(nil), req.Messages...)
	out.Messages[0].Content += "\n\nOUTPUT FORMAT: reply with one JSON object and nothing else (no code fence, no prose), matching this JSON Schema exactly:\n" + string(jsonText(req.ResponseFormat.Schema))
	out.ResponseFormat = nil
	return out
}
