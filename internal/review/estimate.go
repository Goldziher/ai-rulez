package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// Defaults of the spend ceilings when neither a flag nor [review] sets them.
const (
	DefaultMaxCostUSD = 0.50
	DefaultMaxCalls   = 300
)

// expectedOutputTokens is the answer size the lower bound assumes (the design's
// own estimate); the upper bound uses the rubric's max_output_tokens.
const expectedOutputTokens = 400

// DefaultMaxOutputTokens is the judge's completion cap when a rubric leaves max_output_tokens
// unset: the built-in rubric's cap, which leaves a thinking model room to think before the
// verdict (400 came back truncated).
const DefaultMaxOutputTokens = 1500

// callOverheadTokens stands for per-request framing of the model's chat format.
const callOverheadTokens = 16

// nonceStub stands in for the per-request random nonce of the data fence, so the
// planned payload (and its hash) is reproducible. It is as long as a 128-bit
// nonce in hex (32 characters), so the planned byte counts are realistic.
const nonceStub = "<nonce:000000000000000000000000>"

// Prices answers what a call of the given size would cost on the configured model.
type Prices func(promptTokens, completionTokens int) (usd float64, known bool)

// EstimateInput configures Plan.
type EstimateInput struct {
	Rubric  *Rubric
	Results *Results
	// Content is descriptions or full.
	Content string
	// Model, Host and NetworkAllowed describe where a judge call would go.
	Model          string
	Host           string
	NetworkAllowed bool
	IgnoredLLMKeys []string
	// PolicyForbidsLLM is set when an organization policy forbids model calls: a real
	// run is refused whatever the estimate says.
	PolicyForbidsLLM bool
	// Prices converts tokens to dollars; nil means no model is configured.
	Prices Prices
	// MaxCostUSD and MaxCalls are the ceilings (already defaulted).
	MaxCostUSD float64
	MaxCalls   int
	// ShowPrompt includes the planned messages (redacted: withheld items are absent).
	ShowPrompt bool
}

// EgressCall is one planned model call.
type EgressCall struct {
	Group        string   `json:"group"`
	Dimensions   []string `json:"dimensions"`
	Siblings     []string `json:"siblings,omitempty"`
	Bytes        int      `json:"bytes"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens_max"`
}

// EgressItem is what one item would send: sizes and a hash, never content.
type EgressItem struct {
	ID        string       `json:"id"`
	Path      string       `json:"path"`
	Bytes     int          `json:"bytes"`
	SHA256    string       `json:"sha256"`
	Truncated bool         `json:"truncated,omitempty"`
	Calls     []EgressCall `json:"calls"`
}

// PromptView is the exact planned messages of one call (--show-prompt).
type PromptView struct {
	Item   string `json:"item"`
	Group  string `json:"group"`
	System string `json:"system"`
	User   string `json:"user"`
}

// Estimate is the egress manifest and cost range of a semantic run.
type Estimate struct {
	// Sent is always false in phase 0: no model is called.
	Sent           bool   `json:"sent"`
	ContentMode    string `json:"content_mode"`
	Model          string `json:"model,omitempty"`
	Host           string `json:"host"`
	NetworkAllowed bool   `json:"network_allowed"`
	// IgnoredLLMKeys are repository [llm] keys the trust rule ignored (user scope only).
	IgnoredLLMKeys []string `json:"ignored_llm_keys,omitempty"`
	// Items lists every item that would send something; withheld items are in Withheld.
	Items    []EgressItem `json:"items"`
	Withheld []string     `json:"withheld,omitempty"`
	Totals   Totals       `json:"totals"`
	// Cost range: the minimum assumes one vote per call and a typical answer, the
	// maximum assumes every call is re-voted rubric.votes.max times at full output.
	CostMinUSD *float64 `json:"cost_min_usd,omitempty"`
	CostMaxUSD *float64 `json:"cost_max_usd,omitempty"`
	CostKnown  bool     `json:"cost_known"`
	Cap        Cap      `json:"cap"`
	// Refused lists why a real run would be refused before its first call.
	Refused []string     `json:"refused,omitempty"`
	Prompts []PromptView `json:"prompts,omitempty"`
}

