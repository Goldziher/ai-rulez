package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The corpus pins what a local server's failures classify to through the real
// liter-llm client, so a renamed or reshaped upstream error fails here instead of
// silently changing gate, budget or retry decisions.
func TestClassifyProviderFailureCorpus(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		header     map[string]string
		body       string
		kind       Kind
		transient  bool
		wantRetry  time.Duration
		wantStatus int
	}{
		{"authentication", 401, nil, `{"error":{"message":"bad key"}}`, KindAuth, false, 0, 401},
		{"forbidden", 403, nil, `{"error":{"message":"no"}}`, KindAuth, false, 0, 403},
		{"rate limited with delay", 429, map[string]string{"Retry-After": "20"}, `{"error":{"message":"slow"}}`, KindRateLimit, true, 20 * time.Second, 429},
		{"exhausted quota is permanent", 429, nil, `{"error":{"message":"You exceeded your current quota","code":"insufficient_quota"}}`, KindRateLimit, false, 0, 429},
		{"context window", 400, nil, `{"error":{"message":"This model's maximum context length is 8192 tokens","code":"context_length_exceeded"}}`, KindContextLength, false, 0, 400},
		{"anthropic prompt too long", 400, nil, `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 250000 tokens > 200000 maximum"}}`, KindContextLength, false, 0, 400},
		{"server error", 500, nil, `{"error":{"message":"boom"}}`, KindProvider, true, 0, 500},
		{"service unavailable", 503, nil, `{"error":{"message":"down"}}`, KindProvider, true, 0, 503},
		{"bad request", 400, nil, `{"error":{"message":"bad field"}}`, KindProvider, false, 0, 400},
		{"not found", 404, nil, `{"error":{"message":"no such model"}}`, KindProvider, false, 0, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})
			l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: -1}), nil)

			// Act
			_, err := l.Chat(context.Background(), chatReq("x"))

			// Assert
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("want *Error, got %T %v", err, err)
			}
			if e.Kind != tt.kind || IsTransient(err) != tt.transient || e.Status != tt.wantStatus || e.RetryAfter != tt.wantRetry {
				t.Errorf("kind=%s transient=%v status=%d retry=%v, want %s/%v/%d/%v", e.Kind, IsTransient(err), e.Status, e.RetryAfter, tt.kind, tt.transient, tt.wantStatus, tt.wantRetry)
			}
		})
	}
}

func TestClassifyConnectionFailureIsTransientAndNamesNoURL(t *testing.T) {
	srv := serve(t, func(http.ResponseWriter, *http.Request) {})
	cfg := localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: -1})
	srv.Close()
	l := newLocal(t, cfg, nil)

	_, err := l.Chat(context.Background(), chatReq("x"))

	if !errors.Is(err, ErrProvider) || !IsTransient(err) {
		t.Fatalf("a refused connection is a transient provider error: %v", err)
	}
}

func TestClassifyTimeoutIsATransientTimeout(t *testing.T) {
	block := make(chan struct{})
	srv := serve(t, func(http.ResponseWriter, *http.Request) { <-block })
	t.Cleanup(func() { close(block) })
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: -1, TimeoutSeconds: 1}), nil)

	_, err := l.Chat(context.Background(), chatReq("x"))

	if !errors.Is(err, ErrTimeout) || !IsTransient(err) {
		t.Fatalf("a client timeout is a transient timeout error: %v", err)
	}
}

// liter-llm echoes what the provider returned and the base_url query in its errors.
func TestClassifyScrubsKeysAndURLQueries(t *testing.T) {
	const key = "local-test-key-123456"
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":{"message":"bad key %s and sk-abcdefghijklmnopqrstuvwx at https://api.example.com/v1?api_key=hunter2hunter2"}}`, key)
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", APIKeyEnv: "K", MaxRetries: -1}), func(string) string { return key })
	l.key = key

	_, err := l.Chat(context.Background(), chatReq("x"))

	for _, secret := range []string{key, "abcdefghij", "hunter2"} {
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("%q leaked: %v", secret, err)
		}
	}
}

func TestClassifyPassesContextErrorsThrough(t *testing.T) {
	l := &literLLM{}
	if err := l.classify(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context errors must pass through: %v", err)
	}
	if err := l.classify(nil); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyUntypedErrorIsPermanent(t *testing.T) {
	l := &literLLM{}
	err := l.classify(errors.New("[2] rate limited: not trusted as text"))
	var e *Error
	if !errors.As(err, &e) || e.Kind != KindProvider || IsTransient(err) {
		t.Fatalf("an error that is not a liter-llm error is a permanent provider error: %v", err)
	}
}

// The HTTP body of one reply is bounded by the client's max_response_bytes, so an oversized
// body is refused while it is read, before the reply is decoded.
func TestChatRefusesAnOversizedReply(t *testing.T) {
	// Arrange
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}]}`, strings.Repeat("x", maxResponseBytes+1))
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: -1}), nil)

	// Act
	_, err := l.Chat(context.Background(), chatReq("x"))

	// Assert
	var e *Error
	if !errors.As(err, &e) || e.Kind != KindProvider || IsTransient(err) || !strings.Contains(e.Message, "configured limit") {
		t.Fatalf("err = %v, want a permanent provider error refusing the oversized body", err)
	}
}

func TestChatCarriesFinishReason(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"x"},"finish_reason":"length"}]}`)
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: -1}), nil)

	resp, err := l.Chat(context.Background(), chatReq("x"))

	if err != nil || resp.FinishReason != "length" {
		t.Fatalf("finish reason lost: %+v %v", resp, err)
	}
}

// OpenAI's reasoning models reject max_tokens.
func TestReasoningModelsGetMaxCompletionTokens(t *testing.T) {
	for model, want := range map[string]bool{
		"openai/gpt-5": true, "gpt-5-mini": true, "openai/o3": true, "o4-mini": true, "openai/gpt-4o": false,
		"gemini/gemini-2.5-flash": false, "ollama/gpt-5-clone": false,
	} {
		req := chatRequest(model, ChatRequest{MaxTokens: 10})
		if got := req.MaxCompletionTokens != nil; got != want || (req.MaxTokens != nil) == want {
			t.Errorf("%s: max_completion_tokens=%v max_tokens=%v, want completion cap %v", model, req.MaxCompletionTokens, req.MaxTokens, want)
		}
	}
}
