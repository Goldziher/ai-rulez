package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

func TestTakeServedDenials_TakesOnlyViolationsNamingAServedPin(t *testing.T) {
	const digest = "sha256:aaaa"
	lock := &lockfile.File{Served: []lockfile.Entry{{Name: "core", Digest: digest}}}
	served := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: `served skill "core": ai-rulez.lock pins sha256:aaaa, which the policy denies (origin: flag)`}
	include := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: `include "shared": ai-rulez.lock pins sha256:aaaa, which the policy denies (origin: flag)`}
	otherName := config.PolicyViolation{Code: lint.CodeDigestDenied, Message: `served skill "kit": ai-rulez.lock pins sha256:aaaa, which the policy denies (origin: flag)`}
	tests := []struct {
		name       string
		outcome    *config.PolicyOutcome
		wantDenied map[string]string
		wantKept   int
	}{
		{name: "served pin is taken", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{served}}, wantDenied: map[string]string{digest: served.Message}, wantKept: 0},
		{name: "include stays a load failure", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{served, include}}, wantDenied: map[string]string{digest: served.Message}, wantKept: 1},
		{name: "a name the lock does not pin is kept", outcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{otherName}}, wantKept: 1},
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