// Totals sums the planned calls.
type Totals struct {
	Items           int `json:"items"`
	CallsMin        int `json:"calls_min"`
	CallsMax        int `json:"calls_max"`
	InputTokensMin  int `json:"input_tokens_min"`
	InputTokensMax  int `json:"input_tokens_max"`
	OutputTokensMin int `json:"output_tokens_min"`
	OutputTokensMax int `json:"output_tokens_max"`
	Bytes           int `json:"bytes"`
}

// Cap is the spend ceiling a run would be held to.
type Cap struct {
	MaxCostUSD float64 `json:"max_cost_usd"`
	MaxCalls   int     `json:"max_calls"`
}

// Plan builds the egress manifest. It reads nothing and calls nothing.
func Plan(in EstimateInput) *Estimate {
	rb := in.Rubric
	est := &Estimate{
		ContentMode: in.Content, Model: in.Model, Host: in.Host, NetworkAllowed: in.NetworkAllowed, IgnoredLLMKeys: in.IgnoredLLMKeys,
		Cap:   Cap{MaxCostUSD: in.MaxCostUSD, MaxCalls: in.MaxCalls},
		Items: []EgressItem{},
	}
	votes := max(rb.Votes.Max, 1)
	system := systemPrompt(rb)
	var costMin, costMax float64
	costKnown := in.Prices != nil
	for i := range in.Results.Items {
		r := &in.Results.Items[i]
		if r.Status == StatusWithheld {
			est.Withheld = append(est.Withheld, r.ID)
			continue
		}
		if r.Status != StatusScored {
			continue
		}
		item, prompts := planItem(in, r, system)
		if len(item.Calls) == 0 {
			continue
		}
		est.Items = append(est.Items, item)
		est.Prompts = append(est.Prompts, prompts...)
		for _, c := range item.Calls {
			est.Totals.CallsMin++
			est.Totals.CallsMax += votes
			est.Totals.InputTokensMin += c.InputTokens
			est.Totals.InputTokensMax += c.InputTokens * votes
			outMin := min(expectedOutputTokens, c.OutputTokens)
			est.Totals.OutputTokensMin += outMin
			est.Totals.OutputTokensMax += c.OutputTokens * votes
			if in.Prices != nil {
				lo, ok1 := in.Prices(c.InputTokens, outMin)
				hi, ok2 := in.Prices(c.InputTokens*votes, c.OutputTokens*votes)
				costMin += lo
				costMax += hi
				costKnown = costKnown && ok1 && ok2
			}
		}
		est.Totals.Items++
		est.Totals.Bytes += item.Bytes
	}
	est.Withheld = dedupSorted(est.Withheld)
	if !in.ShowPrompt {
		est.Prompts = nil
	}
	est.CostKnown = costKnown && est.Totals.CallsMin > 0
	if est.CostKnown {
		lo, hi := roundUSD(costMin), roundUSD(costMax)
		est.CostMinUSD, est.CostMaxUSD = &lo, &hi
	}
	est.Refused = refusals(in, est)
	return est
}

func refusals(in EstimateInput, est *Estimate) []string {
	var out []string
	if est.Totals.CallsMin == 0 {
		return nil
	}
	if in.PolicyForbidsLLM {
		out = append(out, "the organization policy forbids model calls ([llm] allow_network = false): a real run is refused")
	}
	if in.Model != "" && !est.CostKnown && in.MaxCostUSD > 0 {
		out = append(out, "no price for model "+in.Model+" and a cost cap is set: set [llm] price_input_per_mtok and price_output_per_mtok, or --max-cost 0")
	}
	if est.CostKnown && in.MaxCostUSD > 0 && *est.CostMinUSD > in.MaxCostUSD {
		out = append(out, fmt.Sprintf("estimated cost $%.4f exceeds the cap $%g even with one vote per call", *est.CostMinUSD, in.MaxCostUSD))
	}
	if in.MaxCalls > 0 && est.Totals.CallsMin > in.MaxCalls {
		out = append(out, fmt.Sprintf("%d calls exceed the cap of %d even with one vote per call", est.Totals.CallsMin, in.MaxCalls))
	}
	return out
}

func roundUSD(v float64) float64 { return float64(int64(v*1e6+0.5)) / 1e6 }

