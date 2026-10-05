package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleDelivery_ParsesValidatesAndInherits(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte(`
version = "4.0"
name = "x"
[[roles]]
name = "base"
domains = ["backend"]
[roles.delivery]
"deploy*" = "served"
"backend/migrate" = "both"
[[roles]]
name = "dev"
extends = "base"
[roles.delivery]
"deploy-prod" = "static"
`), "config.toml")
	require.NoError(t, err)

	flat, err := cfg.FlattenRole("dev")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"deploy*": "served", "backend/migrate": "both", "deploy-prod": "static"}, flat.Delivery,
		"the child adds to the parent's delivery and wins per key")

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	again, err := decodeConfigTOML(out, "config.toml")
	require.NoError(t, err)
	assert.Equal(t, cfg.Roles, again.Roles)

	require.Error(t, (&Config{Roles: []RoleConfig{{Name: "a", Delivery: map[string]string{"x": "maybe"}}}}).validateRoles())
	require.Error(t, (&Config{Roles: []RoleConfig{{Name: "a", Delivery: map[string]string{"[x": "served"}}}}).validateRoles())
}

func TestRoleDelivery_MostSpecificSelectorWins(t *testing.T) {
	role := &RoleConfig{Delivery: map[string]string{"deploy*": "served", "deploy-prod": "static", "backend/*": "both"}}
	tests := []struct {
		domain, id string
		want       Delivery
		set        bool
	}{
		{"backend", "deploy", DeliveryBoth, true}, // the longer glob is the more specific
		{"frontend", "deploy", DeliveryServed, true},
		{"backend", "deploy-prod", DeliveryStatic, true},
		{"backend", "migrate", DeliveryBoth, true},
		{"frontend", "ui", "", false},
	}
	for _, tt := range tests {
		got, ok := role.DeliveryFor(tt.domain, tt.id)
		assert.Equal(t, tt.set, ok, tt.id)
		assert.Equal(t, tt.want, got, tt.id)
	}
}

func TestRoleDelivery_DrivesEffectiveDeliveryAndResolvedItems(t *testing.T) {
	cfg := roleFixture()
	cfg.Roles = []RoleConfig{
		{Name: "dev", Domains: []string{"backend"}, Delivery: map[string]string{"deploy*": "served", "migrate": "both"}},
		{Name: "plain", Domains: []string{"backend"}},
	}
	cfg.Skills = &SkillsConfig{Delivery: string(DeliveryStatic)}

	res, err := cfg.ResolveRole("dev")
	require.NoError(t, err)
	got := map[string]string{}
	for _, it := range res.ItemsOf(RoleKindSkill) {
		got[it.ID] = it.Delivery
	}
	assert.Equal(t, map[string]string{"shared": "static", "migrate": "both", "deploy": "served", "deploy-prod": "served"}, got)
	assert.Equal(t, "served", res.DeliveryOverride["backend/deploy"])

	plain, err := cfg.ResolveRole("plain")
	require.NoError(t, err)
	for _, it := range plain.ItemsOf(RoleKindSkill) {
		assert.Equal(t, "static", it.Delivery, "a role that sets no delivery leaves the defaults alone: %s", it.ID)
	}
	assert.True(t, cfg.RolesServeSkills())
	assert.False(t, cfg.DeliveryConfigured(cfg.Content), "no role is active, so the project default is unchanged")
}

func TestRoleDelivery_ActiveRoleSplitsTheStaticTrees(t *testing.T) {
	cfg := roleFixture()
	cfg.Roles = []RoleConfig{{Name: "dev", Domains: []string{"backend"}, Delivery: map[string]string{"deploy*": "served"}}}
	res, err := cfg.ResolveRole("dev")
	require.NoError(t, err)

	forRole := *cfg
	forRole.SetRoleDelivery(res.DeliveryOverride)
	tree, err := forRole.FilterTreeForRole(forRole.Content, res.Flat())
	require.NoError(t, err)
	forRole.Content = tree
	static := forRole.ContentForPreset("claude")

	var ids []string
	for _, s := range static.Domains["backend"].Skills {
		ids = append(ids, SkillID(s))
	}
	assert.Equal(t, []string{"migrate"}, ids, "served skills are left out of the role's static tree")
	stub := false
	for _, s := range static.Skills {
		stub = stub || SkillID(s) == DynamicSkillsName
	}
	assert.True(t, stub, "the dynamic-skills stub is added for the role")

	assert.Len(t, cfg.ContentForPreset("claude").Domains["backend"].Skills, 3, "the shared config is untouched")
}

func TestRoleProblems_ReportsADeliveryEntryThatMatchesNothing(t *testing.T) {
	cfg := roleFixture()
	cfg.Roles = []RoleConfig{{Name: "dev", Domains: []string{"backend"}, Delivery: map[string]string{"nope": "served"}}}
	var found bool
	for _, p := range cfg.RoleProblems() {
		found = found || (p.Kind == RoleProblemReference && assert.Contains(t, p.Message, "delivery"))
	}
	assert.True(t, found)
}
