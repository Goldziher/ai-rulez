package llm

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Diagnosis is the resolved, secret-free view of the LLM setup.
type Diagnosis struct {
	Backend        string   `json:"backend"`
	NativeCompiled bool     `json:"literllm_compiled"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	EmbeddingModel string   `json:"embedding_model,omitempty"`
	BaseURLHost    string   `json:"base_url_host,omitempty"`
	APIKeyEnv      string   `json:"api_key_env,omitempty"`
	APIKeySet      bool     `json:"api_key_set"`
	AllowNetwork   bool     `json:"allow_network"`
	CacheEnabled   bool     `json:"cache_enabled"`
	CacheDir       string   `json:"cache_dir,omitempty"`
	Limits         string   `json:"limits"`
	Problems       []string `json:"problems,omitempty"`
}

// Diagnose resolves cfg (with env overrides applied by the caller) into a
// Diagnosis. It reads the environment only to learn whether the key variable is
// set, never its value, and makes no network call.
func Diagnose(cfg Config, opts Options) Diagnosis {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	d := Diagnosis{
		Backend:        ResolveBackend(cfg.Backend),
		NativeCompiled: NativeAvailable(),
		Provider:       cfg.Provider,
		Model:          cfg.FullModel(),
		EmbeddingModel: cfg.EmbeddingModel,
		APIKeyEnv:      cfg.APIKeyEnv,
		AllowNetwork:   cfg.AllowNetwork,
		CacheEnabled:   cfg.CacheEnabled() && !opts.NoCache && opts.ConfigDir != "",
		Limits:         Limits{MaxCostUSD: cfg.MaxCostUSD, MaxTokens: cfg.MaxTokens, MaxCalls: cfg.MaxCalls}.Describe(),
		Problems:       cfg.Validate(),
	}
	if d.CacheEnabled {
		d.CacheDir = filepath.Join(opts.ConfigDir, filepath.FromSlash(CacheDirRel))
	}
	if u, err := url.Parse(cfg.BaseURL); err == nil {
		d.BaseURLHost = u.Host
	}
	if cfg.APIKeyEnv != "" && envNameRe.MatchString(cfg.APIKeyEnv) && !looksLikeSecret(cfg.APIKeyEnv) {
		d.APIKeySet = strings.TrimSpace(getenv(cfg.APIKeyEnv)) != ""
	}
	if d.Backend == BackendLiterLLM && !d.NativeCompiled {
		d.Problems = append(d.Problems, "backend literllm is selected but not compiled in (build with -tags literllm)")
	}
	if cfg.AllowNetwork && cfg.Model == "" {
		d.Problems = append(d.Problems, "no model configured")
	}
	return d
}

// WriteText prints the diagnosis for humans. The API key variable is named, its value never shown.
func (d Diagnosis) WriteText(w io.Writer) {
	host := d.BaseURLHost
	if host == "" {
		host = "(provider default)"
	}
	key := "(none)"
	if d.APIKeyEnv != "" {
		key = d.APIKeyEnv + " (set: " + fmt.Sprint(d.APIKeySet) + ")"
	}
	cache := "off"
	if d.CacheEnabled {
		cache = d.CacheDir
	}
	printf(w, "backend:         %s (literllm compiled in: %v)\n", d.Backend, d.NativeCompiled)
	printf(w, "model:           %s\n", orNone(d.Model))
	printf(w, "embedding model: %s\n", orNone(d.EmbeddingModel))
	printf(w, "base_url host:   %s\n", host)
	printf(w, "api key env:     %s\n", key)
	printf(w, "network allowed: %v\n", d.AllowNetwork)
	printf(w, "cache:           %s\n", cache)
	printf(w, "limits:          %s\n", d.Limits)
	for _, p := range d.Problems {
		printf(w, "problem:         %s %s\n", CodeConfigInvalid, p)
	}
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// Ping makes one 1-token call. It refuses unless allow_network is true, and it
// bypasses the cache so it tests the real path.
func Ping(ctx context.Context, cfg Config, opts Options) error {
	opts.NoCache = true
	c, err := New(cfg, opts)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Chat(ctx, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "ping"}}, MaxTokens: 1, NoCache: true})
	return err
}

// EstimateResult is the offline cost estimate for sending a file as a prompt.
type EstimateResult struct {
	Model        string  `json:"model"`
	Bytes        int     `json:"bytes"`
	PromptTokens int     `json:"prompt_tokens"`
	MaxOutput    int     `json:"max_completion_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	CostKnown    bool    `json:"cost_known"`
}

// Estimate approximates the tokens and cost of sending text as one user
// message. No call is made and nothing is read from the network.
func Estimate(cfg Config, text string, maxOutput int) EstimateResult {
	if maxOutput <= 0 {
		maxOutput = DefaultCompletionCap
	}
	req := ChatRequest{Messages: []Message{{Role: RoleUser, Content: text}}}
	u := Usage{PromptTokens: EstimatePromptTokens(req), CompletionTokens: maxOutput}
	cost, known := NewPricing(cfg).Cost(cfg.FullModel(), u)
	return EstimateResult{Model: cfg.FullModel(), Bytes: len(text), PromptTokens: u.PromptTokens, MaxOutput: maxOutput, CostUSD: cost, CostKnown: known}
}
