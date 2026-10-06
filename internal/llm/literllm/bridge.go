//go:build cgo && literllm

// Package literllm is the cgo bridge to the liter-llm Go binding. It lives in its
// own Go module so the default ai-rulez module graph, go.sum and CGO_ENABLED=0
// builds never see the binding. Only OpenAI-shaped JSON crosses this boundary;
// the request and response mapping lives in the parent llm package, where it is
// tested without cgo.
//
// Release builds enable it with GOWORK=literllm.work (both modules) and
// -tags literllm; see docs/llm.md. Status: experimental.
package literllm

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	lit "github.com/xberg-io/liter-llm/packages/go/v2"
)

// Native wraps one liter-llm DefaultClient.
type Native struct {
	mu     sync.RWMutex // calls hold RLock; Free waits for in-flight calls (cancel their context to end them sooner)
	client *lit.DefaultClient
}

// New creates a client. baseURL may be empty (provider routing decides).
// modelHint is the configured model ("provider/model"); with a baseURL, liter-llm
// strips that provider's prefix from the model it sends (upstream #249). A
// timeoutSecs or maxRetries of 0 is passed through as 0 retries / default timeout.
func New(apiKey, baseURL, modelHint string, timeoutSecs, maxRetries int) (*Native, error) {
	var base *string
	if baseURL != "" {
		base = &baseURL
	}
	var timeout *uint64
	if timeoutSecs > 0 {
		t := uint64(timeoutSecs)
		timeout = &t
	}
	retries := uint32(max(maxRetries, 0))
	var hint *string
	if modelHint != "" {
		hint = &modelHint
	}
	c, err := lit.CreateClient(apiKey, base, timeout, &retries, hint)
	if err != nil {
		return nil, err
	}
	return &Native{client: c}, nil
}

var errFreed = errors.New("liter-llm client is closed")

// Error is a typed liter-llm failure. The parent package reads it through its
// NativeError interface (variant, status, transient flag, retry delay) so it
// never interprets message text; errors.Is and errors.As still reach the
// binding's own *lit.Error and its sentinels through Unwrap.
type Error struct{ err *lit.Error }

func (e *Error) Error() string { return e.err.Message }
func (e *Error) Unwrap() error { return e.err }

// NativeVariant is the liter-llm error variant name, such as "RateLimited".
func (e *Error) NativeVariant() string { return e.err.Code }

// NativeStatus is the upstream HTTP status, or 0.
func (e *Error) NativeStatus() int { return int(e.err.StatusCode) }

// NativeTransient is liter-llm's own retry hint.
func (e *Error) NativeTransient() bool { return e.err.IsTransient }

// NativeRetryAfter is the provider's requested delay, or 0.
func (e *Error) NativeRetryAfter() time.Duration {
	if e.err.RetryAfter == nil {
		return 0
	}
	return time.Duration(*e.err.RetryAfter) * time.Millisecond
}

// wrap converts the binding's typed error; anything else passes through.
func wrap(err error) error {
	var le *lit.Error
	if errors.As(err, &le) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return &Error{err: le}
	}
	return err
}

// ChatJSON sends an OpenAI-shaped chat request and returns the response JSON.
// Ending ctx aborts the in-flight native request.
func (n *Native) ChatJSON(ctx context.Context, body []byte) ([]byte, error) {
	var req lit.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.client == nil {
		return nil, errFreed
	}
	resp, err := n.client.ChatWithContext(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	return json.Marshal(resp)
}

// EmbedJSON sends an OpenAI-shaped embeddings request and returns the response JSON.
func (n *Native) EmbedJSON(ctx context.Context, body []byte) ([]byte, error) {
	var req lit.EmbeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.client == nil {
		return nil, errFreed
	}
	resp, err := n.client.EmbedWithContext(ctx, req)
	if err != nil {
		return nil, wrap(err)
	}
	return json.Marshal(resp)
}

// Free releases the native client once in-flight calls finish.
func (n *Native) Free() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.client != nil {
		n.client.Free()
		n.client = nil
	}
}
