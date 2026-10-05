package evals

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

func mustCounter(t *testing.T) tokens.Counter {
	t.Helper()
	c, err := tokens.New("")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
