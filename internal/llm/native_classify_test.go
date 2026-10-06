package llm

import (
	"context"
	"errors"
	"testing"
)

// The corpus pins the exact flat strings the liter-llm binding produces
// ("[code] message", upstream issue #244), so a reworded upstream message fails
// here instead of silently changing retry, budget or gate decisions.
func TestClassifyNativeCorpus(t *testing.T) {
	tests := []struct {
		name       string
		msg        string
		kind       Kind
		transient  bool
		wantStatus int
	}{
		{"auth sentinel text", "[1] authentication failed: invalid x-api-key", KindAuth, false, 0},
		{"unauthorized", "[1] Unauthorized", KindAuth, false, 0},
		{"http 401", "[3] provider returned HTTP 401", KindAuth, false, 0},
		{"http 403", "[3] HTTP 403 forbidden", KindAuth, false, 0},
		{"rate limited", "[2] rate limited: retry after 20s", KindRateLimit, true, 0},
		{"ratelimit one word", "[2] RateLimitError: too many requests", KindRateLimit, true, 0},
		{"http 429", "[4] HTTP 429 Too Many Requests", KindRateLimit, true, 0},
		{"context window", "[5] context window exceeded: 200123 tokens", KindContextLength, false, 0},
		{"context length", "[5] maximum context length is 128000 tokens", KindContextLength, false, 0},
		{"budget", "[6] budget exceeded", KindBudget, false, 0},
		{"server 503", "[7] upstream HTTP 503 service unavailable", KindProvider, true, 503},
		{"server 500", "[7] HTTP 500 internal error", KindProvider, true, 500},
		// Unknown messages take the safe default: provider error, never retried.
		{"unknown english", "[9] something nobody has seen before", KindProvider, false, 0},
		{"unknown localised", "[2] Ratenbegrenzung erreicht", KindProvider, false, 0},
		{"unknown empty body", "[8] native error", KindProvider, false, 0},
		{"digits inside an id", "request req-14013-x failed", KindProvider, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := classifyNative(errors.New(tt.msg))
			// Assert
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("want *Error, got %T", err)
			}
			if e.Kind != tt.kind || IsTransient(err) != tt.transient || e.Status != tt.wantStatus {
				t.Errorf("%q: kind=%s transient=%v status=%d, want %s/%v/%d", tt.msg, e.Kind, IsTransient(err), e.Status, tt.kind, tt.transient, tt.wantStatus)
			}
		})
	}
}

func TestClassifyNativeKeepsContextErrors(t *testing.T) {
	if err := classifyNative(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context errors must pass through: %v", err)
	}
}

func TestLiterLLMNilNilIsAnError(t *testing.T) {
	stub := &stubNative{
		chat:  func([]byte) ([]byte, error) { return nil, nil },
		embed: func([]byte) ([]byte, error) { return []byte{}, nil },
	}
	l := &literLLM{native: stub, model: "m", embedModel: "e"}
	if r, err := l.Chat(context.Background(), chatReq("x")); err == nil || IsTransient(err) {
		t.Fatalf("(nil, nil) from Chat must be a permanent error, got %+v %v", r, err)
	}
	if r, err := l.Embed(context.Background(), EmbedRequest{Input: []string{"a"}}); err == nil || IsTransient(err) {
		t.Fatalf("(nil, nil) from Embed must be a permanent error, got %+v %v", r, err)
	}
}
