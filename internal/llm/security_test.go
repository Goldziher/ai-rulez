package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestOpenAICompatNeverFollowsRedirects(t *testing.T) {
	var otherHits atomic.Int32
	var otherBody atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		b, _ := io.ReadAll(r.Body) //nolint:errcheck // test
		otherBody.Store(string(b))
		fmt.Fprint(w, `{"choices":[{"message":{"content":"x"}}]}`)
	}))
	defer other.Close()
	// a second loopback name is a different host for the redirect policy
	target := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target+"/other", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	env := map[string]string{"K": "sk-live-very-secret-123456"}
	// no HTTPClient injected: the default client must refuse the hop
	m, err := New(allowed(Config{Model: "m", BaseURL: srv.URL + "/v1", APIKeyEnv: "K", Cache: ptr(false), MaxRetries: -1}), Options{Getenv: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Chat(context.Background(), chatReq("private prompt"))
	if err == nil || !errors.Is(err, ErrProvider) {
		t.Fatalf("a redirect must surface as a provider error, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("prompt was re-POSTed to another host: %v", otherBody.Load())
	}
	// an injected client is wrapped too
	m, err = New(allowed(Config{Model: "m", BaseURL: srv.URL + "/v1", APIKeyEnv: "K", Cache: ptr(false), MaxRetries: -1}), Options{Getenv: func(k string) string { return env[k] }, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Chat(context.Background(), chatReq("private prompt")); err == nil || otherHits.Load() != 0 {
		t.Fatalf("injected client followed a redirect: %v hits=%d", err, otherHits.Load())
	}
}

func TestPlainHTTPWithKeyNeedsLoopback(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		bad  bool
	}{
		{"https remote with key", Config{BaseURL: "https://gw.example/v1", APIKeyEnv: "K"}, false},
		{"http remote with key", Config{BaseURL: "http://gw.example/v1", APIKeyEnv: "K"}, true},
		{"http remote no key", Config{BaseURL: "http://ollama.lan:11434/v1"}, false},
		{"http localhost with key", Config{BaseURL: "http://localhost:8080/v1", APIKeyEnv: "K"}, false},
		{"http 127.0.0.1 with key", Config{BaseURL: "http://127.0.0.1:8080/v1", APIKeyEnv: "K"}, false},
		{"http ::1 with key", Config{BaseURL: "http://[::1]:8080/v1", APIKeyEnv: "K"}, false},
		{"http lookalike with key", Config{BaseURL: "http://localhost.evil.example/v1", APIKeyEnv: "K"}, true},
	}
	for _, tc := range cases {
		got := strings.Contains(strings.Join(tc.cfg.Validate(), "|"), "https")
		if got != tc.bad {
			t.Errorf("%s: problems=%v want bad=%v", tc.name, tc.cfg.Validate(), tc.bad)
		}
	}
}
