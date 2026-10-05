package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserConfigFileIsTheOnePathForLLMAndTelemetry(t *testing.T) {
	xdg := t.TempDir()
	get := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return xdg
		}
		return ""
	}
	want := filepath.Join(xdg, "ai-rulez", "config.toml")
	if got := UserConfigFile(get); got != want {
		t.Fatalf("UserConfigFile = %q, want %q", got, want)
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("[llm]\nallow_network = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := (&Config{}).ResolveLLM(get)
	if err != nil {
		t.Fatal(err)
	}
	if res.UserFile != want || !res.Config.AllowNetwork {
		t.Fatalf("ResolveLLM read %q (allow_network=%v), want %q", res.UserFile, res.Config.AllowNetwork, want)
	}
}
