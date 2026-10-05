package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roleFixture() *Config {
	skill := func(id string, skills ...string) ContentFile {
		return ContentFile{Name: id, Path: "/p/.ai-rulez/skills/" + id + "/SKILL.md", Metadata: &Metadata{Skills: skills}}
	}
	rule := func(name string, skills ...string) ContentFile {
		return ContentFile{Name: name, Path: "/p/.ai-rulez/rules/" + name + ".md", Metadata: &Metadata{Skills: skills}}
	}
	return &Config{
		ConfigDir: "/p/.ai-rulez",
		Content: &ContentTree{
			Rules:  []ContentFile{rule("style")},
			Skills: []ContentFile{skill("shared")},
			Domains: map[string]*Domain{
				"backend": {
					Rules:  []ContentFile{rule("db", "migrate")},
					Skills: []ContentFile{skill("migrate"), skill("deploy"), skill("deploy-prod")},
					Agents: []ContentFile{{Name: "reviewer"}},
				},
				"frontend": {Skills: []ContentFile{skill("ui")}},
			},
		},
	}
}

func TestRolesParseFromTOML(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte(`
version = "4.0"
name = "x"
[role_manifest]
enabled = true
[lock]
enforce = true
scope = "skills"
[[roles]]
name = "dev"
domains = ["backend"]
[roles.skills]
exclude = ["deploy*"]
[roles.skill_mode]
"deploy*" = "off"
[roles.match]
groups = ["okta:dev"]
`), "config.toml")
	require.NoError(t, err)
	require.Len(t, cfg.Roles, 1)
	assert.Equal(t, []string{"deploy*"}, cfg.Roles[0].Skills.Exclude)
	assert.Equal(t, "off", cfg.Roles[0].SkillMode["deploy*"])
	assert.Equal(t, []string{"okta:dev"}, cfg.Roles[0].Match.Groups)
	assert.True(t, cfg.RoleManifest.Enabled)
	assert.True(t, cfg.LockEnforced())
	assert.Equal(t, LockScopeSkills, cfg.LockScope())

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	again, err := decodeConfigTOML(out, "config.toml")
	require.NoError(t, err)
	assert.Equal(t, cfg.Roles, again.Roles)
	assert.Equal(t, cfg.Lock, again.Lock)
}

func TestValidateRoles(t *testing.T) {
	cases := map[string][]RoleConfig{
		"bad name":  {{Name: "Bad Name"}},
		"duplicate": {{Name: "a"}, {Name: "a"}},
		"bad mode":  {{Name: "a", SkillMode: map[string]string{"x": "maybe"}}},
		"bad glob":  {{Name: "a", Skills: &RoleSelector{Include: []string{"[x"}}}},
	}
	for name, roles := range cases {
		t.Run(name, func(t *testing.T) {
			require.Error(t, (&Config{Roles: roles}).validateRoles())
		})
	}
	// extends problems only warn in Validate.
	require.NoError(t, (&Config{Roles: []RoleConfig{{Name: "a", Extends: "missing"}}}).validateRoles())
	require.Error(t, (&Config{Lock: &LockConfig{Scope: "nope"}}).validateLock())
}

func TestRoleInheritance(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{
		{Name: "base", Description: "base role", Domains: []string{"backend"},
			Skills:    &RoleSelector{Include: []string{"migrate", "deploy"}, Exclude: []string{"deploy-prod"}},
			SkillMode: map[string]string{"deploy": "off", "migrate": "name-only"},
			Match:     &RoleMatch{Groups: []string{"g1"}}},
		{Name: "child", Extends: "base", Domains: []string{"frontend"},
			Skills:    &RoleSelector{Include: []string{"ui"}, Exclude: []string{"shared"}},
			SkillMode: map[string]string{"deploy": "on"}},
	}
	flat, err := c.FlattenRole("child")
	require.NoError(t, err)
	assert.Equal(t, []string{"backend", "frontend"}, flat.Domains)
	assert.Equal(t, []string{"migrate", "deploy", "ui"}, flat.Skills.Include)
	assert.Equal(t, []string{"deploy-prod", "shared"}, flat.Skills.Exclude)
	assert.Equal(t, "on", flat.SkillMode["deploy"])
	assert.Equal(t, "name-only", flat.SkillMode["migrate"])
	assert.Equal(t, "base role", flat.Description)
	assert.Nil(t, flat.Match, "match hints are not inherited")

	// include only on one side keeps that side.
	c.Roles[1].Skills = &RoleSelector{Exclude: []string{"x"}}
	flat, err = c.FlattenRole("child")
	require.NoError(t, err)
	assert.Equal(t, []string{"migrate", "deploy"}, flat.Skills.Include)
}

func TestRoleExtendsProblems(t *testing.T) {
	c := &Config{Roles: []RoleConfig{
		{Name: "a", Extends: "b"}, {Name: "b", Extends: "a"},
		{Name: "c", Extends: "nope"},
		{Name: "d", Extends: "e"}, {Name: "e", Extends: "f"}, {Name: "f"},
		{Name: "self", Extends: "self"},
	}}
	problems := c.RoleProblems()
	roles := map[string]bool{}
	for _, p := range problems {
		assert.Equal(t, RoleProblemExtends, p.Kind)
		roles[p.Role] = true
	}
	for _, want := range []string{"a", "b", "c", "d", "self"} {
		assert.True(t, roles[want], want)
	}
	assert.False(t, roles["f"])
	_, err := c.FlattenRole("d")
	require.Error(t, err)
}

