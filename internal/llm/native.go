package llm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
)

// NativeClient is the byte-level surface of the liter-llm binding as this
// package uses it. The binding's own types stay outside this module (see
// internal/llm/literllm); only OpenAI-shaped JSON crosses the boundary. The
// methods are blocking and cannot be canceled by the binding, so the adapter
// abandons a call when its context ends.
type NativeClient interface {
	ChatJSON(req []byte) ([]byte, error)
	EmbedJSON(req []byte) ([]byte, error)
	Free()
}

// NativeConfig is what a native factory needs to create a client.
type NativeConfig struct {
	APIKey         string
	BaseURL        string // empty: provider routing decides
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
	n, err := f(NativeConfig{APIKey: key, BaseURL: cfg.BaseURL, TimeoutSeconds: int(cfg.Timeout().Seconds()), MaxRetries: 0})
	if err != nil {
		return nil, &Error{Kind: KindProvider, Message: "cannot create liter-llm client: " + RedactSecrets(err.Error())}
	}
	return &literLLM{native: n, model: cfg.FullModel(), embedModel: embedFull(cfg), pricing: NewPricing(cfg)}, nil
}

func embedFull(cfg Config) string {
	if cfg.Provider != "" && cfg.EmbeddingModel != "" && !strings.Contains(cfg.EmbeddingModel, "/") {
		return cfg.Provider + "/" + cfg.EmbeddingModel
	}
	return cfg.EmbeddingModel
}

// run executes a blocking native call, returning early when ctx ends. The
// abandoned call finishes in the background; its result is dropped.
func run(ctx context.Context, call func() ([]byte, error)) ([]byte, error) {
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		b, err := call()
		ch <- result{b, err}
	}()
	select {
	case r := <-ch:
		return r.b, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// classifyNative maps the binding's flat "[code] message" errors onto typed errors.
// This is the one place the message text is interpreted (upstream liter-llm #244:
// the binding flattens its typed errors, so sentinels never match). A message it
// does not recognise maps to a permanent provider error: never retried, and an
// error is never cached. The corpus in native_classify_test.go pins the strings
// this relies on, so a reworded upstream message fails CI instead of silently
// changing retry, budget or gate behaviour.
func classifyNative(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	msg := RedactSecrets(err.Error())
	e := &Error{Kind: KindProvider, Message: msg, permanent: true}
	switch l := strings.ToLower(msg); {
	case strings.Contains(l, "authentication") || strings.Contains(l, "unauthorized") || nativeStatusRe(l, "401", "403"):
		e.Kind, e.permanent = KindAuth, false
	case strings.Contains(l, "rate limit") || strings.Contains(l, "ratelimit") || nativeStatusRe(l, "429"):
		e.Kind, e.permanent = KindRateLimit, false
	case strings.Contains(l, "context window") || strings.Contains(l, "context length") || strings.Contains(l, "context_length"):
		e.Kind, e.permanent = KindContextLength, false
	case strings.Contains(l, "budget"):
		e.Kind, e.permanent = KindBudget, false
	default:
		// A server-side failure the binding reports by HTTP status is worth a retry.
		for _, code := range []int{500, 502, 503, 504} {
			if nativeStatusRe(l, strconv.Itoa(code)) {
				e.Status, e.permanent = code, false
				break
			}
		}
	}
	return e
}

// errEmptyNative is returned when the binding reports neither a result nor an error
// (upstream liter-llm #246: a serialisation failure surfaces as (nil, nil)).
var errEmptyNative = permanentError("liter-llm returned an empty reply without an error")

func (l *literLLM) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := firstNonEmpty(req.Model, l.model)
	if model == "" {
		return ChatResponse{}, newError(KindConfig, "%s llm-config-invalid: no model configured; set [llm] model", CodeConfigInvalid)
	}
	body, err := encodeChat(model, req)
	if err != nil {
		return ChatResponse{}, newError(KindConfig, "cannot encode request: %v", err)
	}
	out, err := run(ctx, func() ([]byte, error) { return l.native.ChatJSON(body) })
	if err != nil {
		return ChatResponse{}, classifyNative(err)
	}
	if len(out) == 0 {
		return ChatResponse{}, errEmptyNative
	}
	return decodeChat(out, l.pricing, model)
}

func (l *literLLM) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model := firstNonEmpty(req.Model, l.embedModel)
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
	out, err := run(ctx, func() ([]byte, error) { return l.native.EmbedJSON(body) })
	if err != nil {
		return EmbedResponse{}, classifyNative(err)
	}
	if len(out) == 0 {
		return EmbedResponse{}, errEmptyNative
	}
	return decodeEmbed(out, l.pricing, model, len(req.Input))
}

func (l *literLLM) Close() error {
	l.native.Free()
	return nil
}

// nativeStatusRe reports whether msg carries one of the HTTP status codes as a
// whole number (not inside a longer digit run such as a request id).
func nativeStatusRe(msg string, codes ...string) bool {
	for _, c := range codes {
		for from := 0; ; {
			i := strings.Index(msg[from:], c)
			if i < 0 {
				break
			}
			i += from
			end := i + len(c)
			before := i == 0 || !isDigit(msg[i-1])
			after := end >= len(msg) || !isDigit(msg[end])
			if before && after {
				return true
			}
			from = end
		}
	}
	return false
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
