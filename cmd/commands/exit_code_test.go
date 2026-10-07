package commands

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestExitCodeForRoleReference(t *testing.T) {
	if got := exitCodeFor(fmt.Errorf("resolve role: %w", config.ErrRoleReference)); got != 2 {
		t.Fatalf("AR971 exit code = %d, want 2", got)
	}
	if got := exitCodeFor(errors.New("boom")); got != 1 {
		t.Fatalf("other errors exit 1, got %d", got)
	}
}
