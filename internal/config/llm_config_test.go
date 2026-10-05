package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/llm"
)

func TestLoadLLMTableAndLocalOverlay(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shared := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[llm]\nprovider = \"openai\"\nmodel = \"gpt-4o-mini\"\napi_key_env = \"OPENAI_API_KEY\"\nmax_cost_usd = 1.5\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(shared), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(context.Background(), dir, WithoutRemote())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM == nil || cfg.LLM.Model != "gpt-4o-mini" || cfg.LLM.MaxCostUSD != 1.5 || cfg.LLM.AllowNetwork {
		t.Fatalf("shared [llm] not loaded with network off: %+v", cfg.LLM)
	}

	// the local overlay lives in the checkout, so it is repository scope: it cannot opt in
	if err := os.WriteFile(filepath.Join(cfgDir, "config.local.toml"), []byte("[llm]\nallow_network = true\nmax_tokens = 10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(context.Background(), dir, WithoutRemote())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM == nil || !cfg.LLM.AllowNetwork || cfg.LLM.Model != "gpt-4o-mini" || cfg.LLM.MaxTokens != 10 {
		t.Fatalf("local overlay must still merge per key: %+v", cfg.LLM)
	}
	isolateLLMEnv(t)
	res, err := cfg.ResolveLLM(nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.AllowNetwork || res.Config.APIKeyEnv != "" || res.Config.MaxTokens != 10 {
		t.Fatalf("repo scope must not enable the network or name a key variable: %+v", res.Config)
	}
	if got := strings.Join(res.Ignored, ","); got != "allow_network,api_key_env" {
		t.Fatalf("ignored keys: %q", got)
	}
}

// isolateLLMEnv points user scope at an empty directory and clears AI_RULEZ_LLM_*.
func isolateLLMEnv(t *testing.T) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, k := range []string{"PROVIDER", "MODEL", "BACKEND", "BASE_URL", "API_KEY_ENV", "EMBEDDING_MODEL", "MAX_COST_USD", "MAX_TOKENS", "MAX_CALLS", "TIMEOUT_SECONDS", "CACHE", "ALLOW_NETWORK"} {
		t.Setenv("AI_RULEZ_LLM_"+k, "")
	}
	return xdg
}

func TestResolveLLMTrustRule(t *testing.T) {
	repo := &Config{LLM: &llm.Config{
		Model: "gpt-4o-mini", AllowNetwork: true, BaseURL: "https://evil.example/v1", APIKeyEnv: "AWS_SECRET_ACCESS_KEY",
		PriceInputPerMTok: 0.0001, MaxCostUSD: 5, MaxCalls: 100,
	}}
	xdg := isolateLLMEnv(t)

	res, err := repo.ResolveLLM(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := res.Config
	if c.AllowNetwork || c.BaseURL != "" || c.APIKeyEnv != "" || c.PriceInputPerMTok != 0 {
		t.Fatalf("repo scope must not set privileged keys: %+v", c)
	}
	if c.Model != "gpt-4o-mini" || c.MaxCostUSD != 5 || c.MaxCalls != 100 {
		t.Fatalf("repo scope keeps non-sensitive knobs: %+v", c)
	}
	if got := strings.Join(res.Ignored, ","); got != "allow_network,base_url,api_key_env,price_input_per_mtok" {
		t.Fatalf("ignored: %q", got)
	}

	// user config file: privileged keys honoured; repo limits can only tighten the user's
	dir := filepath.Join(xdg, "ai-rulez")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	user := "[llm]\nallow_network = true\nbase_url = \"https://gw.internal/v1\"\napi_key_env = \"GW_KEY\"\nmax_cost_usd = 2\nmax_calls = 500\nmodel = \"user-model\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err = repo.ResolveLLM(nil)
	if err != nil {
		t.Fatal(err)
	}
	c = res.Config
	if !c.AllowNetwork || c.BaseURL != "https://gw.internal/v1" || c.APIKeyEnv != "GW_KEY" || c.Model != "user-model" {
		t.Fatalf("user scope must be honoured: %+v", c)
	}
	if c.MaxCostUSD != 2 || c.MaxCalls != 100 {
		t.Fatalf("limits: the lower of user and repo wins: %+v", c)
	}

	// environment is user scope too
	t.Setenv("AI_RULEZ_LLM_BASE_URL", "https://env.example/v1")
	res, err = repo.ResolveLLM(nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.BaseURL != "https://env.example/v1" {
		t.Fatalf("env must win: %+v", res.Config)
	}
}
