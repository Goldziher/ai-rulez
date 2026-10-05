package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// rolesConfig is a project with root, billing and docs skills and three roles.
func rolesConfig() *config.Config {
	skill := func(id string) config.ContentFile {
		return config.ContentFile{Name: id, Path: "/p/.ai-rulez/skills/" + id + "/SKILL.md"}
	}
	return &config.Config{
		ConfigDir: "/p/.ai-rulez",
		Content: &config.ContentTree{
			Skills: []config.ContentFile{skill("git-workflow")},
			Domains: map[string]*config.Domain{
				"billing": {Skills: []config.ContentFile{skill("refund-policy"), skill("invoice-format")}},
				"docs":    {Skills: []config.ContentFile{skill("pdf-forms")}},
			},
		},
		Roles: []config.RoleConfig{
			{Name: "billing-agent", Domains: []string{"billing"}, Skills: &config.RoleSelector{Exclude: []string{"invoice-*"}},
				Delivery: map[string]string{"refund-policy": "served"}},
			{Name: "docs-agent", Domains: []string{"docs"}},
			{Name: "broken", Extends: "missing"},
		},
	}
}

func TestRolesFromConfig_ResolvesTheRealRoleModel(t *testing.T) {
	t.Parallel()
	cfg := rolesConfig()
	resolve := RolesFromConfig(func() *config.Config { return cfg })

	scope, ok := resolve("billing-agent")
	require.True(t, ok)
	keeps := func(domain, name string) bool { return scope.Includes(&CatalogSkill{Name: name, Domain: domain}) }
	assert.True(t, keeps("", "git-workflow"), "root skills stay in every role")
	assert.True(t, keeps("billing", "refund-policy"))
	assert.False(t, keeps("billing", "invoice-format"), "excluded by the role's skills selector")
	assert.False(t, keeps("docs", "pdf-forms"), "a domain the role does not select")
	assert.True(t, keeps("", "from-a-source"), "a skill the project does not define (a source skill) is matched by name against the selectors")
	assert.Equal(t, "served", scope.Delivery["billing/refund-policy"])

	_, ok = resolve("nobody")
	assert.False(t, ok, "an unknown role is not resolved")
	_, ok = resolve("broken")
	assert.False(t, ok, "a role with broken inheritance is not resolved")
	_, ok = RolesFromConfig(func() *config.Config { return nil })("billing-agent")
	assert.False(t, ok)
}

