package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestAdmit_ADeniedDigestIsNeverServed(t *testing.T) {
	t.Parallel()
	base := scanCatalog(t, func(s []generator.ServedSkill) { s[1].Files = s[1].Files[:1]; s[2].Files = s[2].Files[:1] })
	clean := mustSkill(t, base, "clean")
	lock := &lockfile.File{Version: lockfile.Version, Deny: []lockfile.Deny{{Digest: clean.LockDigest, Reason: "exfiltrates ~/.ssh"}}}
	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{"no governance at all", &config.Config{}},
		{"governance that does not select the skill", &config.Config{Governance: &config.GovernanceConfig{RequireApproval: []string{"kind:hook"}}}},
		{"governance that selects it, enforced", &config.Config{Governance: &config.GovernanceConfig{RequireApproval: []string{"kind:served"}, Enforce: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cat := base.Admit(Admission{Config: tt.cfg, Lock: lock})

			// Assert
			r, refused := cat.Refusal("clean")
			require.True(t, refused)
			assert.Equal(t, "AR717", r.Code)
			assert.Contains(t, r.Reason, "exfiltrates ~/.ssh")
		})
	}
	_, refused := base.Admit(Admission{Config: &config.Config{}, Lock: lock}).Refusal("preachy")
	assert.False(t, refused, "other digests are served")
}
