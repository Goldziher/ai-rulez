package commands

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestLooseningPolicyExitsTwo(t *testing.T) {
	cfg := &config.Config{PolicyOutcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{
		{Code: "AR740", File: ".ai-rulez/config.toml", Line: 4, Message: "below the floor"},
	}}}
	err := policyGate(cfg)
	if err == nil {
		t.Fatal("expected a loosening error")
	}
	if got := exitCodeFor(err); got != 2 {
		t.Fatalf("exitCodeFor = %d, want 2", got)
	}
}
