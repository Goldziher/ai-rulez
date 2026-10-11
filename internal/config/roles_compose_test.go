package config

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitRoleNames(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"empty":           {"", nil},
		"single":          {"dev", []string{"dev"}},
		"trimmed":         {"  dev  ", []string{"dev"}},
		"list":            {"dev,oncall", []string{"dev", "oncall"}},
		"spaces":          {"dev , oncall", []string{"dev", "oncall"}},
		"dedupe":          {"dev,oncall,dev", []string{"dev", "oncall"}},
		"drops empties":   {"dev,,oncall,", []string{"dev", "oncall"}},
		"whitespace only": {" , , ", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, SplitRoleNames(tc.in))
		})
	}
}

func TestCanonicalRoleNames(t *testing.T) {
	assert.Equal(t, "dev,oncall", CanonicalRoleNames([]string{"dev", "oncall", "dev"}))
	assert.Equal(t, "", CanonicalRoleNames(nil))
	assert.Equal(t, "dev", CanonicalRoleNames([]string{" dev "}))
}

func composedIDs(t *testing.T, c *Config, names ...string) []string {
	t.Helper()
	res, err := c.ResolveRoles(names)
	require.NoError(t, err)
	ids := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		ids = append(ids, it.Kind+":"+it.Domain+"/"+it.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestResolveRolesComposesUnionOfItems(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{
		{Name: "backend", Domains: []string{"backend"}},
		{Name: "frontend", Domains: []string{"frontend"}},
	}

	res, err := c.ResolveRoles([]string{"backend", "frontend"})
	require.NoError(t, err)
	assert.Equal(t, "backend,frontend", res.Name, "composition name is canonical")
	assert.Equal(t, []string{"backend", "frontend"}, res.Domains)

	assert.Equal(t, []string{
		"agent:backend/reviewer",
		"rule:/style",
		"rule:backend/db",
		"skill:/shared",
		"skill:backend/deploy",
		"skill:backend/deploy-prod",
		"skill:backend/migrate",
		"skill:frontend/ui",
	}, composedIDs(t, c, "backend", "frontend"))

	// A composition equals the union regardless of the order of its members.
	assert.Equal(t, composedIDs(t, c, "backend", "frontend"), composedIDs(t, c, "frontend", "backend"))
}

func TestResolveRolesSingleNameMatchesResolveRole(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{{
		Name: "dev", Domains: []string{"backend"},
		Skills:    &RoleSelector{Exclude: []string{"deploy*"}},
		SkillMode: map[string]string{"migrate": "name-only"},
	}}

	single, err := c.ResolveRole("dev")
	require.NoError(t, err)
	composed, err := c.ResolveRoles([]string{"dev"})
	require.NoError(t, err)

	assert.Equal(t, single.Name, composed.Name)
	assert.Equal(t, single.SkillOverrides, composed.SkillOverrides)
	assert.Equal(t, composedIDs(t, c, "dev"), composedIDs(t, c, "dev", "dev"), "a single name and its self-composition agree")
}

func TestResolveRolesLaterRoleWinsOnEqualSpecificity(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{
		{Name: "a", Domains: []string{"backend"}, SkillMode: map[string]string{"migrate": "off", "deploy": "off"}},
		{Name: "b", Domains: []string{"backend"}, SkillMode: map[string]string{"migrate": "name-only", "deploy": "on"}},
	}

	ab, err := c.ResolveRoles([]string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, "name-only", ab.SkillOverrides["migrate"], "the later-listed role wins a tie")
	assert.Equal(t, "on", ab.SkillOverrides["deploy"])

	ba, err := c.ResolveRoles([]string{"b", "a"})
	require.NoError(t, err)
	assert.Equal(t, "off", ba.SkillOverrides["migrate"], "reversing the list reverses the tie-break")
	assert.Equal(t, "off", ba.SkillOverrides["deploy"])
}

func TestResolveRolesSpecificityBeatsOrder(t *testing.T) {
	c := roleFixture()
	// a's glob is less specific than b's exact id, so a's later position does not
	// let it win.
	c.Roles = []RoleConfig{
		{Name: "glob", Domains: []string{"backend"}, SkillMode: map[string]string{"migrate*": "off"}},
		{Name: "exact", Domains: []string{"backend"}, SkillMode: map[string]string{"migrate": "name-only"}},
	}

	res, err := c.ResolveRoles([]string{"exact", "glob"})
	require.NoError(t, err)
	assert.Equal(t, "name-only", res.SkillOverrides["migrate"], "the exact id wins over a less specific glob")
}

func TestResolveRolesLaterRoleWinsDelivery(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{
		{Name: "a", Domains: []string{"backend"}, Delivery: map[string]string{"migrate": "served"}},
		{Name: "b", Domains: []string{"backend"}, Delivery: map[string]string{"migrate": "static"}},
	}

	ab, err := c.ResolveRoles([]string{"a", "b"})
	require.NoError(t, err)
	got, ok := ab.Flat().DeliveryFor("backend", "migrate")
	require.True(t, ok)
	assert.Equal(t, DeliveryStatic, got, "the later-listed role wins a delivery tie")

	ba, err := c.ResolveRoles([]string{"b", "a"})
	require.NoError(t, err)
	got, ok = ba.Flat().DeliveryFor("backend", "migrate")
	require.True(t, ok)
	assert.Equal(t, DeliveryServed, got, "reversing the list reverses the tie-break")
}

func TestResolveRolesUnknownRoleErrors(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{{Name: "dev", Domains: []string{"backend"}}}
	_, err := c.ResolveRoles([]string{"dev", "ghost"})
	require.Error(t, err)
}

func TestResolveRolesEmptyErrors(t *testing.T) {
	c := roleFixture()
	_, err := c.ResolveRoles(nil)
	require.Error(t, err)
}
