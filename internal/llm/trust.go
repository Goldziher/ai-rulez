package llm

import "strings"

// A committed repository config, and the machine-local overlay that lives in
// the checkout, is attacker-controlled input: cloning a repository must not
// enable network use, pick the host a prompt goes to, or choose which
// environment variable is sent as a credential. Those keys are honored only
// from user scope (the user config file or AI_RULEZ_LLM_* environment variables).
// This mirrors the trust rule of [telemetry].

// PrivilegedKeys names the [llm] keys that only user scope may set and that a
// repository config sets in c. The result is in a stable order.
func (c Config) PrivilegedKeys() []string {
	var out []string
	add := func(set bool, key string) {
		if set {
			out = append(out, key)
		}
	}
	add(c.AllowNetwork, "allow_network")
	add(c.BaseURL != "", "base_url")
	add(c.APIKeyEnv != "", "api_key_env")
	add(c.AllowPlainHTTP, "allow_plain_http")
	add(len(c.PlainHTTPHosts) > 0, "plain_http_hosts")
	add(c.PriceInputPerMTok != 0, "price_input_per_mtok")
	add(c.PriceOutputPerMTok != 0, "price_output_per_mtok")
	return out
}

// stripPrivileged returns c without the user-scope-only keys.
func (c Config) stripPrivileged() Config {
	c.AllowNetwork = false
	c.BaseURL = ""
	c.APIKeyEnv = ""
	c.AllowPlainHTTP = false
	c.PlainHTTPHosts = nil
	c.PriceInputPerMTok = 0
	c.PriceOutputPerMTok = 0
	return c
}

// Resolve layers a repository [llm] table and a user-scope one into the
// effective config, applying the trust rule. Privileged keys set in repo are
// ignored and returned by name; limits a repo sets can only tighten the user's
// (the lower non-zero value wins). The AI_RULEZ_LLM_* environment is applied
// afterwards by WithEnv and, being user scope, may set everything.
func Resolve(repo, user *Config) (cfg Config, ignored []string) {
	if repo != nil {
		ignored = repo.PrivilegedKeys()
		cfg = repo.stripPrivileged()
	}
	if user == nil {
		return cfg, ignored
	}
	u := *user
	merged := u
	// Non-privileged keys the user leaves unset fall back to the repository's.
	fill := func(dst *string, repoVal string) {
		if *dst == "" {
			*dst = repoVal
		}
	}
	fill(&merged.Provider, cfg.Provider)
	fill(&merged.Model, cfg.Model)
	fill(&merged.Backend, cfg.Backend)
	fill(&merged.EmbeddingModel, cfg.EmbeddingModel)
	if merged.Cache == nil {
		merged.Cache = cfg.Cache
	}
	if merged.TimeoutSeconds == 0 {
		merged.TimeoutSeconds = cfg.TimeoutSeconds
	}
	if merged.MaxRetries == 0 {
		merged.MaxRetries = cfg.MaxRetries
	}
	// Provider routing decides which service receives the user's key. When the
	// repository supplies it (the provider field, or a provider/ prefix in model)
	// and the user did not, remember that so the literllm backend can refuse to
	// send the user's key along it.
	merged.repoProvider = u.Provider == "" && cfg.Provider != ""
	merged.repoModelRoute = u.Model == "" && strings.Contains(cfg.Model, "/") &&
		(u.Provider == "" || modelPrefix(cfg.Model) != u.Provider)
	merged.MaxCostUSD = tighterFloat(u.MaxCostUSD, cfg.MaxCostUSD)
	merged.MaxTokens = tighterInt(u.MaxTokens, cfg.MaxTokens)
	merged.MaxCalls = tighterInt(u.MaxCalls, cfg.MaxCalls)
	return merged, ignored
}

func modelPrefix(model string) string {
	prefix, _, _ := strings.Cut(model, "/")
	return prefix
}

// RoutingFromRepo names the provider-routing keys ("provider", "model") whose
// value came from a repository config. They choose the service a literllm call
// goes to, so the literllm backend refuses to send a user-scope key along them
// (see newLiterLLM); set them in user scope, or via AI_RULEZ_LLM_*.
func (c Config) RoutingFromRepo() []string {
	var out []string
	if c.repoProvider {
		out = append(out, "provider")
	}
	if c.repoModelRoute {
		out = append(out, "model")
	}
	return out
}

func tighterInt(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	default:
		return min(a, b)
	}
}

func tighterFloat(a, b float64) float64 {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	default:
		return min(a, b)
	}
}

// IgnoredKeysMessage explains which repository [llm] keys were dropped by the trust rule.
func IgnoredKeysMessage(keys []string) string {
	return "repository [llm] keys ignored (user scope only: set them in the user config file ~/.config/ai-rulez/config.toml or via AI_RULEZ_LLM_*): " + strings.Join(keys, ", ")
}
