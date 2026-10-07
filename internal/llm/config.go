package llm

import (
	"math"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// CodeConfigInvalid is the strict-validation code for a bad [llm] table.
const CodeConfigInvalid = "AR9L0"

// Backend names accepted in config.
const (
	BackendAuto         = "auto"
	BackendOpenAICompat = "openaicompat"
	BackendLiterLLM     = "literllm"
)

// Config is the [llm] table of config.toml. Nothing calls out unless
// AllowNetwork is true.
//
//nolint:tagliatelle // config keys are snake_case by project convention
type Config struct {
	// Provider is the provider prefix for `provider/model` routing (used by the literllm backend).
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" toml:"provider,omitempty"`
	// Model is the chat model, sent verbatim by openaicompat and as provider/model by literllm.
	Model string `yaml:"model,omitempty" json:"model,omitempty" toml:"model,omitempty"`
	// Backend is auto, openaicompat or literllm. Empty means auto.
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty" toml:"backend,omitempty"`
	// BaseURL is the API root, e.g. https://gateway.internal/v1.
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty" toml:"base_url,omitempty"`
	// APIKeyEnv names the environment variable holding the API key. Never the key itself.
	APIKeyEnv string `yaml:"api_key_env,omitempty" json:"api_key_env,omitempty" toml:"api_key_env,omitempty"`
	// EmbeddingModel is the model used by Embed.
	EmbeddingModel string `yaml:"embedding_model,omitempty" json:"embedding_model,omitempty" toml:"embedding_model,omitempty"`
	// MaxCostUSD stops the run once the estimated spend would exceed it (0 = unlimited).
	MaxCostUSD float64 `yaml:"max_cost_usd,omitempty" json:"max_cost_usd,omitempty" toml:"max_cost_usd,omitempty"`
	// MaxTokens stops the run once prompt plus completion tokens would exceed it (0 = unlimited).
	MaxTokens int `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty" toml:"max_tokens,omitempty"`
	// MaxCalls stops the run after this many provider calls (0 = unlimited).
	MaxCalls int `yaml:"max_calls,omitempty" json:"max_calls,omitempty" toml:"max_calls,omitempty"`
	// Cache enables the on-disk response cache. Nil means true.
	Cache *bool `yaml:"cache,omitempty" json:"cache,omitempty" toml:"cache,omitempty"`
	// AllowNetwork must be true for any provider call. Default false.
	AllowNetwork bool `yaml:"allow_network,omitempty" json:"allow_network,omitempty" toml:"allow_network,omitempty"`
	// TimeoutSeconds bounds one call including retries (default 60).
	TimeoutSeconds int `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty" toml:"timeout_seconds,omitempty"`
	// MaxRetries is the number of retries after the first attempt (default 3; -1 disables).
	MaxRetries int `yaml:"max_retries,omitempty" json:"max_retries,omitempty" toml:"max_retries,omitempty"`
	// AllowPlainHTTP, with PlainHTTPHosts, lets a key travel over plain http to the listed
	// non-loopback gateways (for example a service mesh that terminates TLS in a sidecar).
	// User scope only: a repository config cannot set it.
	AllowPlainHTTP bool `yaml:"allow_plain_http,omitempty" json:"allow_plain_http,omitempty" toml:"allow_plain_http,omitempty"`
	// PlainHTTPHosts is the allow-list for AllowPlainHTTP: host or host:port, matched against base_url's host.
	PlainHTTPHosts []string `yaml:"plain_http_hosts,omitempty" json:"plain_http_hosts,omitempty" toml:"plain_http_hosts,omitempty"`
	// PriceInputPerMTok and PriceOutputPerMTok override the built-in price table (USD per million tokens).
	PriceInputPerMTok  float64 `yaml:"price_input_per_mtok,omitempty" json:"price_input_per_mtok,omitempty" toml:"price_input_per_mtok,omitempty"`
	PriceOutputPerMTok float64 `yaml:"price_output_per_mtok,omitempty" json:"price_output_per_mtok,omitempty" toml:"price_output_per_mtok,omitempty"`

	// repoProvider and repoModelRoute record that provider routing (the provider
	// field, or a provider/ prefix in model or embedding_model) came from a repository config rather
	// than from user scope. Set only by Resolve; see RoutingFromRepo.
	repoProvider, repoModelRoute, repoEmbedRoute bool
}

// MaxRetriesLimit bounds max_retries so a typo cannot keep a run retrying for hours.
const MaxRetriesLimit = 10

// Defaults.
const (
	DefaultTimeoutSeconds = 60
	DefaultMaxRetries     = 3
	// DefaultCompletionCap bounds a completion when a budget is active and the request sets no cap.
	DefaultCompletionCap = 1024
)

// CacheEnabled reports whether the response cache is on (default true).
func (c Config) CacheEnabled() bool { return c.Cache == nil || *c.Cache }

// Timeout returns the per-call timeout.
func (c Config) Timeout() time.Duration {
	if c.TimeoutSeconds > 0 {
		return time.Duration(c.TimeoutSeconds) * time.Second
	}
	return DefaultTimeoutSeconds * time.Second
}

// Retries returns the retry count.
func (c Config) Retries() int {
	switch {
	case c.MaxRetries < 0:
		return 0
	case c.MaxRetries == 0:
		return DefaultMaxRetries
	default:
		return c.MaxRetries
	}
}

// FullModel returns the chat model with the provider prefix when it has none.
func (c Config) FullModel() string {
	if c.Provider != "" && c.Model != "" && !strings.Contains(c.Model, "/") {
		return c.Provider + "/" + c.Model
	}
	return c.Model
}

// WithEnv returns a copy with AI_RULEZ_LLM_* overrides applied. getenv may be nil (os.Getenv).
func (c Config) WithEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = ambient.GetenvFunc(nil)
	}
	str := func(name string, dst *string) {
		if v := strings.TrimSpace(getenv("AI_RULEZ_LLM_" + name)); v != "" {
			*dst = v
		}
	}
	str("PROVIDER", &c.Provider)
	str("MODEL", &c.Model)
	if strings.TrimSpace(getenv("AI_RULEZ_LLM_PROVIDER")) != "" {
		c.repoProvider = false
	}
	if strings.TrimSpace(getenv("AI_RULEZ_LLM_MODEL")) != "" {
		c.repoModelRoute = false
	}
	if strings.TrimSpace(getenv("AI_RULEZ_LLM_EMBEDDING_MODEL")) != "" {
		c.repoEmbedRoute = false
	}
	str("BACKEND", &c.Backend)
	str("BASE_URL", &c.BaseURL)
	str("API_KEY_ENV", &c.APIKeyEnv)
	str("EMBEDDING_MODEL", &c.EmbeddingModel)
	var errs []string
	flt := func(name string, dst *float64) {
		if v := strings.TrimSpace(getenv("AI_RULEZ_LLM_" + name)); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				errs = append(errs, "AI_RULEZ_LLM_"+name+" is not a number")
				return
			}
			*dst = f
		}
	}
	num := func(name string, dst *int) {
		if v := strings.TrimSpace(getenv("AI_RULEZ_LLM_" + name)); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				errs = append(errs, "AI_RULEZ_LLM_"+name+" is not an integer")
				return
			}
			*dst = n
		}
	}
	boolean := func(name string) (bool, bool) {
		v := strings.TrimSpace(getenv("AI_RULEZ_LLM_" + name))
		if v == "" {
			return false, false
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, "AI_RULEZ_LLM_"+name+" is not a boolean")
			return false, false
		}
		return b, true
	}
	flt("MAX_COST_USD", &c.MaxCostUSD)
	num("MAX_TOKENS", &c.MaxTokens)
	num("MAX_CALLS", &c.MaxCalls)
	num("TIMEOUT_SECONDS", &c.TimeoutSeconds)
	if b, ok := boolean("CACHE"); ok {
		c.Cache = &b
	}
	if b, ok := boolean("ALLOW_NETWORK"); ok {
		c.AllowNetwork = b
	}
	if b, ok := boolean("ALLOW_PLAIN_HTTP"); ok {
		c.AllowPlainHTTP = b
	}
	if v := strings.TrimSpace(getenv("AI_RULEZ_LLM_PLAIN_HTTP_HOSTS")); v != "" {
		c.PlainHTTPHosts = nil
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				c.PlainHTTPHosts = append(c.PlainHTTPHosts, h)
			}
		}
	}
	if len(errs) > 0 {
		return c, newError(KindConfig, "%s: %s", CodeConfigInvalid, strings.Join(errs, "; "))
	}
	return c, nil
}

