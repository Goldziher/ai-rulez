package llm

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLiteralKeyInAPIKeyEnvIsNeverEchoed(t *testing.T) {
	for _, secret := range []string{"Zq9-Lk2mNp4RsT7vWx0Yb3Cd", "sk-proj-abc123def456ghi789", "my secret key 123"} {
		cfg := Config{Model: "x", APIKeyEnv: secret, AllowNetwork: true, Backend: BackendOpenAICompat}
		var all strings.Builder
		for _, p := range cfg.Validate() {
			all.WriteString(p + "\n")
		}
		if err := cfg.Err(); err != nil {
			all.WriteString(err.Error() + "\n")
		}
		d := Diagnose(cfg, Options{Getenv: func(string) string { return "set" }})
		var text bytes.Buffer
		d.WriteText(&text)
		all.Write(text.Bytes())
		js, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(js)
		if _, err := New(cfg, Options{}); err != nil {
			all.WriteString(err.Error())
		}
		if strings.Contains(all.String(), secret) {
			t.Errorf("literal key %q echoed:\n%s", secret, all.String())
		}
		if d.APIKeySet {
			t.Errorf("an invalid variable name must not be looked up")
		}
	}
	// invalid values of other keys are not echoed either
	cfg := Config{Backend: "sk-live-abcdef0123456789", Model: "a b"}
	if strings.Contains(strings.Join(cfg.Validate(), "|"), "sk-live") {
		t.Error("backend value echoed")
	}
	if _, err := (Config{}).WithEnv(func(k string) string {
		if k == "AI_RULEZ_LLM_MAX_CALLS" {
			return "sk-live-abcdef0123456789"
		}
		return ""
	}); err == nil || strings.Contains(err.Error(), "sk-live") {
		t.Errorf("env value echoed: %v", err)
	}
}
