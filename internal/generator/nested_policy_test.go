package generator

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// afterFirstLoad reports a policy violation on every configuration it sees
// after the first one: the root load is clean, so only a nested load (a
// monorepo member, the shared view, the drift baseline) can trip it.
type afterFirstLoad struct{ seen atomic.Int32 }

func (e *afterFirstLoad) Enforce(context.Context, *config.Config) (*config.PolicyOutcome, error) {
	out := &config.PolicyOutcome{}
	if e.seen.Add(1) > 1 {
		out.Violations = []config.PolicyViolation{{Code: "AR740", Key: "lock.enforce", Message: "loosened"}}
	}
	return out, nil
}

func (*afterFirstLoad) Locks(string) bool { return false }

func TestNestedLoadsAreHeldToThePolicy(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, enforcer *afterFirstLoad) error
	}{
		{"monorepo member", func(t *testing.T, enforcer *afterFirstLoad) error {
			root := t.TempDir()
			require.NoError(t, os.CopyFS(root, os.DirFS("../../tests/fixtures/plugin/monorepo")))
			cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal(), config.WithPolicy(enforcer))
			require.NoError(t, err)
			_, err = NewGenerator(cfg).collectPluginOutputs("")
			return err
		}},
		{"shared view for roles.json", func(t *testing.T, enforcer *afterFirstLoad) error {
			p := newDriftProject(t, driftIgnoring)
			p.overlay(t, "[profiles]\nonlylocal = []\n")
			cfg := p.load(t, config.WithPolicy(enforcer))
			_, err := NewGenerator(cfg).sharedConfig()
			return err
		}},
		{"drift baseline", func(t *testing.T, enforcer *afterFirstLoad) error {
			p := newDriftProject(t, driftIgnoring)
			p.overlay(t, "[profiles]\nonlylocal = []\n")
			cfg := p.load(t, config.WithPolicy(enforcer))
			_, _, err := NewGenerator(cfg).renderBaseline("")
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			enforcer := &afterFirstLoad{}

			// Act
			err := tt.run(t, enforcer)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "loosens the organization policy")
			assert.GreaterOrEqual(t, enforcer.seen.Load(), int32(2), "the nested load ran under the policy")
		})
	}
}
