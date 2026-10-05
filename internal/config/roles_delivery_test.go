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

func TestRoleProblems_SkillSourceSkillsCannotBeCheckedOffline(t *testing.T) {
	cfg := roleFixture()
	cfg.SkillSources = []SkillSourceConfig{{Name: "vendor", URL: "/x"}}
	cfg.Roles = []RoleConfig{{Name: "dev", Domains: []string{"backend"},
		Skills:   &RoleSelector{Exclude: []string{"v-*"}},
		Delivery: map[string]string{"v-*": "served"}}}
	assert.Empty(t, cfg.RoleProblems(), "entries that may name a source's skills are not reported")

	cfg.SkillSources = nil
	assert.Len(t, cfg.RoleProblems(), 2, "without sources the same entries match nothing")
}

func TestTOMLRoundTripKeepsEveryTableOfTheMergedFeatures(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte(`
version = "4.0"
name = "x"
[skills]
delivery = "served"
[domains.billing]
delivery = "both"
[[skill_sources]]
name = "vendor"
url = "/x"
name_prefix = "v-"
[lock]
enforce = true
include_outputs = false
scope = "skills"
[role_manifest]
enabled = true
[[roles]]
name = "dev"
domains = ["billing"]
[roles.delivery]
"v-*" = "served"
`), "config.toml")
	require.NoError(t, err)
	assert.Equal(t, "served", cfg.Skills.Delivery)
	assert.Equal(t, "both", cfg.DomainSettings["billing"].Delivery)
	require.Len(t, cfg.SkillSources, 1)
	assert.True(t, cfg.LockEnforced())
	assert.False(t, cfg.LockIncludeOutputs())
	assert.Equal(t, LockScopeSkills, cfg.LockScope())
	assert.True(t, cfg.Roles[0].Delivery["v-*"] == "served")

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	again, err := decodeConfigTOML(out, "config.toml")
	require.NoError(t, err)
	assert.Equal(t, cfg.Skills, again.Skills)
	assert.Equal(t, cfg.DomainSettings, again.DomainSettings)
	assert.Equal(t, cfg.SkillSources, again.SkillSources)
	assert.Equal(t, cfg.Lock, again.Lock)
	assert.Equal(t, cfg.Roles, again.Roles)
	assert.Equal(t, cfg.RoleManifest, again.RoleManifest)
}

func TestRolesSelectChecksLikeAnyOtherKind(t *testing.T) {
	cfg := roleFixture()
	check := func(name string) ContentFile {
		return ContentFile{Name: name, Path: "/p/.ai-rulez/checks/" + name + ".md"}
	}
	cfg.Content.Checks = []ContentFile{check("security")}
	cfg.Content.Domains["backend"].Checks = []ContentFile{check("sql-injection"), check("n-plus-one")}
	cfg.Roles = []RoleConfig{{Name: "dev", Domains: []string{"backend"}, Checks: &RoleSelector{Exclude: []string{"n-plus-*"}}}}

	res, err := cfg.ResolveRole("dev")
	require.NoError(t, err)
	var ids []string
	for _, it := range res.ItemsOf(RoleKindCheck) {
		ids = append(ids, it.Domain+"/"+it.ID)
	}
	assert.Equal(t, []string{"/security", "backend/sql-injection"}, ids)

	tree, err := cfg.FilterTreeForRole(cfg.Content, res.Flat())
	require.NoError(t, err)
	assert.Len(t, tree.Checks, 1, "root checks reach the role's tree")
	assert.Len(t, tree.Domains["backend"].Checks, 1)
}
