package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// A response longer than maxResponseBytes used to be cut and then failed JSON
// decoding with a confusing message; it is now refused as too large.
func TestPostRefusesAnOversizedResponse(t *testing.T) {
	// Arrange
	chunk := []byte(strings.Repeat("x", 1<<20))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for range maxResponseBytes/len(chunk) + 1 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	o := &openAICompat{baseURL: srv.URL, http: srv.Client()}

	// Act
	_, err := o.post(context.Background(), "/chat/completions", []byte("{}"))

	// Assert
	var llmErr *Error
	if !errors.As(err, &llmErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if llmErr.Message != "response too large" || !errors.Is(llmErr.Cause, safefs.ErrTooLarge) {
		t.Fatalf("err = %+v, want a response-too-large provider error", llmErr)
	}
}
