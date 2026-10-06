package llm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// NativeClient is the byte-level surface of the liter-llm binding as this
// package uses it. The binding's own types stay outside this module (see
// internal/llm/literllm); only OpenAI-shaped JSON crosses the boundary. Ending
// the context aborts the in-flight native request (liter-llm 2.1.3 and later).
type NativeClient interface {
	ChatJSON(ctx context.Context, req []byte) ([]byte, error)
	EmbedJSON(ctx context.Context, req []byte) ([]byte, error)
	Free()
}

// NativeError is the typed failure a NativeClient returns for a provider or
// transport error. NativeVariant is the liter-llm error variant name
// ("RateLimited", "Authentication", "Timeout", ...); it is the only thing the
// classifier reads, never message text.
type NativeError interface {
	error
	NativeVariant() string
	NativeStatus() int
	NativeTransient() bool
	NativeRetryAfter() time.Duration
}

// NativeConfig is what a native factory needs to create a client.
type NativeConfig struct {
	APIKey         string
	BaseURL        string // empty: provider routing decides
	ModelHint      string // the configured provider/model; lets the binding strip the provider prefix when BaseURL is set
	TimeoutSeconds int
	MaxRetries     int
}

// NativeFactory creates a NativeClient.
type NativeFactory func(NativeConfig) (NativeClient, error)

var (
	nativeMu      sync.RWMutex
	nativeFactory NativeFactory
)

// RegisterNative installs the liter-llm backend. Only the literllm build tag's
// file calls it; the default build has no native backend.
func RegisterNative(f NativeFactory) {
	nativeMu.Lock()
	nativeFactory = f
	nativeMu.Unlock()
}

// NativeAvailable reports whether a liter-llm backend is compiled in.
func NativeAvailable() bool {
	nativeMu.RLock()
	defer nativeMu.RUnlock()
	return nativeFactory != nil
}

// literLLM adapts a NativeClient to Client. Status: experimental.
type literLLM struct {
	native     NativeClient
	provider   string // the provider prefix every request model must route to
	model      string
	embedModel string
	pricing    Pricing
}

