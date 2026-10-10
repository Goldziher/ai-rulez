package llm

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	lit "github.com/xberg-io/liter-llm/packages/go/v2"
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
	maxBytes := uint64(maxResponseBytes)
	c, err := lit.CreateClientWithOptions(lit.ClientOptions{
		APIKey:           key,
		BaseURL:          base,
		TimeoutSecs:      timeout,
		MaxRetries:       &retries,
		MaxResponseBytes: &maxBytes,
		ModelHint:        hint,
	})
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
	case errors.Is(err, lit.ErrProviderQuotaExceeded):
		// An exhausted quota (liter-llm's own ProviderQuotaExceeded variant, code 120) is a
		// billing condition, not a rate limit: no retry fixes it.
		e.Kind = KindRateLimit
		e.permanent = true
	case errors.Is(err, lit.ErrRateLimited):
		e.Kind = KindRateLimit
	case errors.Is(err, lit.ErrContextWindowExceeded):
		e.Kind = KindContextLength
	case errors.Is(err, lit.ErrBudgetExceeded):
		e.Kind = KindBudget
	case errors.Is(err, lit.ErrTimeout):
		e.Kind = KindTimeout
	default:
		// BadRequest, NotFound, ContentPolicy, Serialization, ServerError, Network, ...: only
		// what liter-llm itself marks transient is worth another attempt.
		e.permanent = !le.IsTransient
	}
	return e
}

var urlQueryRe = regexp.MustCompile(`(https?://[^\s?"']+)\?[^\s"']*`)

// scrub removes what must not reach a message: the configured key by value, anything
// key-shaped, and the query string of any URL. liter-llm redacts the configured secret
// from a provider error body and drops the URL from a network error, but a provider can
// still echo a bare key-shaped token or a URL with a query in an error body.
// TODO(liter-llm#259, still missing in v2.2.3): neither is scrubbed upstream.
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
// liter-llm 2.2.3 renames max_tokens to max_completion_tokens for its own openai and azure providers,
// but a base_url endpoint is served by its generic "custom" provider, which leaves the field alone.
// TODO(liter-llm#264, partial in v2.2.3): still needed for base_url reasoning models.
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
	respModel := firstNonEmpty(resp.Model, model)
	usage := usageOf(resp.Usage)
	cost, known := l.pricing.Cost(respModel, usage)
	out := ChatResponse{Text: text, Model: respModel, Usage: usage, CostUSD: cost, CostKnown: known}
	if choice.FinishReason != nil {
		out.FinishReason = string(*choice.FinishReason)
	}
	return out, nil
}

// maxResponseBytes bounds the HTTP body liter-llm reads for one reply. It is passed as the
// client's max_response_bytes, so an oversized body is refused while it is read.
const maxResponseBytes = 32 << 20

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
	resp, err := l.embedRaw(ctx, model, req.Input)
	if err != nil {
		return EmbedResponse{}, err
	}
	return l.embedResult(resp, model, len(req.Input))
}

// embedRaw sends one embeddings request. liter-llm 2.2.3 preserves every input and returns one
// vector per input on the Gemini and Vertex routes, so a single request always serves the batch.
func (l *literLLM) embedRaw(ctx context.Context, model string, input []string) (*lit.EmbeddingResponse, error) {
	in := []byte(mustJSON(input))
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.client == nil {
		return nil, errClosed
	}
	resp, err := l.client.EmbedWithContext(ctx, lit.EmbeddingRequest{Model: model, Input: lit.EmbeddingInput(in)})
	if err != nil {
		return nil, l.classify(err)
	}
	if resp == nil {
		return nil, permanentError("liter-llm returned an empty reply without an error")
	}
	return resp, nil
}

func (l *literLLM) embedResult(resp *lit.EmbeddingResponse, model string, want int) (EmbedResponse, error) {
	if len(resp.Data) != want {
		return EmbedResponse{}, permanentError("got %d embeddings for %d inputs", len(resp.Data), want)
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
