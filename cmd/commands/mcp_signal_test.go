package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// RV-DYN-4: a signal ends the server through its context; shutdown must still
// run every closer (the usage sink flush) and must not report a failure.
func TestServeUntilDone_RunsClosersOnShutdown(t *testing.T) {
	boom := errors.New("transport broke")
	tests := []struct {
		name    string
		run     func(ctx context.Context) error
		cancel  bool
		wantErr error
	}{
		{name: "signal cancels the context", run: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, cancel: true},
		{name: "client closes stdin", run: func(context.Context) error { return nil }},
		{name: "transport error is reported", run: func(context.Context) error { return boom }, wantErr: boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			var closed []string

			// Act
			err := serveUntilDone(ctx, tt.run, func() { closed = append(closed, "telemetry") }, func() { closed = append(closed, "server") })

			// Assert
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			assert.Equal(t, []string{"telemetry", "server"}, closed)
		})
	}
}
