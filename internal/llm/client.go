package llm

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lit "github.com/xberg-io/liter-llm/packages/go/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// literLLM is the only provider backend: it adapts a liter-llm DefaultClient to
// Client. liter-llm owns provider routing and wire formats, retries with
// jittered backoff and Retry-After, the typed error taxonomy and the catalog
// prices. This file keeps the ai-rulez policy around it: which provider a model
// may route to (and with it which key is sent), what counts as usage, and the
// request and reply bounds.
type literLLM struct {
	mu         sync.RWMutex // calls hold RLock; Close waits for in-flight calls (cancel their context to end them sooner)
	client     *lit.DefaultClient
	key        string // scrubbed from every error message
	provider   string // the provider prefix every request model must route to
	model      string
	embedModel string
	pricing    Pricing
	// perInput is set once a batch embed came back with the wrong number of vectors (Gemini's
	// native route answers a batch with one). Later batches then go out one input per request
	// instead of repeating the wasted batch call.
	perInput atomic.Bool
}

var errClosed = permanentError("liter-llm client is closed")

func newLiterLLM(cfg Config, getenv func(string) string) (*literLLM, error) {
	// liter-llm reads the routed provider's own key variable (OPENAI_API_KEY, ...) when no key is
	// given, so a repository route picks which of the user's keys is sent and billed even without
	// api_key_env: refuse it either way.
	if routed := cfg.RoutingFromRepo(); len(routed) > 0 {
		source := "the provider's own environment variable"
		if cfg.APIKeyEnv != "" {
			source = cfg.APIKeyEnv
		}
		return nil, newError(KindConfig, "%s llm-config-invalid: refusing to send the key from %s to a provider chosen by the repository config (%s); set provider and model in the user config file or AI_RULEZ_LLM_PROVIDER / AI_RULEZ_LLM_MODEL", CodeConfigInvalid, source, strings.Join(routed, ", "))
	}
	key := ""
	if cfg.APIKeyEnv != "" {
		if key = getenv(cfg.APIKeyEnv); key == "" {
			return nil, newError(KindAuth, "environment variable %s (api_key_env) is empty or unset", cfg.APIKeyEnv)
		}
	}
	var base, hint *string
	if cfg.BaseURL != "" {
		base = &cfg.BaseURL
	}
	if m := cfg.FullModel(); m != "" {
		hint = &m // with a base_url, liter-llm strips that provider's prefix from the model it sends
	}
	var timeout *uint64
	if secs := uint64(cfg.Timeout().Seconds()); secs > 0 {
		timeout = &secs
	}
	retries := uint32(min(max(cfg.Retries(), 0), MaxRetriesLimit)) //nolint:gosec // bounded to [0, MaxRetriesLimit]
	c, err := lit.CreateClient(key, base, timeout, &retries, hint)
	if err != nil {
		return nil, &Error{Kind: KindProvider, Message: "cannot create liter-llm client: " + RedactSecrets(err.Error())}
	}
	return &literLLM{client: c, key: key, provider: routeProvider(cfg), model: cfg.FullModel(), embedModel: embedFull(cfg), pricing: NewPricing(cfg)}, nil
}

// routeProvider is the provider the configured models route to: the provider
// field, else the prefix of the chat model, else of the embedding model.
func routeProvider(cfg Config) string {
	if cfg.Provider != "" {
		return cfg.Provider
	}
	for _, m := range []string{cfg.Model, cfg.EmbeddingModel} {
		if strings.Contains(m, "/") {
			return modelPrefix(m)
		}
	}
	return ""
}

// requestModel resolves a per-request model override. liter-llm routes on the
// provider/ prefix, and with it picks the key, so an override that names another
// provider could send the configured credential somewhere the user never chose
// (a repository config sets verifier and embedding models). A bare override is
// pinned to the configured provider; a prefixed one must already name it.
func (l *literLLM) requestModel(override, configured string) (string, error) {
	if override == "" || override == configured {
		return configured, nil
	}
	if !strings.Contains(override, "/") {
		if l.provider == "" {
			return override, nil
		}
		return l.provider + "/" + override, nil
	}
	if modelPrefix(override) != l.provider {
		return "", newError(KindConfig, "%s llm-config-invalid: refusing request model %q: its provider prefix is not the configured provider %q, so the configured key would be sent to another service; set provider and model in the user config instead", CodeConfigInvalid, override, l.provider)
	}
	return override, nil
}

