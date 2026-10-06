package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Request counts per backend against a local server: the retry policy must be
// ours alone (liter-llm is created with zero retries), so a persistent 503 costs
// exactly 1+max_retries requests on both backends.
func TestBackendsShouldRetryTransientAndNotPermanentFailuresWithExactRequestCounts(t *testing.T) {
	const okBody = `{"id":"x","object":"chat.completion","created":1,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"pong"}}],"model":"m","usage":{"prompt_tokens":3,"completion_tokens":1}}`
	type step struct {
		status int
		header map[string]string
	}
	tests := []struct {
		name       string
		steps      []step // repeated last step when exhausted
		wantReqs   int64
		wantErr    *Error
		minElapsed time.Duration
	}{
		{"503 twice then ok", []step{{503, nil}, {503, nil}, {200, nil}}, 3, nil, 0},
		{"persistent 503 stops after 1+retries", []step{{503, nil}}, 4, ErrProvider, 0},
		{"429 with Retry-After is honoured", []step{{429, map[string]string{"Retry-After": "1"}}, {200, nil}}, 2, nil, 900 * time.Millisecond},
		{"401 is not retried", []step{{401, nil}}, 1, ErrAuth, 0},
		{"400 is not retried", []step{{400, nil}}, 1, ErrProvider, 0},
	}
	for _, backend := range []string{BackendOpenAICompat, BackendLiterLLM} {
		for _, tc := range tests {
			t.Run(backend+"/"+tc.name, func(t *testing.T) {
				if backend == BackendLiterLLM && !NativeAvailable() {
					t.Skip("literllm not compiled in (-tags literllm)")
				}
				// Arrange
				var n atomic.Int64
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					i := int(n.Add(1)) - 1
					s := tc.steps[min(i, len(tc.steps)-1)]
					for k, v := range s.header {
						w.Header().Set(k, v)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(s.status)
					if s.status == 200 {
						_, _ = w.Write([]byte(okBody))
					} else {
						_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
					}
				}))
				defer srv.Close()
				cfg := Config{Backend: backend, Provider: "openai", Model: "m", BaseURL: srv.URL + "/v1", APIKeyEnv: "QA_KEY",
					AllowNetwork: true, Cache: ptr(false), MaxRetries: 3, TimeoutSeconds: 30}
				opts := Options{Getenv: func(string) string { return "local-test-key-123456" }}
				if tc.minElapsed == 0 {
					opts.Retry = &RetryPolicy{Retries: 3, Base: time.Millisecond, Max: 2 * time.Millisecond}
				}
				m, err := New(cfg, opts)
				if err != nil {
					t.Fatal(err)
				}
				defer m.Close() //nolint:errcheck // test

				// Act
				start := time.Now()
				_, err = m.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "ping"}}, MaxTokens: 4})

				// Assert
				if got := n.Load(); got != tc.wantReqs {
					t.Errorf("requests = %d, want %d (err=%v)", got, tc.wantReqs, err)
				}
				if tc.wantErr == nil && err != nil {
					t.Errorf("err = %v, want success", err)
				}
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Errorf("err = %v, want kind %s", err, tc.wantErr.Kind)
				}
				if el := time.Since(start); el < tc.minElapsed {
					t.Errorf("returned after %v, want at least %v (Retry-After)", el, tc.minElapsed)
				}
			})
		}
	}
}
