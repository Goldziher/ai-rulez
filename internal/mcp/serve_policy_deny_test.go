package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
)

// orgPolicyContext is a context whose loads run under the organization policy
// file at path, as `--policy <path>` sets it.
func orgPolicyContext(t *testing.T, path string) context.Context {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "managed.toml")
	e := policy.NewEnforcer(func() policy.DiscoverOptions {
		return policy.DiscoverOptions{Flag: path, Env: ambient.MapEnv{Vars: map[string]string{}, Home: t.TempDir()}, ManagedPaths: []string{missing}}
	})
	return config.WithPolicyContext(context.Background(), e)
}

// rereadingPolicy is a policy enforcer that reads its files on every use, as
// an enforcer that notices an edited policy file does.
type rereadingPolicy struct{ opts func() policy.DiscoverOptions }

func (r rereadingPolicy) fresh() *policy.Enforcer { return policy.NewEnforcer(r.opts) }
func (r rereadingPolicy) Enforce(ctx context.Context, cfg *config.Config) (*config.PolicyOutcome, error) {
	return r.fresh().Enforce(ctx, cfg) //nolint:wrapcheck // test double
}
func (r rereadingPolicy) EnforceContent(ctx context.Context, cfg *config.Config) []config.PolicyViolation {
	return r.fresh().EnforceContent(ctx, cfg)
}
func (r rereadingPolicy) Locks(feature string) bool       { return r.fresh().Locks(feature) }
func (r rereadingPolicy) Load() (*policy.Resolved, error) { return r.fresh().Load() } //nolint:wrapcheck // test double

// The live reload watches the organization policy's files: a digest added to
// deny_digests while the server runs refuses that skill without a restart.
func TestWatch_ReloadsWhenThePolicyFileChanges(t *testing.T) {
	// Arrange
	root := project(t, baseConfig, map[string]string{
		"skills/core/SKILL.md":  skillFile("core", "Core conventions", ""),
		"skills/other/SKILL.md": skillFile("other", "Other skill", ""),
	})
	pin := &ServeSetup{WorkDir: root, CacheDir: filepath.Join(t.TempDir(), "cache")}
	_, served, _, err := pin.LockRecords(context.Background())
	require.NoError(t, err)
	require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), &lockfile.File{Version: lockfile.Version, Served: served}))
	pol := filepath.Join(t.TempDir(), "policy.toml")
	require.NoError(t, os.WriteFile(pol, []byte("policy_version = 1\nname = \"t\"\n"), 0o644))
	missing := filepath.Join(t.TempDir(), "managed.toml")
	home := t.TempDir()
	enforcer := rereadingPolicy{opts: func() policy.DiscoverOptions {
		return policy.DiscoverOptions{Flag: pol, Env: ambient.MapEnv{Vars: map[string]string{}, Home: home}, ManagedPaths: []string{missing}}
	}}
	ctx, cancel := context.WithCancel(config.WithPolicyContext(context.Background(), enforcer))
	defer cancel()
	setup := &ServeSetup{WorkDir: root, PollInterval: 10 * time.Millisecond, CacheDir: filepath.Join(t.TempDir(), "cache")}
	srv, err := setup.NewServer(ctx)
	require.NoError(t, err)
	_, ok := srv.Catalog().Lookup("core")
	require.True(t, ok)
	go srv.Watch(ctx)
	var coreDigest string
	for _, e := range served {
		if e.Name == "core" {
			coreDigest = e.Digest
		}
	}

	// Act
	require.NoError(t, os.WriteFile(pol, []byte("policy_version = 1\nname = \"t\"\n[sources]\ndeny_digests = [\""+coreDigest+"\"]\n"), 0o644))

	// Assert
	require.Eventually(t, func() bool {
		_, refused := srv.Catalog().Refusal("core")
		return refused
	}, 5*time.Second, 10*time.Millisecond, "the policy edit was not reloaded")
	_, ok = srv.Catalog().Lookup("other")
	assert.True(t, ok)
}

