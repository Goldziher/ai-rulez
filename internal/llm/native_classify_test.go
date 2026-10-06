package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The corpus pins the liter-llm error variant names the classifier reads, so a
// renamed upstream variant fails here instead of silently changing retry,
// budget or gate decisions.
func TestClassifyNativeCorpus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		kind       Kind
		transient  bool
		wantStatus int
		wantRetry  time.Duration
	}{
		{"authentication", &stubNativeErr{variant: "Authentication", status: 401}, KindAuth, false, 401, 0},
		{"rate limited with delay", &stubNativeErr{variant: "RateLimited", status: 429, transient: true, retryAfter: 20 * time.Second}, KindRateLimit, true, 429, 20 * time.Second},
		{"context window", &stubNativeErr{variant: "ContextWindowExceeded", status: 400}, KindContextLength, false, 400, 0},
		{"budget", &stubNativeErr{variant: "BudgetExceeded"}, KindBudget, false, 0, 0},
		{"timeout", &stubNativeErr{variant: "Timeout", transient: true}, KindTimeout, true, 0, 0},
		{"server error", &stubNativeErr{variant: "ServerError", status: 500, transient: true}, KindProvider, true, 500, 0},
		{"service unavailable", &stubNativeErr{variant: "ServiceUnavailable", status: 503, transient: true}, KindProvider, true, 503, 0},
		{"bad request", &stubNativeErr{variant: "BadRequest", status: 400}, KindProvider, false, 400, 0},
		{"not found", &stubNativeErr{variant: "NotFound", status: 404}, KindProvider, false, 404, 0},
		{"content policy", &stubNativeErr{variant: "ContentPolicy", status: 400}, KindProvider, false, 400, 0},
		{"unknown variant", &stubNativeErr{variant: "SomethingNew"}, KindProvider, false, 0, 0},
		{"wrapped typed error", fmt.Errorf("chat: %w", &stubNativeErr{variant: "RateLimited", transient: true}), KindRateLimit, true, 0, 0},
		{"untyped error", errors.New("[2] rate limited: not trusted as text"), KindProvider, false, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := classifyNative(tt.err)
			// Assert
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("want *Error, got %T", err)
			}
			if e.Kind != tt.kind || IsTransient(err) != tt.transient || e.Status != tt.wantStatus || e.RetryAfter != tt.wantRetry {
				t.Errorf("kind=%s transient=%v status=%d retry=%v, want %s/%v/%d/%v", e.Kind, IsTransient(err), e.Status, e.RetryAfter, tt.kind, tt.transient, tt.wantStatus, tt.wantRetry)
			}
		})
	}
}

func TestClassifyNativeRedactsKeys(t *testing.T) {
	err := classifyNative(&stubNativeErr{variant: "Authentication", msg: "bad key sk-abcdefghijklmnopqrstuvwx"})
	if strings.Contains(err.Error(), "abcdefghij") {
		t.Fatalf("key leaked: %v", err)
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