func TestSkillModePrecedence(t *testing.T) {
	r := &RoleConfig{SkillMode: map[string]string{
		"*":            "name-only",
		"deploy*":      "user-invocable-only",
		"deploy-prod":  "off",
		"backend/dep*": "on",
		"d*":           "on",
		"a?":           "off",
		"b?":           "on",
	}}
	mode := func(domain, id string) string { m, _ := r.SkillModeFor(domain, id); return m }
	assert.Equal(t, "off", mode("backend", "deploy-prod"), "exact id beats glob")
	assert.Equal(t, "on", mode("backend", "deploy"), "longer glob beats shorter")
	assert.Equal(t, "user-invocable-only", mode("", "deploy"), "domain glob does not apply to root")
	assert.Equal(t, "name-only", mode("", "other"))
	// tie: equal-length globs resolve to the lexically first pattern.
	tie := &RoleConfig{SkillMode: map[string]string{"b?": "on", "a?": "off"}}
	for range 20 {
		m, _ := tie.SkillModeFor("", "ab")
		assert.Equal(t, "off", m)
		m, _ = tie.SkillModeFor("", "ba")
		assert.Equal(t, "on", m)
	}
	tie = &RoleConfig{SkillMode: map[string]string{"x?": "on", "?x": "off"}}
	for range 20 {
		m, _ := tie.SkillModeFor("", "xx")
		assert.Equal(t, "off", m, "?x sorts before x?")
	}
	_, ok := (&RoleConfig{}).SkillModeFor("", "x")
	assert.False(t, ok)
}

func TestResolveRole(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{{
		Name: "dev", Domains: []string{"backend"},
		Skills:    &RoleSelector{Exclude: []string{"deploy*"}},
		SkillMode: map[string]string{"migrate": "name-only"},
	}}
	res, err := c.ResolveRole("dev")
	require.NoError(t, err)
	var ids []string
	for _, it := range res.Items {
		ids = append(ids, it.Kind+":"+it.Domain+"/"+it.ID)
	}
	assert.Equal(t, []string{
		"rule:/style", "rule:backend/db",
		"skill:/shared", "skill:backend/migrate",
		"agent:backend/reviewer",
	}, ids)
	assert.Equal(t, map[string]string{"migrate": "name-only"}, res.SkillOverrides)
	assert.Equal(t, "skills/migrate/SKILL.md", res.ItemsOf(RoleKindSkill)[1].Path)

	_, err = c.ResolveRole("ghost")
	require.Error(t, err)
}

func TestRoleProblems(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{
		{Name: "dev", Domains: []string{"backend", "ghost"},
			Skills:    &RoleSelector{Exclude: []string{"migrate"}, Include: []string{"nothing-*", "ui"}},
			SkillMode: map[string]string{"deploy": "off"}},
		{Name: "ok", Domains: []string{"backend"}},
	}
	byKind := map[string][]string{}
	for _, p := range c.RoleProblems() {
		byKind[p.Kind] = append(byKind[p.Kind], p.Message)
		assert.NotEqual(t, "ok", p.Role)
	}
	assert.Len(t, byKind[RoleProblemReference], 3, "ghost domain, nothing-*, ui outside the domains")
	require.Len(t, byKind[RoleProblemUnreachable], 1)
	assert.Contains(t, byKind[RoleProblemUnreachable][0], `uses skill "migrate"`)
}

func TestRoleUnreachableByMode(t *testing.T) {
	c := roleFixture()
	c.Roles = []RoleConfig{{Name: "dev", Domains: []string{"backend"}, SkillMode: map[string]string{"migrate": "off"}}}
	var got []string
	for _, p := range c.RoleProblems() {
		if p.Kind == RoleProblemUnreachable {
			got = append(got, p.Message)
		}
	}
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "hides it from the model")
}

func TestRolesOverlayMergesByName(t *testing.T) {
	shared := map[string]any{"roles": []any{
		map[string]any{"name": "dev", "description": "shared"},
		map[string]any{"name": "ops"},
	}}
	local := map[string]any{"roles": []any{
		map[string]any{"name": "dev", "description": "mine"},
		map[string]any{"name": "extra"},
		map[string]any{"name": "ops", "remove": true},
	}}
	merged, _, err := MergeConfigDocs(shared, local)
	require.NoError(t, err)
	roles, ok := merged["roles"].([]any)
	require.True(t, ok)
	var names []string
	for _, r := range roles {
		m, isMap := r.(map[string]any)
		require.True(t, isMap)
		names = append(names, m["name"].(string))
		if m["name"] == "dev" {
			assert.Equal(t, "mine", m["description"])
		}
	}
	assert.ElementsMatch(t, []string{"dev", "extra"}, names)
}

func TestRoleProblemsSameSkillIDWithDifferentModes(t *testing.T) {
	c := roleFixture()
	c.Content.Domains["frontend"].Skills = append(c.Content.Domains["frontend"].Skills,
		ContentFile{Name: "deploy", Path: "/p/.ai-rulez/domains/frontend/skills/deploy/SKILL.md"})
	c.Roles = []RoleConfig{
		{Name: "split", Domains: []string{"backend", "frontend"}, SkillMode: map[string]string{"backend/deploy": "off"}},
		{Name: "same", Domains: []string{"backend", "frontend"}, SkillMode: map[string]string{"deploy": "off"}},
		{Name: "one", Domains: []string{"backend"}, SkillMode: map[string]string{"backend/deploy": "off"}},
	}
	var got []RoleProblem
	for _, p := range c.RoleProblems() {
		if p.Kind == RoleProblemReference {
			got = append(got, p)
		}
	}
	require.Len(t, got, 1, "only the role whose two deploy skills resolve to different modes")
	assert.Equal(t, "split", got[0].Role)
	assert.Contains(t, got[0].Message, `id "deploy"`)
	assert.Contains(t, got[0].Message, "off in backend")
}