func TestFindSkill_ScopedToARealRole(t *testing.T) {
	t.Parallel()
	cfg := rolesConfig()
	p, _ := startSkillServerWith(t, roleCatalog(t), ServeOptions{
		Role: "billing-agent", Roles: RolesFromConfig(func() *config.Config { return cfg }),
	})

	out, isErr, _ := callTool(t, p, "find_skill", map[string]any{"task": "git conventions customer invoices refund requests pdf forms"})
	require.False(t, isErr)
	assert.Equal(t, "billing-agent", out["role"], "the server's role is the default")
	inRole := map[string]bool{}
	for _, r := range out["results"].([]any) {
		m := r.(map[string]any)
		inRole[m["name"].(string)] = m["in_role"] == true
	}
	assert.True(t, inRole["refund-policy"])
	assert.True(t, inRole["git-workflow"])
	assert.False(t, inRole["invoice-format"], "excluded from the role")
	assert.False(t, inRole["pdf-forms"], "another role's domain")

	results := out["results"].([]any)
	assert.Equal(t, true, results[0].(map[string]any)["in_role"], "in-role skills are listed first")

	out, isErr, _ = callTool(t, p, "find_skill", map[string]any{"task": "pdf forms", "role": "docs-agent"})
	require.False(t, isErr)
	first := out["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "pdf-forms", first["name"])
	assert.Equal(t, true, first["in_role"], "the tool's role argument overrides the server's")

	_, isErr, text := callTool(t, p, "find_skill", map[string]any{"task": "x", "role": "broken"})
	assert.True(t, isErr)
	assert.Contains(t, text, "unknown role")
}

const rolesProjectConfig = baseConfig + `
[[roles]]
name = "billing-agent"
domains = ["billing"]
[roles.skills]
exclude = ["invoice-*"]
[roles.delivery]
"refund-policy" = "served"
`

func rolesProject(t *testing.T) string {
	t.Helper()
	return project(t, rolesProjectConfig, map[string]string{
		"skills/git-workflow/SKILL.md":                   skillFile("git-workflow", "Follow git conventions", ""),
		"domains/billing/skills/refund-policy/SKILL.md":  skillFile("refund-policy", "Process refund requests", ""),
		"domains/billing/skills/invoice-format/SKILL.md": skillFile("invoice-format", "Format invoices", ""),
		"domains/billing/rules/billing.md":               "# billing\n",
	})
}

func TestServeSetup_RoleServesOnlyWhatTheRoleDeliversServed(t *testing.T) {
	root := rolesProject(t)
	ctx := context.Background()

	full, err := (&ServeSetup{WorkDir: root, NoWatch: true}).build(ctx, buildOptions{admit: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"git-workflow", "invoice-format", "refund-policy"}, catalogNames(full.catalog),
		"no role and no project-wide delivery: every skill, as before")

	b, err := (&ServeSetup{WorkDir: root, NoWatch: true, Role: "billing-agent"}).build(ctx, buildOptions{admit: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"refund-policy"}, catalogNames(b.catalog),
		"the role delivers refund-policy as served; its other skills are static for it, and invoice-format is not its")

	b, err = (&ServeSetup{WorkDir: root, NoWatch: true, Role: "billing-agent", IncludeStatic: true}).build(ctx, buildOptions{admit: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"git-workflow", "refund-policy"}, catalogNames(b.catalog),
		"--include-static adds the role's static skills, still minus what the role drops")

	_, err = (&ServeSetup{WorkDir: root, NoWatch: true, Role: "nobody"}).build(ctx, buildOptions{admit: true})
	require.ErrorContains(t, err, "nobody")

	_, err = (&ServeSetup{WorkDir: root, NoWatch: true, Role: "billing-agent", Profile: "default"}).build(ctx, buildOptions{admit: true})
	require.ErrorContains(t, err, "mutually exclusive")
}

func TestServeSetup_RoleScopedFindSkillOverTheWire(t *testing.T) {
	root := rolesProject(t)
	srv := newServerFor(t, &ServeSetup{WorkDir: root, Role: "billing-agent"})
	assert.Equal(t, []string{"refund-policy"}, catalogNames(srv.Catalog()))

	p, _ := startSkillServerWith(t, srv.Catalog(), ServeOptions{Role: "billing-agent", Roles: srv.serve.opts.Roles})
	out, isErr, _ := callTool(t, p, "find_skill", map[string]any{"task": "refund"})
	require.False(t, isErr)
	assert.Equal(t, "billing-agent", out["role"])
	results := out["results"].([]any)
	require.Len(t, results, 1)
	assert.Equal(t, "refund-policy", results[0].(map[string]any)["name"])
	assert.Equal(t, true, results[0].(map[string]any)["in_role"])

	// The restriction is the server's --role: a skill outside it is not in the
	// catalog, so it cannot be loaded, listed or read by name (the find_skill role
	// argument alone only ranks).
	for _, name := range []string{"invoice-format", "git-workflow"} {
		_, isErr, _ := callTool(t, p, "load_skill", map[string]any{"name": name})
		assert.True(t, isErr, name)
		_, isErr, _ = callTool(t, p, "list_skill_resources", map[string]any{"name": name})
		assert.True(t, isErr, name)
	}
	_, isErr, _ = callTool(t, p, "load_skill", map[string]any{"name": "refund-policy"})
	assert.False(t, isErr)
}

func TestServeSetup_LockCoversSkillsAnyRoleServes(t *testing.T) {
	root := rolesProject(t)
	// The project-wide delivery makes the unscoped view serve only `served`
	// skills; the role still serves refund-policy, which the lock must pin.
	root2 := project(t, rolesProjectConfig+"\n[skills]\ndelivery = \"static\"\n", map[string]string{
		"skills/git-workflow/SKILL.md":                  skillFile("git-workflow", "Follow git conventions", ""),
		"domains/billing/skills/refund-policy/SKILL.md": skillFile("refund-policy", "Process refund requests", ""),
	})
	for _, dir := range []string{root, root2} {
		setup := &ServeSetup{Version: "test", WorkDir: dir, NoWatch: true}
		_, served, refused, err := setup.LockRecords(context.Background())
		require.NoError(t, err)
		require.Empty(t, refused)
		var names []string
		for i := range served {
			names = append(names, served[i].Name)
			assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, served[i].Digest)
		}
		assert.Contains(t, names, "refund-policy", "served by the billing-agent role")
	}
}