func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// planItem plans the calls of one scored item: at most one intrinsic and one
// contextual call, each only when a dimension of the group still needs a judge.
func planItem(in EstimateInput, r *ItemResult, system string) (EgressItem, []PromptView) {
	rb := in.Rubric
	item := EgressItem{ID: r.ID, Path: r.Path}
	var prompts []PromptView
	hash := sha256.New()
	sent := false
	for _, group := range []string{GroupIntrinsic, GroupContextual} {
		dims := judgeDimensions(rb, r, group, in.Content)
		if len(dims) == 0 {
			continue
		}
		var sibs []Item
		if group == GroupContextual {
			sibs = shortlist(in.Results.pool, r.Item, rb.Limits.MaxSiblings)
			if len(sibs) == 0 {
				continue
			}
		}
		built := buildCall(callSpec{rb: rb, system: system, item: sendView(r.Item, r.Redacted), group: group, dims: dims, sibs: sibs, content: in.Content, vote: 1, stub: true})
		call := EgressCall{
			Group: group, Dimensions: dimIDs(dims), Bytes: len(built.User),
			InputTokens:  tokens.Estimate(system) + tokens.Estimate(built.User) + callOverheadTokens,
			OutputTokens: rb.Limits.MaxOutputTokens,
		}
		if call.OutputTokens == 0 {
			call.OutputTokens = DefaultMaxOutputTokens
		}
		for i := range sibs {
			call.Siblings = append(call.Siblings, sibs[i].ID)
		}
		item.Calls = append(item.Calls, call)
		item.Bytes += call.Bytes
		item.Truncated = item.Truncated || built.Truncated
		hash.Write([]byte(built.Data)) // every call's payload is part of the digest
		sent = true
		prompts = append(prompts, PromptView{Item: r.ID, Group: group, System: system, User: built.User})
	}
	if sent {
		item.SHA256 = hex.EncodeToString(hash.Sum(nil))
	}
	return item, prompts
}

// judgeDimensions lists the dimensions of group that a judge would still be
// asked about: not preempted by a twin error, and answerable from what is sent
// (a dimension that needs the body is left out unless the body is sent).
func judgeDimensions(rb *Rubric, r *ItemResult, group, content string) []Dimension {
	pre := map[string]bool{}
	for i := range r.Dimensions {
		if d := &r.Dimensions[i]; d.Preempted {
			pre[d.ID] = true
		}
	}
	var out []Dimension
	for i := range rb.Dimensions {
		d := &rb.Dimensions[i]
		if d.Group == group && !pre[d.ID] && (!d.NeedsBody || content == config.ReviewContentFull) {
			out = append(out, *d)
		}
	}
	return out
}

func dimIDs(dims []Dimension) []string {
	out := make([]string, len(dims))
	for i := range dims {
		out[i] = dims[i].ID
	}
	return out
}

// shortlist picks the n pool items of the same kind whose descriptions are most
// similar to it (word-set Jaccard, the AR702 measure), ties by id.
func shortlist(pool []Item, it Item, n int) []Item {
	if n <= 0 {
		return nil
	}
	own := wordSet(it.Description)
	type scored struct {
		item  Item
		score float64
	}
	var cand []scored
	for i := range pool {
		p := &pool[i]
		if p.ID == it.ID || p.Kind != it.Kind {
			continue
		}
		cand = append(cand, scored{*p, jaccard(own, wordSet(p.Description))})
	}
	sort.SliceStable(cand, func(i, j int) bool {
		if cand[i].score != cand[j].score {
			return cand[i].score > cand[j].score
		}
		return cand[i].item.ID < cand[j].item.ID
	})
	if len(cand) > n {
		cand = cand[:n]
	}
	out := make([]Item, len(cand))
	for i := range cand {
		out[i] = cand[i].item
	}
	return out
}

// truncateBody keeps the head and tail of a body longer than maxTokens. The cuts
// land on rune boundaries and invalid bytes become U+FFFD, in one linear pass.
func truncateBody(body string, maxTokens int) (string, bool) {
	limit := maxTokens * tokens.BudgetBytesPerToken
	if maxTokens <= 0 || len(body) <= limit {
		return body, false
	}
	half := limit / 2
	headEnd := half
	for headEnd > 0 && !utf8.RuneStart(body[headEnd]) {
		headEnd--
	}
	tailStart := len(body) - half
	for tailStart < len(body) && !utf8.RuneStart(body[tailStart]) {
		tailStart++
	}
	head := strings.ToValidUTF8(body[:headEnd], "\ufffd")
	tail := strings.ToValidUTF8(body[tailStart:], "\ufffd")
	return head + "\n[... truncated ...]\n" + tail, true
}
