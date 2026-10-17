package policy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func sourceEnforcer(t *testing.T, body string) *Enforcer {
	t.Helper()
	flag := ""
	if body != "" {
		flag = writePolicy(t, t.TempDir(), "p.toml", body)
	}
	return NewEnforcer(func() DiscoverOptions {
		return DiscoverOptions{Flag: flag, Env: ambient.MapEnv{}, ManagedPaths: []string{filepath.Join(t.TempDir(), "none.toml")}}
	})
}

func TestEnforcerCheckSourceHostRules(t *testing.T) {
	const denyGitHub = "policy_version = 1\n[sources]\ndeny_hosts = [\"github.com/evil\", \"*.bad.example\"]\n"
	const allowOrg = "policy_version = 1\n[sources]\nallowed_hosts = [\"github.com/acme\", \"git.example.org\"]\n"
	tests := []struct {
		name    string
		policy  string
		source  string
		wantErr string
	}{
		{"no policy allows any source", "", "https://evil.example/x/y.git", ""},
		{"deny host path prefix is refused", denyGitHub, "https://github.com/evil/repo.git", "denied by sources.deny_hosts"},
		{"deny does not touch a sibling path", denyGitHub, "https://github.com/good/repo.git", ""},
		{"deny wildcard subdomain is refused", denyGitHub, "https://x.y.bad.example/o/r", "denied by sources.deny_hosts"},
		{"deny wildcard also covers the bare suffix", denyGitHub, "https://bad.example/o/r", "denied by sources.deny_hosts"},
		{"deny is case insensitive", denyGitHub, "HTTPS://GitHub.COM/Evil/Repo.git", "denied by sources.deny_hosts"},
		{"deny ignores the port", denyGitHub, "https://github.com:8443/evil/repo", "denied by sources.deny_hosts"},
		{"deny sees through credentials in the URL", denyGitHub, "https://good.example@github.com/evil/repo", "denied by sources.deny_hosts"},
		{"deny sees through user and password", denyGitHub, "https://user:pw@github.com/evil/repo", "denied by sources.deny_hosts"},
		{"deny sees through a git+ prefix", denyGitHub, "git+https://github.com/evil/repo", "denied by sources.deny_hosts"},
		{"deny sees the scp-like form", denyGitHub, "git@github.com:evil/repo.git", "denied by sources.deny_hosts"},
		{"deny sees ssh urls with a port", denyGitHub, "ssh://git@github.com:22/evil/repo.git", "denied by sources.deny_hosts"},
		{"deny sees a trailing-dot host", denyGitHub, "https://github.com./evil/repo", "denied by sources.deny_hosts"},
		{"deny ignores a trailing slash and .git", denyGitHub, "https://github.com/evil/repo.git/", "denied by sources.deny_hosts"},
		{"a local path names no host", denyGitHub, "./vendor/skills", ""},
		{"a file url names no host", denyGitHub, "file:///srv/evil/repo", ""},
		{"allow list admits a listed prefix", allowOrg, "https://github.com/acme/rules.git", ""},
		{"allow list admits a listed host", allowOrg, "https://git.example.org/any/where", ""},
		{"allow list refuses another owner", allowOrg, "https://github.com/other/rules.git", "not covered by sources.allowed_hosts"},
		{"allow list refuses a look-alike prefix", allowOrg, "https://github.com/acme-evil/rules.git", "not covered by sources.allowed_hosts"},
		{"allow list refuses a look-alike host", allowOrg, "https://git.example.org.evil.io/a/b", "not covered by sources.allowed_hosts"},
		{"allow list refuses a credential trick", allowOrg, "https://git.example.org@evil.io/a/b", "not covered by sources.allowed_hosts"},
		{"allow list refuses a host with only the owner missing", allowOrg, "https://github.com/", "not covered by sources.allowed_hosts"},
		{"allow list lets local paths through", allowOrg, "../shared", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			e := sourceEnforcer(t, tt.policy)
			cfg := &config.Config{}
			// Act
			err := e.CheckSource(cfg, "installed plugin source", "acme-plugin", tt.source)
			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, config.ErrPolicyLoosens)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), `installed plugin source "acme-plugin"`)
		})
	}
}

