package mcp

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestActiveDomainFilter(t *testing.T) {
	skill := func(id string) config.ContentFile {
		return config.ContentFile{Name: id, Path: "/p/.ai-rulez/skills/" + id + "/SKILL.md"}
	}
	baseContent := &config.ContentTree{
		Domains: map[string]*config.Domain{
			"billing": {Skills: []config.ContentFile{skill("refund-policy")}},
			"ui":      {Skills: []config.ContentFile{skill("theme")}},
		},
	}

	t.Run("a project with no skill sources accepts every domain", func(t *testing.T) {
		cfg := &config.Config{Content: baseContent}
		filter, err := (&ServeSetup{}).activeDomainFilter(cfg)
		require.NoError(t, err)
		assert.True(t, filter("billing"))
		assert.True(t, filter("ui"))
		assert.True(t, filter(""))
	})

	t.Run("a role narrows the active domains to the domains it keeps", func(t *testing.T) {
		cfg := &config.Config{
			Content: baseContent,
			Roles: []config.RoleConfig{
				{Name: "billing-agent", Domains: []string{"billing"}},
			},
			SkillSources: []config.SkillSourceConfig{{Name: "s", URL: "x"}},
		}
		filter, err := (&ServeSetup{Role: "billing-agent"}).activeDomainFilter(cfg)
		require.NoError(t, err)
		assert.True(t, filter("billing"))
		assert.False(t, filter("ui"))
		assert.True(t, filter(""), "an untagged source is global")
	})

	t.Run("a profile narrows the active domains to the domains it selects", func(t *testing.T) {
		cfg := &config.Config{
			Profiles:     map[string][]string{"backend": {"billing"}, "frontend": {"ui"}},
			Content:      baseContent,
			SkillSources: []config.SkillSourceConfig{{Name: "s", URL: "x"}},
		}
		filter, err := (&ServeSetup{Profile: "backend"}).activeDomainFilter(cfg)
		require.NoError(t, err)
		assert.True(t, filter("billing"))
		assert.False(t, filter("ui"))
		assert.True(t, filter(""))
	})

	t.Run("no profiles defined means every domain is active", func(t *testing.T) {
		cfg := &config.Config{
			Content:      baseContent,
			SkillSources: []config.SkillSourceConfig{{Name: "s", URL: "x"}},
		}
		filter, err := (&ServeSetup{}).activeDomainFilter(cfg)
		require.NoError(t, err)
		assert.True(t, filter("billing"))
		assert.True(t, filter("ui"))
	})
}

func TestServeSetup_SkillSourceFilteredByInactiveDomain(t *testing.T) {
	root := t.TempDir()
	configToml := fmt.Sprintf(`version = "5.0"
name = "p"
gitignore = false
presets = ["claude"]

[profiles]
backend = ["billing"]
frontend = ["ui"]

[[skill_sources]]
name = "billing-src"
url = "%s"
path = "skills"
domain = "billing"

[[skill_sources]]
name = "ui-src"
url = "%s"
path = "skills"
domain = "ui"
`, filepath.ToSlash(filepath.Join(root, "billing-skills")), filepath.ToSlash(filepath.Join(root, "ui-skills")))
	writeFile(t, root, ".ai-rulez/config.toml", configToml)
	// The domains must exist as project content for profile selection to reach them.
	writeFile(t, root, ".ai-rulez/domains/billing/rules/billing.md", "# billing\n")
	writeFile(t, root, ".ai-rulez/domains/ui/rules/ui.md", "# ui\n")
	writeFile(t, root, "billing-skills/skills/pdf/SKILL.md", skillFile("pdf", "Billing forms", ""))
	writeFile(t, root, "ui-skills/skills/theme/SKILL.md", skillFile("theme", "Theme tokens", ""))

	tests := []struct {
		name      string
		profile   string
		wantNames []string
	}{
		{name: "the backend profile reaches the billing domain", profile: "backend", wantNames: []string{"pdf"}},
		{name: "the frontend profile reaches the ui domain", profile: "frontend", wantNames: []string{"theme"}},
		{name: "the default fallback excludes both tagged sources", profile: "", wantNames: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServerFor(t, &ServeSetup{WorkDir: root, Profile: tt.profile})
			assert.Equal(t, tt.wantNames, catalogNames(srv.Catalog()))
		})
	}
}