func newLiterLLM(cfg Config, getenv func(string) string) (*literLLM, error) {
	nativeMu.RLock()
	f := nativeFactory
	nativeMu.RUnlock()
	if f == nil {
		return nil, newError(KindConfig, "%s llm-config-invalid: backend %q is not compiled in; use a release built with -tags literllm or set backend = \"openaicompat\"", CodeConfigInvalid, BackendLiterLLM)
	}
	key := ""
	if cfg.APIKeyEnv != "" {
		if routed := cfg.RoutingFromRepo(); len(routed) > 0 {
			return nil, newError(KindConfig, "%s llm-config-invalid: refusing to send the key from %s to a provider chosen by the repository config (%s); set provider and model in the user config file or AI_RULEZ_LLM_PROVIDER / AI_RULEZ_LLM_MODEL", CodeConfigInvalid, cfg.APIKeyEnv, strings.Join(routed, ", "))
		}
		if key = getenv(cfg.APIKeyEnv); key == "" {
			return nil, newError(KindAuth, "environment variable %s (api_key_env) is empty or unset", cfg.APIKeyEnv)
		}
	}
	n, err := f(NativeConfig{APIKey: key, BaseURL: cfg.BaseURL, ModelHint: cfg.FullModel(), TimeoutSeconds: int(cfg.Timeout().Seconds()), MaxRetries: 0})
	if err != nil {
		return nil, &Error{Kind: KindProvider, Message: "cannot create liter-llm client: " + RedactSecrets(err.Error())}
	}
	return &literLLM{native: n, provider: routeProvider(cfg), model: cfg.FullModel(), embedModel: embedFull(cfg), pricing: NewPricing(cfg)}, nil
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

// classifyNative maps a typed liter-llm error onto the package's errors by its
// variant. A failure that is not a NativeError (a request that could not be
// built, a closed client) is a permanent provider error: never retried, and an
// error is never cached. The variant names are pinned by native_classify_test.go.
func classifyNative(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var ne NativeError
	if !errors.As(err, &ne) {
		return &Error{Kind: KindProvider, Message: RedactSecrets(err.Error()), permanent: true}
	}
	e := &Error{Kind: KindProvider, Message: RedactSecrets(ne.Error()), Status: ne.NativeStatus(), RetryAfter: ne.NativeRetryAfter()}
	switch ne.NativeVariant() {
	case "Authentication":
		e.Kind = KindAuth
	case "RateLimited":
		e.Kind = KindRateLimit
	case "ContextWindowExceeded":
		e.Kind = KindContextLength
	case "BudgetExceeded":
		e.Kind = KindBudget
	case "Timeout":
		e.Kind = KindTimeout
	case "ServerError", "ServiceUnavailable":
		e.permanent = !ne.NativeTransient() && e.Status == 0
	case "Network", "InternalError":
		// liter-llm reports a transport failure as Network, and as InternalError once the
		// error has been cloned across the binding; openaicompat retries the same failure.
		e.permanent = false
	default:
		// BadRequest, NotFound, ContentPolicy, Serialization, ...: a retry cannot fix them,
		// unless liter-llm itself marks the variant transient.
		e.permanent = !ne.NativeTransient()
	}
	return e
}

// errEmptyNative is returned when the binding reports neither a result nor an error
// (defensive; liter-llm 2.1.3 returns an error instead).
var errEmptyNative = permanentError("liter-llm returned an empty reply without an error")

func (l *literLLM) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model, err := l.requestModel(req.Model, l.model)
	if err != nil {
		return ChatResponse{}, err
	}
	if model == "" {
		return ChatResponse{}, newError(KindConfig, "%s llm-config-invalid: no model configured; set [llm] model", CodeConfigInvalid)
	}
	body, err := encodeChat(model, req)
	if err != nil {
		return ChatResponse{}, newError(KindConfig, "cannot encode request: %v", err)
	}
	out, err := l.native.ChatJSON(ctx, body)
	if err != nil {
		return ChatResponse{}, classifyNative(err)
	}
	if len(out) == 0 {
		return ChatResponse{}, errEmptyNative
	}
	return decodeChat(out, l.pricing, model)
}

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
	body, err := encodeEmbed(model, req)
	if err != nil {
		return EmbedResponse{}, newError(KindConfig, "cannot encode request: %v", err)
	}
	out, err := l.native.EmbedJSON(ctx, body)
	if err != nil {
		return EmbedResponse{}, classifyNative(err)
	}
	if len(out) == 0 {
		return EmbedResponse{}, errEmptyNative
	}
	resp, err := decodeEmbed(out, l.pricing, model, len(req.Input))
	if errors.Is(err, errEmbedCount) && len(req.Input) > 1 {
		return l.embedEach(ctx, model, req)
	}
	return resp, err
}

// embedEach embeds the inputs one call at a time. Gemini's native route behind
// liter-llm answers a batch with a single vector, so the batch is retried as
// singles rather than failing or returning too few vectors.
func (l *literLLM) embedEach(ctx context.Context, model string, req EmbedRequest) (EmbedResponse, error) {
	out := EmbedResponse{Model: model, CostKnown: true}
	for _, in := range req.Input {
		one := req
		one.Input = []string{in}
		body, err := encodeEmbed(model, one)
		if err != nil {
			return EmbedResponse{}, newError(KindConfig, "cannot encode request: %v", err)
		}
		raw, err := l.native.EmbedJSON(ctx, body)
		if err != nil {
			return EmbedResponse{}, classifyNative(err)
		}
		if len(raw) == 0 {
			return EmbedResponse{}, errEmptyNative
		}
		r, err := decodeEmbed(raw, l.pricing, model, 1)
		if err != nil {
			return EmbedResponse{}, err
		}
		out.Vectors = append(out.Vectors, r.Vectors[0])
		out.Usage.PromptTokens += r.Usage.PromptTokens
		out.CostUSD += r.CostUSD
		out.CostKnown = out.CostKnown && r.CostKnown
		out.Model = r.Model
	}
	return out, nil
}

func (l *literLLM) Close() error {
	l.native.Free()
	return nil
}
