package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestAdmit_GovernanceRefusesServedSkillsWithoutAValidApproval(t *testing.T) {
	t.Parallel()
	base := scanCatalog(t, func(s []generator.ServedSkill) { s[1].Files = s[1].Files[:1]; s[2].Files = s[2].Files[:1] })
	clean := mustSkill(t, base, "clean")
	cfg := func(enforce bool) *config.Config {
		return &config.Config{Governance: &config.GovernanceConfig{RequireApproval: []string{"kind:served"}, Enforce: enforce}}
	}
	lock := &lockfile.File{Version: lockfile.Version}
	approved := func(name, digest string) lockfile.Approval {
		return lockfile.Approval{Kind: "served", ID: name, Digest: digest, Reviewer: "alice", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-05T12:00:00Z"}
	}
	lock.Approval = []lockfile.Approval{
		approved("clean", clean.LockDigest),
		approved("installer", "sha256:old"), // approved at another digest: stale
	}

	tests := []struct {
		name     string
		skill    string
		cfg      *config.Config
		lock     *lockfile.File
		wantCode string
	}{
		{"approved skill is served", "clean", cfg(true), lock, ""},
		{"stale approval is refused", "installer", cfg(true), lock, "AR711"},
		{"no approval is refused", "preachy", cfg(true), lock, "AR710"},
		{"not enforced: served, only annotated", "preachy", cfg(false), lock, ""},
		{"no lock under enforce is refused", "preachy", cfg(true), nil, "AR710"},
		{"no lock, not enforced: served", "preachy", cfg(false), nil, ""},
		{"no governance: untouched", "preachy", &config.Config{}, lock, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cat := base.Admit(Admission{Config: tt.cfg, Lock: tt.lock})

			// Assert
			r, refused := cat.Refusal(tt.skill)
			if tt.wantCode == "" {
				assert.False(t, refused, "the skill is served")
				return
			}
			require.True(t, refused)
			assert.Equal(t, tt.wantCode, r.Code)
			assert.Contains(t, r.Reason, "served:"+tt.skill)
		})
	}

	// Provenance says who approved a served skill.
	cat := base.Admit(Admission{Config: cfg(true), Lock: lock})
	got := mustSkill(t, cat, "clean")
	assert.True(t, got.Approved)
	assert.Equal(t, []string{"alice"}, got.Approvers)
	p, _ := startSkillServerWith(t, cat, ServeOptions{})
	out, isErr, _ := callTool(t, p, "load_skill", map[string]any{"name": "clean"})
	require.False(t, isErr)
	prov := out["provenance"].(map[string]any)
	assert.Equal(t, true, prov["approved"])
	assert.Equal(t, []any{"alice"}, prov["approvers"])
}

func TestAdmit_RemoteSelectorCoversSkillsTheLockCallsRemote(t *testing.T) {
	t.Parallel()
	base := scanCatalog(t, func(s []generator.ServedSkill) {
		s[2].Source = "https://github.com/example/skills.git"
		s[1].Files = s[1].Files[:1]
		s[2].Files = s[2].Files[:1]
	})
	cfg := &config.Config{Governance: &config.GovernanceConfig{RequireApproval: []string{"remote"}, Enforce: true}}
	lock := &lockfile.File{Version: lockfile.Version}

	cat := base.Admit(Admission{Config: cfg, Lock: lock})

	_, localRefused := cat.Refusal("clean")
	assert.False(t, localRefused, "an authored skill is not remote")
	r, refused := cat.Refusal("preachy")
	require.True(t, refused)
	assert.Equal(t, "AR710", r.Code)
}
