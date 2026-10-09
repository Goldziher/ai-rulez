package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
		{"a bare host entry does not cover a base_url with a port", Config{BaseURL: "http://gateway.internal:8080/v1", APIKeyEnv: "K", AllowPlainHTTP: true, PlainHTTPHosts: []string{"gateway.internal"}}, true},
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
	// user scope (or the environment) honors it, and doctor reports the use
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

func TestRefusesUserKeyWithRepoChosenProvider(t *testing.T) {
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
		// Without api_key_env liter-llm reads the routed provider's own variable (OPENAI_API_KEY, ...),
		// so a repository route still picks which of the user's keys is sent and billed (RV-LLM-10).
		{"repo embedding route, no api_key_env", &Config{EmbeddingModel: "openai/text-embedding-3-large"}, &Config{Provider: "gemini", Model: "gemini-2.5-flash", AllowNetwork: true}, nil, true, "embedding_model"},
		{"repo provider, no api_key_env", &Config{Provider: "evil", Model: "m"}, &Config{AllowNetwork: true}, nil, true, "provider"},
		{"user route, no api_key_env", nil, &Config{Provider: "gemini", Model: "gemini-2.5-flash", AllowNetwork: true}, nil, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := Resolve(tt.repo, tt.user)
			cfg, err := cfg.WithEnv(func(k string) string { return tt.env[k] })
			if err != nil {
				t.Fatal(err)
			}
			m, err := New(cfg, Options{Getenv: getenv})
			if m != nil {
				t.Cleanup(func() { _ = m.Close() })
			}
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

// A request model chosen by the repository (verifier llm.model, [search.embeddings] model)
// must not reroute the user's key to another provider: the literllm backend routes on the
// provider/ prefix of the model it is given. With a base_url liter-llm strips the
// configured provider's prefix, so the wire model is the bare name.
func TestLiterLLMShouldRefuseRequestModelRoutedToAnotherProvider(t *testing.T) {
	tests := []struct {
		name      string
		reqModel  string
		wantModel string
		wantErr   bool
	}{
		{"no override uses the configured model", "", "gpt-4o-mini", false},
		{"same provider prefix", "openai/gpt-4o", "gpt-4o", false},
		{"bare model is pinned to the configured provider", "gpt-4o", "gpt-4o", false},
		{"other provider prefix is refused", "evil/gpt-4o", "", true},
		{"nested prefix on another provider is refused", "evil/openai/gpt-4o", "", true},
	}
	for _, tt := range tests {
		for _, kind := range []string{"chat", "embed"} {
			t.Run(kind+" "+tt.name, func(t *testing.T) {
				// Arrange
				var sent string
				srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
					sent, _ = readBody(r)["model"].(string)
					if kind == "chat" {
						fmt.Fprint(w, chatReply)
						return
					}
					fmt.Fprint(w, `{"object":"list","model":"x","data":[{"object":"embedding","index":0,"embedding":[1]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`)
				})
				l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "gpt-4o-mini", EmbeddingModel: "gpt-4o-mini", MaxRetries: -1}), nil)

				// Act
				var err error
				if kind == "chat" {
					_, err = l.Chat(context.Background(), ChatRequest{Model: tt.reqModel, Messages: []Message{{Role: RoleUser, Content: "hi"}}})
				} else {
					_, err = l.Embed(context.Background(), EmbedRequest{Model: tt.reqModel, Input: []string{"a"}})
				}

				// Assert
				if tt.wantErr {
					var e *Error
					if !errors.As(err, &e) || e.Kind != KindConfig || sent != "" {
						t.Fatalf("want a config refusal with nothing sent, got err=%v sent=%q", err, sent)
					}
					return
				}
				if err != nil || sent != tt.wantModel {
					t.Fatalf("err=%v sent=%q, want %q", err, sent, tt.wantModel)
				}
			})
		}
	}
}

func TestResolveShouldFlagRepoEmbeddingModelRoute(t *testing.T) {
	tests := []struct {
		name       string
		repo, user *Config
		want       bool
	}{
		{"repo prefix, user provider differs", &Config{EmbeddingModel: "evil/e"}, &Config{Provider: "openai"}, true},
		{"repo prefix equals user provider", &Config{EmbeddingModel: "openai/e"}, &Config{Provider: "openai"}, false},
		{"user sets the embedding model", &Config{EmbeddingModel: "evil/e"}, &Config{Provider: "openai", EmbeddingModel: "e"}, false},
		{"bare repo model", &Config{EmbeddingModel: "e"}, &Config{Provider: "openai"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := Resolve(tt.repo, tt.user)
			if got := strings.Contains(strings.Join(cfg.RoutingFromRepo(), ","), "embedding_model"); got != tt.want {
				t.Errorf("RoutingFromRepo = %v, want embedding_model=%v", cfg.RoutingFromRepo(), tt.want)
			}
		})
	}
}
