package roles

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCapabilitiesAreCited(t *testing.T) {
	valid := map[string]bool{ModeNameOnly: true, ModeUserInvocable: true, ModeOff: true}
	for _, c := range Capabilities() {
		assert.NotEmpty(t, c.Source, c.Preset)
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, c.Verified, c.Preset)
		for mode := range c.Modes {
			assert.True(t, valid[mode], "%s lists unknown mode %q", c.Preset, mode)
		}
		for mode, reason := range c.Documented {
			assert.True(t, valid[mode], "%s documents unknown mode %q", c.Preset, mode)
			assert.NotEmpty(t, reason)
			assert.NotContains(t, c.Modes, mode, "%s: a mode is rendered or documented-only, not both", c.Preset)
		}
	}
}

func TestHonours(t *testing.T) {
	tests := []struct {
		preset, mode string
		want         bool
	}{
		{"claude", ModeOff, true},
		{"claude", ModeNameOnly, true},
		{"cursor", ModeUserInvocable, true},
		{"cursor", ModeOff, false},
		{"codex", ModeUserInvocable, true},
		{"codex", ModeOff, false},
		{"copilot", ModeOff, true},
		{"opencode", ModeOff, false},
		{"unknown-harness", ModeUserInvocable, false},
		{"unknown-harness", ModeOn, true},
	}
	for _, tt := range tests {
		c, _ := CapabilityOf(tt.preset)
		_, got := c.Honours(tt.mode)
		assert.Equal(t, tt.want, got, "%s %s", tt.preset, tt.mode)
	}
}

func presetList(names ...string) []config.Preset {
	out := make([]config.Preset, len(names))
	for i, n := range names {
		out[i] = config.Preset{BuiltIn: n}
	}
	return out
}

func TestPlanSkillModes(t *testing.T) {
	tests := []struct {
		name         string
		presets      []string
		fallback     string
		mode         string
		wantAction   string
		wantDegraded []string
	}{
		{name: "claude only needs nothing more", presets: []string{"claude"}, mode: ModeOff},
		{name: "off with a harness lacking a setting is dropped", presets: []string{"claude", "cursor"}, mode: ModeOff, wantAction: ActionDrop, wantDegraded: []string{"cursor"}},
		{name: "off with the serve fallback is served", presets: []string{"claude", "cursor"}, fallback: "serve", mode: ModeOff, wantAction: ActionServe, wantDegraded: []string{"cursor"}},
		{name: "off where every harness has a setting is rendered", presets: []string{"claude", "copilot"}, mode: ModeOff, wantAction: ActionFrontmatter},
		{name: "user-invocable-only is rendered as frontmatter", presets: []string{"claude", "cursor", "codex"}, mode: ModeUserInvocable, wantAction: ActionFrontmatter},
		{name: "user-invocable-only is degraded where undocumented", presets: []string{"cursor", "opencode"}, mode: ModeUserInvocable, wantAction: ActionFrontmatter, wantDegraded: []string{"opencode"}},
		{name: "name-only is degraded off Claude", presets: []string{"claude", "cursor"}, mode: ModeNameOnly, wantDegraded: []string{"cursor"}},
		{name: "mcp is not a harness", presets: []string{"claude", "mcp"}, mode: ModeOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := fixtureConfig(t)
			cfg.Presets = presetList(tt.presets...)
			if tt.fallback != "" {
				cfg.RoleManifest = &config.RoleManifestConfig{SkillModeFallback: tt.fallback}
			}
			cfg.Roles[0].SkillMode = map[string]string{"migrate": tt.mode}
			res, err := cfg.ResolveRole("zeta")
			require.NoError(t, err)

			// Act
			got := PlanSkillModes(cfg, res)

			// Assert
			require.Len(t, got, 1)
			assert.Equal(t, "backend/migrate", got[0].Key())
			assert.Equal(t, tt.wantAction, got[0].Action)
			assert.Equal(t, tt.wantDegraded, got[0].Degraded)
		})
	}
}

func TestPlanSkillModesReportsFrontmatterOverrides(t *testing.T) {
	tests := []struct {
		name           string
		extra          map[string]string
		wantOverridden []string
		wantHonoured   bool
	}{
		{name: "no frontmatter keys", wantHonoured: true},
		{name: "same value", extra: map[string]string{"disable-model-invocation": "true"}, wantHonoured: true},
		{name: "author says false", extra: map[string]string{"disable-model-invocation": "false"}, wantOverridden: []string{"cursor"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := fixtureConfig(t)
			cfg.Presets = presetList("claude", "cursor")
			cfg.Content.Domains["backend"].Skills[0].Metadata.Extra = map[string]string{}
			for k, v := range tt.extra {
				cfg.Content.Domains["backend"].Skills[0].Metadata.Extra[k] = v
			}
			cfg.Roles[0].SkillMode = map[string]string{"migrate": ModeUserInvocable}
			res, err := cfg.ResolveRole("zeta")
			require.NoError(t, err)

			// Act
			got := PlanSkillModes(cfg, res)

			// Assert
			require.Len(t, got, 1)
			assert.Equal(t, tt.wantOverridden, got[0].Overridden)
			_, honoured := got[0].Honoured["cursor"]
			assert.Equal(t, tt.wantHonoured, honoured)
			_, viaSettings := got[0].Honoured["claude"]
			assert.True(t, viaSettings, "claude is honoured through settings whatever the frontmatter says")
		})
	}
}