var (
	envNameRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	keyPrefixRe = regexp.MustCompile(`^(sk|pk|rk|sess|xox[a-z]|gh[pousr]|glpat|AKIA|ASIA|AIza|hf|ya29|eyJ)[-_A-Za-z0-9]`)
)

// looksLikeSecret reports whether v is more plausibly a key than an environment variable name.
func looksLikeSecret(v string) bool {
	if keyPrefixRe.MatchString(v) && !strings.Contains(v, "_KEY") && !strings.Contains(v, "_TOKEN") {
		return true
	}
	// A long mixed-case alphanumeric string with digits and no separator is not a variable name.
	if len(v) >= 24 && !strings.Contains(v, "_") {
		hasLower, hasUpper, hasDigit := false, false, false
		for _, r := range v {
			switch {
			case r >= 'a' && r <= 'z':
				hasLower = true
			case r >= 'A' && r <= 'Z':
				hasUpper = true
			case r >= '0' && r <= '9':
				hasDigit = true
			}
		}
		return hasLower && hasUpper && hasDigit
	}
	return false
}

// Validate checks the config statically (no network, no environment reads) and
// returns one message per problem. Every message belongs to AR9L0.
func (c Config) Validate() []string {
	var out []string
	switch c.Backend {
	case "", BackendAuto, BackendOpenAICompat, BackendLiterLLM:
	default:
		out = append(out, "backend is not one of auto, openaicompat, literllm")
	}
	out = append(out, c.validateAPIKeyEnv()...)
	out = append(out, c.validateBaseURL()...)
	out = append(out, c.validatePlainHTTP()...)
	out = append(out, c.validateNumbers()...)
	if c.Model != "" && strings.ContainsAny(c.Model, " \t\n") {
		out = append(out, "model must not contain whitespace")
	}
	sort.Strings(out)
	return out
}

