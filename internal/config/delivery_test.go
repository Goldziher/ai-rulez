package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skillWith(name, delivery string) ContentFile {
	extra := map[string]string{"description": "d " + name}
	if delivery != "" {
		extra["delivery"] = delivery
	}
	return ContentFile{Name: name, Path: "/x/skills/" + name + "/SKILL.md", Content: "body", Metadata: &Metadata{Extra: extra}}
}

func TestEffectiveDelivery_Precedence(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Skills:         &SkillsConfig{Delivery: "served"},
		DomainSettings: map[string]DomainConfig{"billing": {Delivery: "both"}, "docs": {Delivery: "static"}},
	}
	tests := []struct {
		name     string
		skill    ContentFile
		domain   string
		override map[string]string
		want     Delivery
	}{
		{"global default", skillWith("a", ""), "", nil, DeliveryServed},
		{"domain beats global", skillWith("a", ""), "billing", nil, DeliveryBoth},
		{"domain static beats global served", skillWith("a", ""), "docs", nil, DeliveryStatic},
		{"unlisted domain falls to global", skillWith("a", ""), "other", nil, DeliveryServed},
		{"frontmatter beats domain", skillWith("a", "static"), "billing", nil, DeliveryStatic},
		{"role by domain beats domain config", skillWith("a", ""), "billing", map[string]string{"billing": "static"}, DeliveryStatic},
		{"role by skill beats role by domain", skillWith("a", ""), "billing", map[string]string{"billing": "static", "a": "served"}, DeliveryServed},
		{"frontmatter beats role", skillWith("a", "both"), "billing", map[string]string{"a": "served"}, DeliveryBoth},
		{"invalid frontmatter is skipped", skillWith("a", "bogus"), "billing", nil, DeliveryBoth},
		{"invalid role value is skipped", skillWith("a", ""), "", map[string]string{"a": "nope"}, DeliveryServed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cfg.EffectiveDelivery(tt.skill, tt.domain, tt.override))
		})
	}
	assert.Equal(t, DeliveryStatic, (&Config{}).EffectiveDelivery(skillWith("a", ""), "", nil), "nothing set means static")
}

func treeWithSkills() *ContentTree {
	return &ContentTree{
		Skills: []ContentFile{skillWith("core", "static"), skillWith("heavy", "served")},
		Domains: map[string]*Domain{
			"billing": {Name: "billing", Skills: []ContentFile{skillWith("refunds", ""), skillWith("invoices", "both")}},
		},
	}
}

func skillNames(tree *ContentTree) []string {
	var out []string
	for _, s := range tree.Skills {
		out = append(out, SkillID(s))
	}
	for _, d := range tree.Domains {
		for _, s := range d.Skills {
			out = append(out, SkillID(s))
		}
	}
	return out
}

func TestContentForPreset_ExcludesServedAndAddsStubOnce(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DomainSettings: map[string]DomainConfig{"billing": {Delivery: "served"}},
		Content:        treeWithSkills(),
	}
	for _, preset := range []string{"claude", "cursor", "codex", "gemini", "opencode", "copilot"} {
		got := skillNames(cfg.ContentForPreset(preset))
		assert.ElementsMatch(t, []string{"core", "invoices", DynamicSkillsName}, got, "preset %s", preset)
	}
	assert.Len(t, cfg.Content.Skills, 2, "the source tree must not be modified")
	assert.Len(t, cfg.Content.Domains["billing"].Skills, 2)

	// A second pass over the same config still has exactly one stub.
	again := cfg.ContentForPreset("claude")
	stubs := 0
	for _, n := range skillNames(again) {
		if n == DynamicSkillsName {
			stubs++
		}
	}
	assert.Equal(t, 1, stubs)
}

func TestContentForPreset_AuthoredStubWins(t *testing.T) {
	t.Parallel()
	cfg := &Config{Content: &ContentTree{Skills: []ContentFile{skillWith(DynamicSkillsName, "static"), skillWith("x", "served")}}}
	got := skillNames(cfg.ContentForPreset("claude"))
	assert.Equal(t, []string{DynamicSkillsName}, got)
	assert.Contains(t, cfg.ContentForPreset("claude").Skills[0].Content, "body", "the authored skill is kept, not replaced")
}

