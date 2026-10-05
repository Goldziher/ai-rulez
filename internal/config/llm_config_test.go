package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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

	// a teammate opts in on their own machine; the shared file stays off
	if err := os.WriteFile(filepath.Join(cfgDir, "config.local.toml"), []byte("[llm]\nallow_network = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(context.Background(), dir, WithoutRemote())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM == nil || !cfg.LLM.AllowNetwork || cfg.LLM.Model != "gpt-4o-mini" {
		t.Fatalf("local overlay must merge per key: %+v", cfg.LLM)
	}
}
