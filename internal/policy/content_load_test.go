package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

func TestLoadBoundsTheHooksAnIncludeDelivers(t *testing.T) {
	// Arrange: a local include ships an agent whose frontmatter declares a hook; the policy forbids hooks
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write(".ai-rulez/config.toml", "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n[[includes]]\nname = \"shared\"\nsource = \"./shared\"\ninclude = [\"agents\"]\n")
	write("shared/.ai-rulez/agents/shared.md", "---\nname: shared\ndescription: shared agent\n"+hookFrontmatter+"---\nBody.\n")
	policyFile := filepath.Join(t.TempDir(), "policy.toml")
	require.NoError(t, os.WriteFile(policyFile, []byte("policy_version = 1\n[hooks]\nallow = false\n"), 0o644))
	enforcer := NewEnforcer(func() DiscoverOptions { return DiscoverOptions{Flag: policyFile, Env: ambient.MapEnv{}} })

	// Act
	cfg, err := config.LoadConfig(context.Background(), root, config.WithResolvers(includes.Resolvers("")), config.WithPolicy(enforcer))

	// Assert
	require.NoError(t, err)
	var agent *config.ContentFile
	for i := range cfg.Content.Agents {
		if cfg.Content.Agents[i].Name == "shared" {
			agent = &cfg.Content.Agents[i]
		}
	}
	require.NotNil(t, agent, "the include delivered the agent")
	_, hasHooks := agent.Metadata.TypedExtra("hooks")
	assert.False(t, hasHooks, "the policy unloaded the include's hooks")
	require.NotNil(t, cfg.PolicyOutcome)
	require.Len(t, cfg.PolicyOutcome.Violations, 1)
	assert.Equal(t, "AR748", cfg.PolicyOutcome.Violations[0].Code)
	assert.Equal(t, "hooks.allow", cfg.PolicyOutcome.Violations[0].Key)
	require.Error(t, config.CheckPolicy(cfg), "generation refuses a configuration whose include loosens the policy")
}
