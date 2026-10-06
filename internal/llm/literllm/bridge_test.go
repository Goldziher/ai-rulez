//go:build cgo && literllm

package literllm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// These tests link the real liter-llm static library; run them with the
// CGO_LDFLAGS (-L only) described in docs/llm.md. They need no network beyond a local server.
func TestBridgeRoundTrip(t *testing.T) {
	var sawAuth, sawModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sawModel, _ = body["model"].(string)
		if strings.HasSuffix(r.URL.Path, "/embeddings") {
			fmt.Fprint(w, `{"object":"list","model":"m","data":[{"object":"embedding","index":0,"embedding":[0.5,0.25]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`)
			return
		}
		fmt.Fprint(w, `{"id":"x","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
	}))
	defer srv.Close()

	n, err := New("sk-test", srv.URL+"/v1", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := n.ChatJSON(context.Background(), []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "pong" || resp.Usage.PromptTokens != 5 {
		t.Fatalf("chat response %s (%v)", out, err)
	}
	if sawAuth != "Bearer sk-test" || sawModel != "gpt-4o-mini" {
		t.Fatalf("auth=%q model=%q", sawAuth, sawModel)
	}

	emb, err := n.EmbedJSON(context.Background(), []byte(`{"model":"m","input":["a"]}`))
	if err != nil || !strings.Contains(string(emb), `"embedding":[0.5,0.25]`) {
		t.Fatalf("embed %s (%v)", emb, err)
	}

	n.Free()
	n.Free() // idempotent
	if _, err := n.ChatJSON(context.Background(), []byte(`{"model":"m","messages":[]}`)); err == nil {
		t.Fatal("a freed client must return an error, not crash")
	}
}

// nativeErr is what the parent package's NativeError interface reads.
type nativeErr interface {
	NativeVariant() string
	NativeStatus() int
	NativeTransient() bool
	NativeRetryAfter() time.Duration
}

func TestBridgeTypedErrorsAndCancellation(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.Header.Get("Authorization"), "bad"):
			http.Error(w, `{"error":{"message":"invalid key"}}`, http.StatusUnauthorized)
		case strings.Contains(r.Header.Get("Authorization"), "limit"):
			w.Header().Set("Retry-After", "7")
			http.Error(w, `{"error":{"message":"slow down"}}`, http.StatusTooManyRequests)
		case strings.Contains(r.Header.Get("Authorization"), "slow"):
			<-block
		}
	}))
	defer srv.Close()
	defer close(block)
	const body = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`

	tests := []struct {
		key       string
		variant   string
		status    int
		transient bool
		retry     time.Duration
	}{
		{"bad", "Authentication", 401, false, 0},
		{"limit", "RateLimited", 429, true, 7 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.variant, func(t *testing.T) {
			// Arrange
			n, err := New(tt.key, srv.URL+"/v1", "", 10, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer n.Free()
			// Act
			_, err = n.ChatJSON(context.Background(), []byte(body))
			// Assert
			var ne nativeErr
			if !errors.As(err, &ne) {
				t.Fatalf("want a typed error, got %T %v", err, err)
			}
			if ne.NativeVariant() != tt.variant || ne.NativeStatus() != tt.status || ne.NativeTransient() != tt.transient || ne.NativeRetryAfter() != tt.retry {
				t.Errorf("variant=%s status=%d transient=%v retry=%v", ne.NativeVariant(), ne.NativeStatus(), ne.NativeTransient(), ne.NativeRetryAfter())
			}
		})
	}

	t.Run("cancellation", func(t *testing.T) {
		n, err := New("slow", srv.URL+"/v1", "", 30, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer n.Free()
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err = n.ChatJSON(ctx, []byte(body))
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
			t.Fatalf("want a prompt deadline error, got %v after %v", err, time.Since(start))
		}
	})

	t.Run("client timeout", func(t *testing.T) {
		n, err := New("slow", srv.URL+"/v1", "", 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer n.Free()
		_, err = n.ChatJSON(context.Background(), []byte(body))
		var ne nativeErr
		if !errors.As(err, &ne) || ne.NativeVariant() != "Timeout" {
			t.Fatalf("want Timeout, got %T %v", err, err)
		}
	})
}

// Upstream #249: a known provider prefix is stripped when base_url is set.
func TestBridgeStripsProviderPrefixWithBaseURL(t *testing.T) {
	var sawModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sawModel, _ = body["model"].(string)
		fmt.Fprint(w, `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()
	n, err := New("k", srv.URL+"/v1", "openai/gpt-4o-mini", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Free()
	if _, err := n.ChatJSON(context.Background(), []byte(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"x"}]}`)); err != nil {
		t.Fatal(err)
	}
	if sawModel != "gpt-4o-mini" {
		t.Fatalf("server saw model %q", sawModel)
	}
}
