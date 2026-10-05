package llm

import (
	"context"
	"errors"
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
func classifyNative(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	msg := RedactSecrets(err.Error())
	e := &Error{Kind: KindProvider, Message: msg}
	switch l := strings.ToLower(msg); {
	case strings.Contains(l, "authentication") || strings.Contains(l, "unauthorized") || strings.Contains(l, "401") || strings.Contains(l, "403"):
		e.Kind = KindAuth
	case strings.Contains(l, "rate limit") || strings.Contains(l, "ratelimit") || strings.Contains(l, "429"):
		e.Kind = KindRateLimit
	case strings.Contains(l, "context window") || strings.Contains(l, "context length") || strings.Contains(l, "context_length"):
		e.Kind = KindContextLength
	case strings.Contains(l, "budget"):
		e.Kind = KindBudget
	}
	return e
}

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
	return decodeEmbed(out, l.pricing, model, len(req.Input))
}

func (l *literLLM) Close() error {
	l.native.Free()
	return nil
}