func TestTakeServedDenials_TakesOnlyViolationsNamingAServedPin(t *testing.T) {
	const digest = "sha256:aaaa"
	lock := &lockfile.File{Served: []lockfile.Entry{{Name: "core", Digest: digest}}}
	served := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: `served skill "core": ai-rulez.lock pins sha256:aaaa, which the policy denies (origin: flag)`,
		Subject: &config.PolicySubject{Kind: lockfile.KindServed, Name: "core", Digest: digest}}
	include := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: `include "shared": ai-rulez.lock pins sha256:aaaa, which the policy denies (origin: flag)`,
		Subject: &config.PolicySubject{Kind: lockfile.KindInclude, Name: "shared", Digest: digest}}
	otherName := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: `served skill "kit": ai-rulez.lock pins sha256:aaaa, which the policy denies (origin: flag)`,
		Subject: &config.PolicySubject{Kind: lockfile.KindServed, Name: "kit", Digest: digest}}
	otherView := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: served.Message,
		Subject: &config.PolicySubject{Kind: lockfile.KindServed, Name: "core", Domain: "role:dev", Digest: digest}}
	noSubject := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: served.Message}
	tests := []struct {
		name       string
		outcome    *config.PolicyOutcome
		wantDenied map[string]string
		wantKept   int
	}{
		{name: "served pin is taken", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{served}}, wantDenied: map[string]string{digest: served.Message}, wantKept: 0},
		{name: "include stays a load failure", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{served, include}}, wantDenied: map[string]string{digest: served.Message}, wantKept: 1},
		{name: "a name the lock does not pin is kept", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{otherName}}, wantKept: 1},
		{name: "a view the lock does not pin is kept", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{otherView}}, wantKept: 1},
		{name: "the message alone never matches", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{noSubject}}, wantKept: 1},
		{name: "no policy", outcome: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{PolicyOutcome: tt.outcome}

			// Act
			denied := takeServedDenials(cfg, lock)

			// Assert
			assert.Equal(t, tt.wantDenied, denied)
			if tt.outcome != nil {
				assert.Len(t, cfg.PolicyOutcome.Violations, tt.wantKept)
			}
		})
	}
}

// A policy's sources.deny_digests naming the pinned digest of one served skill
// refuses that skill (AR747); the server still starts and serves the others.
func TestServeSetup_PolicyDenyDigestRefusesOnlyTheMatchingSkill(t *testing.T) {
	tests := []struct {
		name        string
		deny        func(core, other string) []string
		wantRefused []string
		wantServed  []string
	}{
		{name: "one skill denied", deny: func(core, _ string) []string { return []string{core} }, wantRefused: []string{"core"}, wantServed: []string{"other"}},
		{name: "both denied", deny: func(core, other string) []string { return []string{core, other} }, wantRefused: []string{"core", "other"}},
		{name: "unrelated digest", deny: func(string, string) []string {
			return []string{"sha256:1111111111111111111111111111111111111111111111111111111111111111"}
		}, wantServed: []string{"core", "other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := project(t, baseConfig, map[string]string{
				"skills/core/SKILL.md":  skillFile("core", "Core conventions", ""),
				"skills/other/SKILL.md": skillFile("other", "Other skill", ""),
			})
			pin := &ServeSetup{WorkDir: root, CacheDir: filepath.Join(t.TempDir(), "cache")}
			_, served, _, err := pin.LockRecords(context.Background())
			require.NoError(t, err)
			require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), &lockfile.File{Version: lockfile.Version, Served: served}))
			digests := map[string]string{}
			for _, e := range served {
				digests[e.Name] = e.Digest
			}
			pol := filepath.Join(t.TempDir(), "policy.toml")
			text := "policy_version = 1\nname = \"t\"\n[sources]\ndeny_digests = ["
			for i, d := range tt.deny(digests["core"], digests["other"]) {
				if i > 0 {
					text += ", "
				}
				text += "\"" + d + "\""
			}
			require.NoError(t, os.WriteFile(pol, []byte(text+"]\n"), 0o644))

			// Act
			setup := &ServeSetup{WorkDir: root, NoWatch: true, CacheDir: filepath.Join(t.TempDir(), "cache")}
			srv, err := setup.NewServer(orgPolicyContext(t, pol))

			// Assert
			require.NoError(t, err, "a denied served skill must not stop the server")
			for _, name := range tt.wantRefused {
				r, refused := srv.Catalog().Refusal(name)
				require.True(t, refused, name)
				assert.Equal(t, lint.CodeDigestDenied, r.Code)
				assert.Contains(t, r.Reason, digests[name])
			}
			for _, name := range tt.wantServed {
				_, ok := srv.Catalog().Lookup(name)
				assert.True(t, ok, name)
			}
		})
	}
}