func TestContentForPreset_NoMCPHarnessFallsBackToStatic(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Presets: []Preset{{BuiltIn: "claude"}, {BuiltIn: "cline"}},
		Content: treeWithSkills(),
	}
	got := skillNames(cfg.ContentForPreset("cline"))
	assert.ElementsMatch(t, []string{"core", "heavy", "refunds", "invoices"}, got, "served skills stay static where MCP is unavailable, and no stub is added")

	fallbacks := cfg.DeliveryFallbacks(cfg.Content)
	require.Len(t, fallbacks, 1)
	assert.Equal(t, "cline", fallbacks[0].Preset)
	assert.Equal(t, []string{"heavy"}, fallbacks[0].Skills)
}

func TestContentForPreset_NothingServedIsIdentity(t *testing.T) {
	t.Parallel()
	tree := &ContentTree{Skills: []ContentFile{skillWith("a", ""), skillWith("b", "both")}}
	cfg := &Config{Content: tree}
	got := cfg.ContentForPreset("claude")
	assert.Equal(t, []string{"a", "b", DynamicSkillsName}, skillNames(got), "both still needs the stub so the agent can search")
	empty := &ContentTree{Skills: []ContentFile{skillWith("a", "")}}
	assert.Same(t, empty, (&Config{Content: empty}).ContentForPreset("claude"), "no served skills: the tree is untouched")
}

func TestContentForPreset_ServeModeRendersEverything(t *testing.T) {
	t.Parallel()
	cfg := &Config{Content: treeWithSkills(), ServeMode: true}
	assert.Same(t, cfg.Content, cfg.ContentForPreset("claude"))
}

func TestDecodeDynamicConfig(t *testing.T) {
	t.Parallel()
	cfg, err := decodeConfigTOML([]byte(`
version = "3.0"
name = "x"
presets = ["claude"]

[skills]
delivery = "served"

[domains.billing]
delivery = "both"

[lock]
enforce = true

[[skill_sources]]
name = "team"
url = "git+https://example.com/org/skills"
ref = "v1.2.0"
path = "skills"
include = ["pdf-*"]
exclude = ["*-wip"]
name_prefix = "team-"
trust = "warn"
`), "config.toml")
	require.NoError(t, err)
	assert.Equal(t, "served", cfg.Skills.Delivery)
	assert.Equal(t, "both", cfg.DomainSettings["billing"].Delivery)
	assert.True(t, cfg.Lock.Enforce)
	require.Len(t, cfg.SkillSources, 1)
	assert.Equal(t, SkillSourceConfig{
		Name: "team", URL: "git+https://example.com/org/skills", Ref: "v1.2.0", Path: "skills",
		Include: []string{"pdf-*"}, Exclude: []string{"*-wip"}, NamePrefix: "team-", Trust: "warn",
	}, cfg.SkillSources[0])
	require.NoError(t, cfg.validateDynamicSkills())
}

func TestValidateDynamicSkills(t *testing.T) {
	t.Parallel()
	good := SkillSourceConfig{Name: "s", URL: "/tmp/x"}
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"bad global", Config{Skills: &SkillsConfig{Delivery: "lazy"}}, "skills.delivery"},
		{"bad domain", Config{DomainSettings: map[string]DomainConfig{"d": {Delivery: "x"}}}, "domain"},
		{"no name", Config{SkillSources: []SkillSourceConfig{{URL: "u"}}}, "invalid name"},
		{"no url", Config{SkillSources: []SkillSourceConfig{{Name: "s"}}}, "'url'"},
		{"dup", Config{SkillSources: []SkillSourceConfig{good, good}}, "duplicate"},
		{"bad trust", Config{SkillSources: []SkillSourceConfig{{Name: "s", URL: "u", Trust: "off"}}}, "trust"},
		{"bad prefix", Config{SkillSources: []SkillSourceConfig{{Name: "s", URL: "u", NamePrefix: "a/b"}}}, "name_prefix"},
		{"escaping path", Config{SkillSources: []SkillSourceConfig{{Name: "s", URL: "u", Path: "../x"}}}, "escapes"},
		{"bad glob", Config{SkillSources: []SkillSourceConfig{{Name: "s", URL: "u", Include: []string{"["}}}}, "glob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.validateDynamicSkills()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestSkillSourceConfigValidate_RejectsGitOptions(t *testing.T) {
	t.Parallel()
	for _, s := range []SkillSourceConfig{
		{Name: "s", URL: "--upload-pack=touch /tmp/x;@h:p"},
		{Name: "s", URL: "git+--upload-pack=x;@h:p"},
		{Name: "s", URL: "https://h/r.git", Ref: "--upload-pack=x"},
	} {
		assert.Error(t, s.Validate(0), "%+v", s)
	}
}