func embedFull(cfg Config) string {
	if cfg.Provider != "" && cfg.EmbeddingModel != "" && !strings.Contains(cfg.EmbeddingModel, "/") {
		return cfg.Provider + "/" + cfg.EmbeddingModel
	}
	return cfg.EmbeddingModel
}

// classify maps liter-llm's typed error onto the package's errors by variant
// sentinel, never by message text. A failure that is not a liter-llm error (a
// request that could not be built, a closed client) is a permanent provider
// error: never retried, and an error is never cached. liter-llm has already
// retried what it considers transient, so Transient only tells a caller
// whether a later attempt may succeed.
func (l *literLLM) classify(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var le *lit.Error
	if !errors.As(err, &le) {
		return &Error{Kind: KindProvider, Message: l.scrub(err.Error()), permanent: true}
	}
	e := &Error{Kind: KindProvider, Message: l.scrub(le.Message), Status: int(le.StatusCode)}
	if le.RetryAfter != nil {
		e.RetryAfter = time.Duration(min(*le.RetryAfter, maxRetryAfterMillis)) * time.Millisecond //nolint:gosec // capped to an hour
	}
	switch {
	case errors.Is(err, lit.ErrAuthentication):
		e.Kind = KindAuth
	case errors.Is(err, lit.ErrRateLimited):
		e.Kind = KindRateLimit
		// TODO(liter-llm#257): an exhausted quota is reported as a transient rate limit, but no retry fixes it.
		e.permanent = quotaExhausted(le.Message)
	case errors.Is(err, lit.ErrContextWindowExceeded):
		e.Kind = KindContextLength
	case errors.Is(err, lit.ErrBudgetExceeded):
		e.Kind = KindBudget
	case errors.Is(err, lit.ErrTimeout):
		e.Kind = KindTimeout
	case errors.Is(err, lit.ErrBadRequest) && strings.Contains(strings.ToLower(le.Message), "prompt is too long"):
		// TODO(liter-llm#257): Anthropic reports an over-long prompt as a plain bad request.
		e.Kind = KindContextLength
	default:
		// BadRequest, NotFound, ContentPolicy, Serialization, ServerError, Network (empty Code, see
		// liter-llm#258), ...: only what liter-llm itself marks transient is worth another attempt.
		e.permanent = !le.IsTransient
	}
	return e
}

func quotaExhausted(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "insufficient_quota") || strings.Contains(m, "exceeded your current quota")
}

var urlQueryRe = regexp.MustCompile(`(https?://[^\s?"']+)\?[^\s"']*`)

// scrub removes what must not reach a message: the configured key by value, anything
// key-shaped, and the query string of any URL.
// TODO(liter-llm#259): liter-llm echoes a provider-returned key and the base_url query in errors.
func (l *literLLM) scrub(msg string) string {
	if l.key != "" {
		msg = strings.ReplaceAll(msg, l.key, "[redacted]")
	}
	return RedactSecrets(urlQueryRe.ReplaceAllString(msg, "$1"))
}

// chatRequest renders req for model as a liter-llm chat request.
func chatRequest(model string, req ChatRequest) lit.ChatCompletionRequest {
	out := lit.ChatCompletionRequest{Model: model, Temperature: &req.Temperature}
	switch {
	case req.MaxTokens <= 0:
	case needsMaxCompletionTokens(model):
		out.MaxCompletionTokens = lit.Ptr(uint64(req.MaxTokens))
	default:
		out.MaxTokens = lit.Ptr(uint64(req.MaxTokens))
	}
	for _, m := range req.Messages {
		text := []byte(mustJSON(m.Content))
		content := lit.UserContent(text)
		switch m.Role {
		case RoleSystem:
			out.Messages = append(out.Messages, lit.Message{Role: "system", System: &lit.SystemMessage{Content: content}})
		case RoleAssistant:
			out.Messages = append(out.Messages, lit.Message{Role: "assistant", Assistant: &lit.AssistantMessage{Content: lit.Ptr(lit.AssistantContent(text))}})
		default:
			out.Messages = append(out.Messages, lit.Message{Role: "user", User: &lit.UserMessage{Content: content}})
		}
	}
	if rf := req.ResponseFormat; rf != nil {
		out.ResponseFormat = lit.ResponseFormatJSONSchema{JSONSchema: lit.JSONSchemaFormat{Name: rf.Name, Schema: json.RawMessage(mustJSON(rf.Schema)), Strict: lit.Ptr(true)}}
	}
	return out
}

