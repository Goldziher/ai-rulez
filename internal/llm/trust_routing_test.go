package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestPlainHTTPOptIn(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"refused without opt-in", Config{BaseURL: "http://gateway.internal:8080/v1", APIKeyEnv: "K"}, true},
		{"accepted with opt-in and host", Config{BaseURL: "http://gateway.internal:8080/v1", APIKeyEnv: "K", AllowPlainHTTP: true, PlainHTTPHosts: []string{"gateway.internal:8080"}}, false},
		{"host match is case-insensitive", Config{BaseURL: "http://Gateway.Internal:8080/v1", APIKeyEnv: "K", AllowPlainHTTP: true, PlainHTTPHosts: []string{"gateway.internal:8080"}}, false},
		{"other port refused", Config{BaseURL: "http://gateway.internal:9090/v1", APIKeyEnv: "K", AllowPlainHTTP: true, PlainHTTPHosts: []string{"gateway.internal:8080"}}, true},
		{"other host refused", Config{BaseURL: "http://evil.example/v1", APIKeyEnv: "K", AllowPlainHTTP: true, PlainHTTPHosts: []string{"gateway.internal:8080"}}, true},
		{"flag without list refused", Config{BaseURL: "http://gateway.internal/v1", APIKeyEnv: "K", AllowPlainHTTP: true}, true},
		{"list without flag refused", Config{BaseURL: "http://gateway.internal/v1", APIKeyEnv: "K", PlainHTTPHosts: []string{"gateway.internal"}}, true},
		{"url-shaped entry rejected", Config{BaseURL: "https://x/v1", AllowPlainHTTP: true, PlainHTTPHosts: []string{"http://gateway.internal"}}, true},
		{"no key needs no opt-in", Config{BaseURL: "http://gateway.internal/v1"}, false},
		{"loopback unchanged", Config{BaseURL: "http://127.0.0.1:1/v1", APIKeyEnv: "K"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Err()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestPlainHTTPOptInIsUserScopeOnlyAndReported(t *testing.T) {
	repo := &Config{BaseURL: "http://gateway.internal/v1", AllowPlainHTTP: true, PlainHTTPHosts: []string{"gateway.internal"}, Provider: "openai"}
	user := &Config{APIKeyEnv: "K", BaseURL: "http://gateway.internal/v1"}
	cfg, ignored := Resolve(repo, user)
	if !strings.Contains(strings.Join(ignored, ","), "allow_plain_http") || !strings.Contains(strings.Join(ignored, ","), "plain_http_hosts") {
		t.Fatalf("repo opt-in must be ignored and reported: %v", ignored)
	}
	if cfg.AllowPlainHTTP || len(cfg.PlainHTTPHosts) > 0 || cfg.Err() == nil {
		t.Fatalf("repo opt-in must have no effect: %+v", cfg)
	}
	// user scope (or the environment) honours it, and doctor reports the use
	env := map[string]string{"AI_RULEZ_LLM_ALLOW_PLAIN_HTTP": "1", "AI_RULEZ_LLM_PLAIN_HTTP_HOSTS": "gateway.internal, other:80"}
	got, err := cfg.WithEnv(func(k string) string { return env[k] })
	if err != nil || got.Err() != nil || !got.UsesPlainHTTPOptIn() {
		t.Fatalf("env opt-in: %v %v", err, got.Err())
	}
	d := Diagnose(got, Options{})
	if len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], "plain-http opt-in") {
		t.Fatalf("doctor must report the opt-in: %+v", d.Warnings)
	}
}

