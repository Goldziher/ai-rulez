package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// loosenBadEnforcer reports a policy violation for the configuration named "bad".
type loosenBadEnforcer struct{}

func (loosenBadEnforcer) Enforce(_ context.Context, cfg *config.Config) (*config.PolicyOutcome, error) {
	out := &config.PolicyOutcome{}
	if cfg.Name == "bad" {
		out.Violations = []config.PolicyViolation{{Code: "AR740", Key: "lock.enforce", Message: "loosened"}}
	}
	return out, nil
}

func (loosenBadEnforcer) Locks(string) bool { return false }

func installLoosenBad(t *testing.T) {
	t.Helper()
	previous := activePolicy
	activePolicy = loosenBadEnforcer{}
	t.Cleanup(func() { activePolicy = previous })
}

const policyBadConfig = "version = \"4.0\"\nname = \"bad\"\npresets = [\"claude\"]\ngitignore = false\n"

func TestRecursiveGenerateAndValidateRefusePolicyViolations(t *testing.T) {
	tests := []struct {
		name string
		run  func() int
	}{
		{"recursive generate", runRecursiveGenerate},
		{"recursive validate", runRecursiveValidate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := twoRoots(t, policyBadConfig)
			installLoosenBad(t)

			// Act
			code := tt.run()

			// Assert
			if code != 1 {
				t.Errorf("exit code = %d, want 1: the root that loosens the policy must fail", code)
			}
			if _, err := os.Stat(filepath.Join(root, "b", "CLAUDE.md")); err == nil {
				t.Error("the root that loosens the policy generated output")
			}
		})
	}
}
