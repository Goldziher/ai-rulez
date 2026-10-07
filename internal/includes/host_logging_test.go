package includes

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestTokenWithheldWarningBelongsToTheRunThatRaisedIt(t *testing.T) {
	// Arrange: two runs in one process, each with its own logger and collector.
	const url = "https://git.example.net/acme/rules"
	recs := [2]*testutil.LogRecorder{{}, {}}
	cols := [2]*diag.Collector{diag.New(nil), diag.New(nil)}

	// Act: run 0 asks twice, run 1 once.
	for i, times := range []int{2, 1} {
		ctx := diag.WithContext(logger.WithContext(t.Context(), recs[i]), cols[i])
		ctx = ambient.WithContext(ctx, ambient.Host{Env: ambient.MapEnv{}, Log: recs[i]})
		for range times {
			withAuth(ctx, nil, url, "token")
		}
	}

	// Assert: each logger heard it once, whatever the other run did.
	for i, rec := range recs {
		got := rec.Level("WARN")
		assert.Len(t, got, 1, "run %d: %v", i, got)
		assert.Contains(t, rec.String(), "git.example.net")
	}
}