// needsMaxCompletionTokens reports whether model is an OpenAI reasoning model, which rejects max_tokens.
// TODO(liter-llm#264): liter-llm sends max_tokens verbatim to every OpenAI model.
func needsMaxCompletionTokens(model string) bool {
	if strings.Contains(model, "/") && modelPrefix(model) != "openai" {
		return false
	}
	name := bareModel(model)
	for _, family := range []string{"o1", "o3", "o4", "gpt-5"} {
		if name == family || strings.HasPrefix(name, family+"-") || strings.HasPrefix(name, family+".") {
			return true
		}
	}
	return false
}

// usageOf is the billed usage of a reply. Some providers leave thinking tokens out of
// completion_tokens but bill them at the output rate; they show only in total_tokens.
// OpenAI counts reasoning_tokens inside completion_tokens, another provider may report
// them beside it. The completion charged is therefore the largest of completion_tokens,
// total_tokens minus prompt_tokens, and reasoning_tokens on top of a smaller completion:
// never less than the provider's own total.
func usageOf(u *lit.Usage) Usage {
	if u == nil {
		return Usage{}
	}
	prompt, completion := int(min(u.PromptTokens, maxTokenCount)), int(min(u.CompletionTokens, maxTokenCount))
	if rest := int(min(u.TotalTokens, maxTokenCount)) - prompt; rest > completion {
		completion = rest
	}
	if d := u.CompletionTokensDetails; d != nil && int(min(d.ReasoningTokens, maxTokenCount)) > completion {
		completion += int(min(d.ReasoningTokens, maxTokenCount))
	}
	out := Usage{PromptTokens: prompt, CompletionTokens: completion}
	if d := u.PromptTokensDetails; d != nil {
		out.CachedTokens = min(int(min(d.CachedTokens, maxTokenCount)), prompt)
	}
	return out
}

// maxRetryAfterMillis caps a provider-requested delay at one hour.
const maxRetryAfterMillis = 3600 * 1000

// maxTokenCount bounds a reported count so a hostile or broken reply cannot overflow int arithmetic.
const maxTokenCount = 1 << 40

func (l *literLLM) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model, err := l.requestModel(req.Model, l.model)
	if err != nil {
		return ChatResponse{}, err
	}
	if model == "" {
		return ChatResponse{}, newError(KindConfig, "%s llm-config-invalid: no model configured; set [llm] model", CodeConfigInvalid)
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.client == nil {
		return ChatResponse{}, errClosed
	}
	resp, err := l.client.ChatWithContext(ctx, chatRequest(model, req))
	if err != nil {
		return ChatResponse{}, l.classify(err)
	}
	if resp == nil {
		return ChatResponse{}, permanentError("liter-llm returned an empty reply without an error")
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return ChatResponse{}, permanentError("response has no choices")
	}
	choice := resp.Choices[0]
	text := ""
	if t, terr := choice.Message.Text(); terr == nil && t != nil {
		text = *t
	}
	if len(text) > maxResponseBytes {
		return ChatResponse{}, &Error{Kind: KindProvider, Message: "response too large", permanent: true}
	}
	respModel := firstNonEmpty(resp.Model, model)
	usage := usageOf(resp.Usage)
	cost, known := l.pricing.Cost(respModel, usage)
	out := ChatResponse{Text: text, Model: respModel, Usage: usage, CostUSD: cost, CostKnown: known}
	if choice.FinishReason != nil {
		out.FinishReason = string(*choice.FinishReason)
	}
	return out, nil
}

// maxResponseBytes bounds the text of one chat reply. liter-llm cannot bound the HTTP body it reads
// (TODO(liter-llm#269): max_response_bytes is not reachable from the Go binding), so this
// is checked on the decoded text, after the body has been read.
const maxResponseBytes = 32 << 20

// errEmbedCount marks a reply with the wrong number of vectors.
var errEmbedCount = errors.New("embedding count mismatch")

func (l *literLLM) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model, err := l.requestModel(req.Model, l.embedModel)
	if err != nil {
		return EmbedResponse{}, err
	}
	if model == "" {
		return EmbedResponse{}, newError(KindConfig, "%s llm-config-invalid: no embedding model configured; set [llm] embedding_model", CodeConfigInvalid)
	}
	if len(req.Input) == 0 {
		return EmbedResponse{Model: model}, nil
	}
	if len(req.Input) > 1 && l.perInput.Load() {
		return l.embedEach(ctx, model, req, Usage{}, 0)
	}
	resp, raw, err := l.embedRaw(ctx, model, req.Input)
	if err != nil {
		return EmbedResponse{}, err
	}
	out, err := l.embedResult(resp, model, len(req.Input))
	if errors.Is(err, errEmbedCount) && len(req.Input) > 1 {
		l.perInput.Store(true)
		// The collapsed batch was still sent and billed: keep its usage and count it as a request.
		return l.embedEach(ctx, model, req, raw, 1)
	}
	return out, err
}

