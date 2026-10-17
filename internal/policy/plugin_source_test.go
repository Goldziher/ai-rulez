package policy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginresolve"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func loadWithPolicy(t *testing.T, project, policyBody string) (*config.Config, error) {
	t.Helper()
	testutil.IsolateHome(t)
	t.Setenv(lockfile.EnvAllowFileURLs, "1")
	policyFile := filepath.Join(t.TempDir(), "policy.toml")
	writePolicy(t, filepath.Dir(policyFile), "policy.toml", policyBody)
	enforcer := NewEnforcer(func() DiscoverOptions { return DiscoverOptions{Flag: policyFile, Env: ambient.MapEnv{}} })
	return config.LoadConfig(context.Background(), project,
		config.WithResolvers(pluginresolve.Resolvers("", nil)), config.WithoutLocal(), config.WithPolicy(enforcer))
}

// A marketplace names the repositories of its plugins only once its index is
// read, so a marketplace that itself passes the host rules can still point at a
// denied host. The rules must hold for every spelling of that host.
func TestMarketplaceNamedRepositoriesAreHeldToTheHostRules(t *testing.T) {
	const deny = "policy_version = 1\n[sources]\ndeny_hosts = [\"evil.example\", \"github.com/evil\"]\n"
	const allow = "policy_version = 1\n[sources]\nallowed_hosts = [\"github.com/acme\"]\n"
	tests := []struct {
		name    string
		policy  string
		url     string
		wantErr string
	}{
		{"denied host", deny, "https://evil.example/o/p.git", "denied by sources.deny_hosts"},
		{"denied host in upper case", deny, "https://EVIL.Example/o/p.git", "denied by sources.deny_hosts"},
		{"denied host with a trailing dot", deny, "https://evil.example./o/p.git", "denied by sources.deny_hosts"},
		{"denied host with a port", deny, "https://evil.example:8443/o/p.git", "denied by sources.deny_hosts"},
		{"denied scp address", deny, "git@evil.example:o/p.git", "denied by sources.deny_hosts"},
		{"denied ssh address", deny, "ssh://git@evil.example/o/p.git", "denied by sources.deny_hosts"},
		{"denied owner on a trusted host", deny, "https://github.com/evil/p.git", "denied by sources.deny_hosts"},
		{"owner outside the allow list", allow, "https://github.com/other/p.git", "not covered by sources.allowed_hosts"},
		{"look-alike owner outside the allow list", allow, "https://github.com/acme-evil/p.git", "not covered by sources.allowed_hosts"},
		{"host outside the allow list", allow, "https://git.example.net/acme/p.git", "not covered by sources.allowed_hosts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: a marketplace on disk whose only plugin lives at tt.url
			market := plugintest.NewRepo(t, plugintest.ClaudeMarketplace("acme", `{"name":"ext","source":{"source":"url","url":"`+tt.url+`"}}`))
			project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+market.URL()+"\"\n\n[[installed_plugins]]\nname = \"ext\"\nmarketplace = \"acme\"\n")

			// Act
			_, err := loadWithPolicy(t, project, tt.policy)

			// Assert
			require.Error(t, err)
			assert.ErrorIs(t, err, config.ErrPolicyLoosens)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), `installed plugin source "ext"`)
		})
	}
}

func TestMarketplaceNamedLocalRepositoriesPassTheHostRules(t *testing.T) {
	// Arrange: the plugin repository is a file:// path, which names no host
	plug := plugintest.NewRepo(t, plugintest.ClaudePlugin("ext"))
	market := plugintest.NewRepo(t, plugintest.ClaudeMarketplace("acme", `{"name":"ext","source":{"source":"url","url":"`+plug.URL()+`"}}`))
	project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+market.URL()+"\"\n\n[[installed_plugins]]\nname = \"ext\"\nmarketplace = \"acme\"\n")

	// Act
	cfg, err := loadWithPolicy(t, project, "policy_version = 1\n[sources]\nallowed_hosts = [\"github.com/acme\"]\ndeny_hosts = [\"evil.example\"]\n")

	// Assert
	require.NoError(t, err)
	var skills []string
	for _, s := range cfg.Content.Skills {
		skills = append(skills, s.Name)
	}
	assert.Equal(t, []string{"review-ext"}, skills)
}

func TestInstalledMarketplaceOnADeniedHostIsDroppedBeforeAnythingIsFetched(t *testing.T) {
	// Arrange
	project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \"https://evil.example/o/market.git\"\n")

	// Act
	cfg, err := loadWithPolicy(t, project, "policy_version = 1\n[sources]\ndeny_hosts = [\"evil.example\"]\n")

	// Assert
	require.NoError(t, err, "the entry is removed, so nothing tries to fetch it")
	assert.Empty(t, cfg.InstalledMarketplaces)
	require.NotNil(t, cfg.PolicyOutcome)
	require.Len(t, cfg.PolicyOutcome.Violations, 1)
	assert.Equal(t, "sources.deny_hosts", cfg.PolicyOutcome.Violations[0].Key)
	require.Error(t, config.CheckPolicy(cfg), "generation refuses a configuration the policy had to cut")
}
