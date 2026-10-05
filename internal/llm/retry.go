package llm

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// RetryPolicy configures WithRetry.
type RetryPolicy struct {
	// Retries is the number of retries after the first attempt.
	Retries int
	// Base is the first backoff ceiling (default 500ms); it doubles per attempt up to Max (default 30s).
	Base, Max time.Duration
	// Sleep and Rand are injectable for tests.
	Sleep func(context.Context, time.Duration) error
	Rand  func() float64
}

func (p RetryPolicy) delay(attempt int, err error) time.Duration {
	base, maxD := p.Base, p.Max
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	if maxD <= 0 {
		maxD = 30 * time.Second
	}
	ceiling := min(base<<attempt, maxD)
	rnd := p.Rand
	if rnd == nil {
		rnd = rand.Float64
	}
	d := time.Duration(rnd() * float64(ceiling)) // full jitter
	var e *Error
	if errors.As(err, &e) && e.RetryAfter > d {
		d = min(e.RetryAfter, maxD)
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// WithRetry retries transient failures (rate limits, 5xx, transport errors) with
// exponential backoff and full jitter. Auth, context-length, budget and config
// errors are never retried.
func WithRetry(c Client, p RetryPolicy) Client {
	if p.Retries <= 0 {
		return c
	}
	if p.Sleep == nil {
		p.Sleep = sleepCtx
	}
	return &retryClient{next: c, p: p}
}

type retryClient struct {
	next Client
	p    RetryPolicy
}

func retry[T any](ctx context.Context, p RetryPolicy, call func() (T, error)) (T, error) {
	var zero T
	for attempt := 0; ; attempt++ {
		out, err := call()
		if err == nil {
			return out, nil
		}
		if attempt >= p.Retries || !IsTransient(err) || ctx.Err() != nil {
			return zero, err
		}
		if serr := p.Sleep(ctx, p.delay(attempt, err)); serr != nil {
			return zero, err
		}
	}
}

func (r *retryClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return retry(ctx, r.p, func() (ChatResponse, error) { return r.next.Chat(ctx, req) })
}

func (r *retryClient) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	return retry(ctx, r.p, func() (EmbedResponse, error) { return r.next.Embed(ctx, req) })
}

func (r *retryClient) Close() error { return r.next.Close() }