func (c Config) validateAPIKeyEnv() []string {
	switch {
	case c.APIKeyEnv == "":
		return nil
	case looksLikeSecret(c.APIKeyEnv):
		return []string{"api_key_env looks like a literal API key; it must be the NAME of an environment variable (for example OPENAI_API_KEY), never the key"}
	case !envNameRe.MatchString(c.APIKeyEnv):
		return []string{"api_key_env is not an environment variable name; it must be the NAME of an environment variable (for example OPENAI_API_KEY), never the key"}
	}
	return nil
}

func (c Config) validateBaseURL() []string {
	if c.BaseURL == "" {
		return nil
	}
	u, err := url.Parse(c.BaseURL)
	switch {
	case err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https"):
		return []string{"base_url must be an http(s) URL with a host"}
	case u.User != nil:
		return []string{"base_url must not embed credentials; put the key in the variable named by api_key_env"}
	case u.RawQuery != "":
		return []string{"base_url must not carry a query string (it may hold a key); configure headers at the gateway instead"}
	case u.Scheme == "http" && c.APIKeyEnv != "" && !isLoopbackHost(u.Hostname()) && !c.plainHTTPAllowed(u.Host):
		return []string{"base_url must use https when an API key is sent (plain http is accepted only for a loopback host such as localhost, or a host listed in plain_http_hosts together with allow_plain_http = true in user scope)"}
	}
	return nil
}

// plainHTTPAllowed reports whether the explicit opt-in covers host (host or host:port).
func (c Config) plainHTTPAllowed(host string) bool {
	if !c.AllowPlainHTTP {
		return false
	}
	for _, h := range c.PlainHTTPHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}

// UsesPlainHTTPOptIn reports whether the plain-http opt-in is what lets a key go
// to base_url, so doctor can surface it.
func (c Config) UsesPlainHTTPOptIn() bool {
	u, err := url.Parse(c.BaseURL)
	return err == nil && u.Scheme == "http" && c.APIKeyEnv != "" && !isLoopbackHost(u.Hostname()) && c.plainHTTPAllowed(u.Host)
}

func (c Config) validatePlainHTTP() []string {
	var out []string
	if c.AllowPlainHTTP && len(c.PlainHTTPHosts) == 0 {
		out = append(out, "allow_plain_http requires plain_http_hosts (the gateways allowed over plain http)")
	}
	for _, h := range c.PlainHTTPHosts {
		if h == "" || strings.ContainsAny(h, "/@?# \t") || strings.Contains(h, "://") {
			out = append(out, "plain_http_hosts entries must be host or host:port, without scheme or path")
			break
		}
	}
	return out
}

// isLoopbackHost reports whether host is localhost or a loopback IP literal.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) validateNumbers() []string {
	var out []string
	for name, v := range map[string]float64{"max_cost_usd": c.MaxCostUSD, "price_input_per_mtok": c.PriceInputPerMTok, "price_output_per_mtok": c.PriceOutputPerMTok} {
		switch {
		case !Finite(v):
			// TOML and strconv accept nan and inf; a NaN cap passes every comparison.
			out = append(out, name+" must be a finite number")
		case v < 0:
			out = append(out, name+" must not be negative")
		}
	}
	if c.MaxRetries > MaxRetriesLimit {
		out = append(out, "max_retries must not exceed "+strconv.Itoa(MaxRetriesLimit))
	}
	for name, v := range map[string]int{"max_tokens": c.MaxTokens, "max_calls": c.MaxCalls, "timeout_seconds": c.TimeoutSeconds} {
		if v < 0 {
			out = append(out, name+" must not be negative")
		}
	}
	return out
}

// Finite reports whether v is neither NaN nor an infinity.
func Finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Err returns Validate's problems as one AR9L0 error, or nil.
func (c Config) Err() error {
	if p := c.Validate(); len(p) > 0 {
		return newError(KindConfig, "%s llm-config-invalid: %s", CodeConfigInvalid, strings.Join(p, "; "))
	}
	return nil
}