// embedRaw sends one embeddings request and returns the reply with the usage it reported.
func (l *literLLM) embedRaw(ctx context.Context, model string, input []string) (*lit.EmbeddingResponse, Usage, error) {
	in := []byte(mustJSON(input))
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.client == nil {
		return nil, Usage{}, errClosed
	}
	resp, err := l.client.EmbedWithContext(ctx, lit.EmbeddingRequest{Model: model, Input: lit.EmbeddingInput(in)})
	if err != nil {
		return nil, Usage{}, l.classify(err)
	}
	if resp == nil {
		return nil, Usage{}, permanentError("liter-llm returned an empty reply without an error")
	}
	var billed Usage
	if resp.Usage != nil {
		billed.PromptTokens = int(min(max(resp.Usage.PromptTokens, resp.Usage.TotalTokens), maxTokenCount))
	}
	return resp, billed, nil
}

func (l *literLLM) embedResult(resp *lit.EmbeddingResponse, model string, want int) (EmbedResponse, error) {
	if len(resp.Data) != want {
		e := permanentError("got %d embeddings for %d inputs", len(resp.Data), want)
		e.Cause = errEmbedCount
		return EmbedResponse{}, e
	}
	vecs := make([][]float32, want)
	for _, d := range resp.Data {
		if int(d.Index) >= want || vecs[d.Index] != nil {
			return EmbedResponse{}, permanentError("embedding index %d is out of range or repeated", d.Index)
		}
		vecs[d.Index] = d.Embedding
	}
	respModel := firstNonEmpty(resp.Model, model)
	var usage Usage
	if resp.Usage != nil {
		usage.PromptTokens = int(min(max(resp.Usage.PromptTokens, resp.Usage.TotalTokens), maxTokenCount))
	}
	cost, known := l.pricing.Cost(respModel, usage)
	return EmbedResponse{Vectors: vecs, Model: respModel, Usage: usage, CostUSD: cost, CostKnown: known}, nil
}

// embedEach embeds the inputs one request at a time. Gemini's native route behind
// liter-llm answers a batch with a single vector, so the batch is retried as
// singles rather than failing or returning too few vectors. spent and requests
// are what an already-sent batch cost, so the response charges and counts it.
//
// TODO(liter-llm#268): remove once a batch embedding returns one vector per input on every route.
//
// The caller's budget reservation covers the call's first request (the batch, or else the
// first single); every further request is admitted by the budget before it is sent and charged
// when it ends. On a failure the usage of a completed first request travels back in
// firstBilled, so the budget charges what was billed rather than one estimate for the batch.
func (l *literLLM) embedEach(ctx context.Context, model string, req EmbedRequest, spent Usage, requests int) (EmbedResponse, error) {
	cost, known := l.pricing.Cost(model, spent)
	out := EmbedResponse{Model: model, Usage: spent, CostUSD: cost, CostKnown: known || spent.Total() == 0, Requests: requests}
	sub := subBudgetFrom(ctx)
	var first *Usage
	if requests > 0 {
		first = &spent
	}
	failed := func(err error) (EmbedResponse, error) { return EmbedResponse{firstBilled: first}, err }
	for _, in := range req.Input {
		var res *reservation
		var err error
		if out.Requests > 0 {
			if res, err = sub.reserve(Usage{PromptTokens: tokens.Estimate(in)}); err != nil {
				return failed(err)
			}
		}
		raw, _, err := l.embedRaw(ctx, model, []string{in})
		var r EmbedResponse
		if err == nil {
			r, err = l.embedResult(raw, model, 1)
		}
		res.done(model, r.Usage, err)
		if err != nil {
			return failed(err)
		}
		if out.Requests == 0 {
			first = &r.Usage
		}
		out.Vectors = append(out.Vectors, r.Vectors[0])
		out.Usage.PromptTokens += r.Usage.PromptTokens
		out.CostUSD += r.CostUSD
		out.CostKnown = out.CostKnown && r.CostKnown
		out.Model = r.Model
		out.Requests++
	}
	return out, nil
}

// Close releases the native client once in-flight calls finish; it is idempotent.
func (l *literLLM) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.client != nil {
		l.client.Free()
		l.client = nil
	}
	return nil
}