func TestEnforcerCheckSourceDenyBeatsAllow(t *testing.T) {
	// Arrange
	e := sourceEnforcer(t, "policy_version = 1\n[sources]\nallowed_hosts = [\"github.com\"]\ndeny_hosts = [\"github.com/evil\"]\n")
	// Act
	denied := e.CheckSource(&config.Config{}, "installed plugin", "p", "https://github.com/evil/repo")
	allowed := e.CheckSource(&config.Config{}, "installed plugin", "p", "https://github.com/acme/repo")
	// Assert
	require.Error(t, denied)
	assert.Contains(t, denied.Error(), "denied by sources.deny_hosts")
	require.NoError(t, allowed)
}

func TestEnforcerCheckSourceIsOpenWhenThePolicyBoundsNoSource(t *testing.T) {
	// Arrange
	e := sourceEnforcer(t, "policy_version = 1\n[telemetry]\nallow_network = false\n")
	// Act
	err := e.CheckSource(&config.Config{}, "installed plugin", "p", "https://anywhere.example/o/r")
	// Assert
	require.NoError(t, err)
}

func TestEnforcerCheckSourceLeavesAnUnloadablePolicyToEnforce(t *testing.T) {
	// Arrange
	e := NewEnforcer(func() DiscoverOptions {
		return DiscoverOptions{Flag: filepath.Join(t.TempDir(), "gone.toml"), Env: ambient.MapEnv{}}
	})
	cfg := &config.Config{}
	// Act
	checkErr := e.CheckSource(cfg, "installed plugin", "p", "https://evil.example/o/r")
	_, enforceErr := e.Enforce(context.Background(), cfg)
	// Assert
	require.NoError(t, checkErr, "CheckSource defers to Enforce, which fails closed")
	require.Error(t, enforceErr)
	assert.Contains(t, enforceErr.Error(), "AR742")
}

func TestEnforceDropsEverySourceKindThePolicyDenies(t *testing.T) {
	// Arrange
	e := sourceEnforcer(t, "policy_version = 1\n[sources]\ndeny_hosts = [\"evil.example\", \"github.com/evil\"]\n")
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.Includes = []config.IncludeConfig{
		{Name: "bad-inc", Source: "https://evil.example/o/inc"},
		{Name: "ok-inc", Source: "https://github.com/o/inc"},
	}
	cfg.InstalledSkills = []config.InstalledSkillConfig{
		{Name: "bad-skill", Source: "https://evil.example/o/skill"},
		{Name: "local-skill", Source: "./skills"},
	}
	cfg.SkillSources = []config.SkillSourceConfig{
		{Name: "bad-src", URL: "git@evil.example:o/skills.git"},
		{Name: "ok-src", URL: "https://github.com/o/skills"},
	}
	cfg.InstalledMarketplaces = []config.InstalledMarketplaceConfig{
		{Name: "bad-mp", Source: "https://evil.example/o/mp"},
		{Name: "shorthand-mp", Source: "evil/mp"},
		{Name: "ok-mp", Source: "github:acme/mp"},
	}
	cfg.InstalledPlugins = []config.InstalledPluginConfig{
		{Name: "bad-plugin", Source: "https://EVIL.example/o/plugin"},
		{Name: "bad-shorthand-plugin", Source: "github:evil/plugin"},
		{Name: "via-marketplace", Marketplace: "ok-mp"},
	}
	// Act
	out, err := e.Enforce(context.Background(), cfg)
	// Assert
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Len(t, out.Violations, 7)
	assert.Equal(t, []string{"ok-inc"}, includeNames(cfg.Includes))
	require.Len(t, cfg.InstalledSkills, 1)
	assert.Equal(t, "local-skill", cfg.InstalledSkills[0].Name)
	require.Len(t, cfg.SkillSources, 1)
	assert.Equal(t, "ok-src", cfg.SkillSources[0].Name)
	require.Len(t, cfg.InstalledMarketplaces, 1)
	assert.Equal(t, "ok-mp", cfg.InstalledMarketplaces[0].Name)
	require.Len(t, cfg.InstalledPlugins, 1)
	assert.Equal(t, "via-marketplace", cfg.InstalledPlugins[0].Name)
}

func includeNames(in []config.IncludeConfig) []string {
	var out []string
	for i := range in {
		out = append(out, in[i].Name)
	}
	return out
}