func TestLiterLLMRefusesUserKeyWithRepoChosenProvider(t *testing.T) {
	stubFactory := func(NativeConfig) (NativeClient, error) { return &stubNative{}, nil }
	nativeMu.RLock()
	previous := nativeFactory
	nativeMu.RUnlock()
	RegisterNative(stubFactory)
	t.Cleanup(func() { RegisterNative(previous) })
	getenv := func(string) string { return "sk-test-key-value" }

	tests := []struct {
		name         string
		repo, user   *Config
		env          map[string]string
		wantRefusal  bool
		wantRouteKey string
	}{
		{"repo provider, user key", &Config{Provider: "evil", Model: "m"}, &Config{APIKeyEnv: "K", AllowNetwork: true}, nil, true, "provider"},
		{"repo model prefix, user key", &Config{Model: "evil/m"}, &Config{APIKeyEnv: "K", AllowNetwork: true}, nil, true, "model"},
		{"user provider wins over repo provider", &Config{Provider: "evil", Model: "m"}, &Config{Provider: "openai", APIKeyEnv: "K", AllowNetwork: true}, nil, false, ""},
		{"repo prefix matching the user's provider is fine", &Config{Model: "openai/m"}, &Config{Provider: "openai", APIKeyEnv: "K", AllowNetwork: true}, nil, false, ""},
		{"repo prefix differing from the user's provider", &Config{Model: "evil/m"}, &Config{Provider: "openai", APIKeyEnv: "K", AllowNetwork: true}, nil, true, "model"},
		{"user sets provider and model", &Config{Provider: "evil"}, &Config{Provider: "openai", Model: "gpt-4o-mini", APIKeyEnv: "K", AllowNetwork: true}, nil, false, ""},
		{"env-only user, repo provider", &Config{Provider: "evil", Model: "m"}, nil, map[string]string{"AI_RULEZ_LLM_API_KEY_ENV": "K", "AI_RULEZ_LLM_ALLOW_NETWORK": "1"}, true, "provider"},
		{"env-only user, repo model prefix", &Config{Model: "evil/m"}, nil, map[string]string{"AI_RULEZ_LLM_API_KEY_ENV": "K", "AI_RULEZ_LLM_ALLOW_NETWORK": "1"}, true, "model"},
		{"env-only user, env supplies provider and model", &Config{Provider: "evil", Model: "evil/m"}, nil, map[string]string{"AI_RULEZ_LLM_API_KEY_ENV": "K", "AI_RULEZ_LLM_ALLOW_NETWORK": "1", "AI_RULEZ_LLM_PROVIDER": "openai", "AI_RULEZ_LLM_MODEL": "gpt-4o-mini"}, false, ""},
		{"env supplies the provider", &Config{Provider: "evil", Model: "m"}, &Config{APIKeyEnv: "K", AllowNetwork: true}, map[string]string{"AI_RULEZ_LLM_PROVIDER": "openai"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := Resolve(tt.repo, tt.user)
			cfg, err := cfg.WithEnv(func(k string) string { return tt.env[k] })
			if err != nil {
				t.Fatal(err)
			}
			cfg.Backend = BackendLiterLLM
			_, err = New(cfg, Options{Getenv: getenv})
			refused := err != nil && errors.Is(err, ErrConfig) && strings.Contains(err.Error(), "refusing to send the key")
			if refused != tt.wantRefusal {
				t.Fatalf("refused=%v, want %v (err=%v)", refused, tt.wantRefusal, err)
			}
			if tt.wantRefusal {
				if got := Diagnose(cfg, Options{Getenv: getenv}).Problems; len(got) == 0 || !strings.Contains(strings.Join(got, ";"), "chooses the provider") {
					t.Errorf("doctor must report it: %v", got)
				}
			}
		})
	}
}

func TestBudgetChargesAmbiguousFailuresConservatively(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		free bool
	}{
		{"400", &Error{Kind: KindProvider, Status: 400}, true},
		{"401", &Error{Kind: KindAuth, Status: 401}, true},
		{"403", &Error{Kind: KindAuth, Status: 403}, true},
		{"404", &Error{Kind: KindProvider, Status: 404}, true},
		{"422", &Error{Kind: KindProvider, Status: 422}, true},
		{"429 is charged", &Error{Kind: KindRateLimit, Status: 429}, false},
		{"500 is charged", &Error{Kind: KindProvider, Status: 500}, false},
		{"502 is charged", &Error{Kind: KindProvider, Status: 502}, false},
		{"504 is charged", &Error{Kind: KindProvider, Status: 504}, false},
		{"rate limit without status is charged", &Error{Kind: KindRateLimit}, false},
		{"timeout is charged", &Error{Kind: KindTimeout}, false},
		{"config error is free", &Error{Kind: KindConfig}, true},
		{"budget refusal is free", &Error{Kind: KindBudget}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unbilled(tt.err); got != tt.free {
				t.Fatalf("unbilled=%v, want %v", got, tt.free)
			}
		})
	}
}
