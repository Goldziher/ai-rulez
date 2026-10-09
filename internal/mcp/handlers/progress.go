package handlers

import "context"

// ProgressFunc reports how far a long tool call has come.
type ProgressFunc func(progress, total float64, message string)

type progressKey struct{}

// WithProgress returns a context whose Progress calls reach fn. The server sets
// it for calls whose client asked for progress notifications.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// Progress reports progress of the running tool call; it does nothing when the
// client did not ask for notifications.
func Progress(ctx context.Context, progress, total float64, message string) {
	if fn, ok := ctx.Value(progressKey{}).(ProgressFunc); ok {
		fn(progress, total, message)
	}
}
