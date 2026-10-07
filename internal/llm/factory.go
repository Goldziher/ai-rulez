package llm

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// Options tune New and Wrap. The zero value is correct for production.
type Options struct {
	// ConfigDir is the ai-rulez config directory (for example /repo/.ai-rulez).
	// It names the project: the response cache lives in the user cache directory
	// (~/.cache/ai-rulez/llm/<hash of ConfigDir>), outside the repository. Empty disables the cache.
	ConfigDir string
	// CacheDir overrides the cache directory (tests, CI with a restored cache).
	CacheDir string
	// SecretPath overrides the per-user cache secret file (default
	// $XDG_CONFIG_HOME/ai-rulez/llm-cache.key or ~/.config/ai-rulez/llm-cache.key).
	SecretPath string
	// Getenv resolves api_key_env; nil means the environment of Env.
	Getenv func(string) string
	// Env is the environment and home directory the cache and secret locations
	// and the default Getenv read; nil is the real process.
	Env ambient.Env
	// HTTPClient is used by the openaicompat backend; nil means a default client.
	HTTPClient *http.Client
	// Logger receives content-free debug summaries; nil disables logging.
	Logger *slog.Logger
	// Retry overrides the retry policy derived from the config (tests inject sleep and jitter).
	Retry *RetryPolicy
	// DryRun, when set, makes New return a client that prints what would be sent
	// to this writer and calls nothing, even when allow_network is false.
	DryRun io.Writer
	// ShowContent prints message text in dry-run output; off by default.
	ShowContent bool
	// NoCache disables the cache regardless of config (the --no-cache flag).
	NoCache bool
}

// Managed is the Client New returns: the middleware stack plus accounting.
type Managed struct {
	Client
	budget *Budget
	cache  *Cache
	// Backend is the resolved backend name.
	Backend string
}

// Spent returns the totals charged against the budget so far.
func (m *Managed) Spent() Spent {
	if m.budget == nil {
		return Spent{}
	}
	return m.budget.Spent()
}

// Cache returns the response cache, or nil when disabled.
func (m *Managed) Cache() *Cache { return m.cache }

// ResolveBackend turns a configured backend ("", auto, openaicompat, literllm) into a concrete one.
func ResolveBackend(configured string) string {
	switch configured {
	case BackendOpenAICompat, BackendLiterLLM:
		return configured
	default:
		if NativeAvailable() {
			return BackendLiterLLM
		}
		return BackendOpenAICompat
	}
}

// New validates cfg, selects the backend and wraps it in the middleware stack:
//
//	gate (allow_network, timeout) -> cache -> retry -> budget -> backend
//
// A cache hit therefore costs no budget and no network, and every retry attempt
// is checked against the budget.
func New(cfg Config, opts Options) (*Managed, error) {
	if err := cfg.Err(); err != nil {
		return nil, err
	}
	getenv := opts.Getenv
	if getenv == nil {
		getenv = ambient.GetenvFunc(opts.Env)
	}
	backend := ResolveBackend(cfg.Backend)
	if opts.DryRun != nil {
		d := &DryRun{Out: opts.DryRun, Model: cfg.FullModel(), EmbedModel: cfg.EmbeddingModel, Pricing: NewPricing(cfg), ShowContent: opts.ShowContent}
		return &Managed{Client: d, Backend: backend}, nil
	}
	if !cfg.AllowNetwork {
		// Refuse before building anything: no key lookup, no client, no connection.
		return &Managed{Client: withGate(NewFake(), false, 0), Backend: backend}, nil
	}
	var inner Client
	switch backend {
	case BackendLiterLLM:
		l, err := newLiterLLM(cfg, getenv)
		if err != nil {
			return nil, err
		}
		inner = l
	default:
		o, err := newOpenAICompat(cfg, getenv, opts.HTTPClient)
		if err != nil {
			return nil, err
		}
		inner = o
	}
	m := Wrap(inner, cfg, opts)
	m.Backend = backend
	return m, nil
}

// Wrap puts the middleware stack around an arbitrary backend (tests pass a Fake).
func Wrap(backend Client, cfg Config, opts Options) *Managed {
	pricing := NewPricing(cfg)
	budget := NewBudget(Limits{MaxCostUSD: cfg.MaxCostUSD, MaxTokens: cfg.MaxTokens, MaxCalls: cfg.MaxCalls}, pricing)
	c := WithBudget(backend, budget, cfg.FullModel(), cfg.EmbeddingModel)
	policy := RetryPolicy{Retries: cfg.Retries()}
	if opts.Retry != nil {
		policy = *opts.Retry
	}
	c = WithRetry(c, policy)
	var cache *Cache
	if dir := CacheDirFor(opts); cfg.CacheEnabled() && !opts.NoCache && dir != "" {
		secret, err := loadOrCreateSecret(secretPathFor(opts))
		if err != nil {
			if opts.Logger != nil {
				opts.Logger.Warn("llm response cache disabled: no usable cache secret", "error", err.Error())
			}
		} else {
			cache = NewCache(dir, cacheIdentity(cfg), secret)
			injected := 0
			if budget.limits.Active() {
				injected = DefaultCompletionCap
			}
			c = withCapCache(c, cache, cfg.FullModel(), cfg.EmbeddingModel, injected)
		}
	}
	c = withGate(c, cfg.AllowNetwork, cfg.Timeout())
	c = WithLogging(c, opts.Logger)
	return &Managed{Client: c, budget: budget, cache: cache}
}

// cacheIdentity names the provider endpoint so two gateways never share answers.
// The identity also carries the resolved backend, the URL scheme and the NAME of
// the key variable (never its value), so two credentials or backends behind one
// gateway do not share answers.
func cacheIdentity(cfg Config) string {
	id := cfg.Provider
	if u, err := url.Parse(cfg.BaseURL); err == nil && u.Host != "" {
		id += "@" + u.Scheme + "://" + u.Host + u.Path
	}
	return id + "|backend=" + ResolveBackend(cfg.Backend) + "|key=" + cfg.APIKeyEnv + "|prices=" + NewPricing(cfg).identity()
}
