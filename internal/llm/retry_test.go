package llm

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Request counts against a local server: liter-llm owns the retries, bounded by max_retries,
// so a persistent 503 costs exactly 1+max_retries requests and a permanent rejection costs one.
func TestRetriesTransientAndNotPermanentFailuresWithExactRequestCounts(t *testing.T) {
	const okBody = `{"id":"x","object":"chat.completion","created":1,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"pong"}}],"model":"m","usage":{"prompt_tokens":3,"completion_tokens":1}}`
	type step struct {
		status int
		header map[string]string
	}
	tests := []struct {
		name       string
		retries    int
		steps      []step // the last step repeats when exhausted
		wantReqs   int64
		wantErr    *Error
		minElapsed time.Duration
	}{
		{"503 once then ok", 3, []step{{503, nil}, {200, nil}}, 2, nil, 0},
		{"persistent 503 stops after 1+retries", 1, []step{{503, nil}}, 2, ErrProvider, 0},
		{"429 with Retry-After is honored", 3, []step{{429, map[string]string{"Retry-After": "1"}}, {200, nil}}, 2, nil, 900 * time.Millisecond},
		{"401 is not retried", 3, []step{{401, nil}}, 1, ErrAuth, 0},
		{"400 is not retried", 3, []step{{400, nil}}, 1, ErrProvider, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var n atomic.Int64
			srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				s := tc.steps[min(int(n.Add(1))-1, len(tc.steps)-1)]
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
			})
			cfg := localConfig(srv, Config{Provider: "openai", Model: "m", APIKeyEnv: "QA_KEY", MaxRetries: tc.retries, TimeoutSeconds: 30})
			m, err := New(cfg, Options{Getenv: func(string) string { return "local-test-key-123456" }})
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
