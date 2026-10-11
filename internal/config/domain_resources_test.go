package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestDomainActive(t *testing.T) {
	tree := &ContentTree{Domains: map[string]*Domain{"billing": {Name: "billing"}}}

	tests := []struct {
		name   string
		domain string
		tree   *ContentTree
		want   bool
	}{
		{name: "an untagged resource is global", domain: "", tree: tree, want: true},
		{name: "an untagged resource is global without a tree", domain: "", tree: nil, want: true},
		{name: "a tagged resource is active when the tree carries the domain", domain: "billing", tree: tree, want: true},
		{name: "a tagged resource is inactive when the tree does not carry the domain", domain: "ui", tree: tree, want: false},
		{name: "a tagged resource is inactive without a tree", domain: "billing", tree: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DomainActive(tt.domain, tt.tree))
		})
	}
}

func TestResolveInstalledSkillsPlacesDomainTaggedSkill(t *testing.T) {
	tests := []struct {
		name         string
		domain       string
		wantInDomain string
		wantInRoot   bool
		wantWarn     string
	}{
		{name: "an untagged skill stays in the root content", wantInRoot: true},
		{name: "a tagged skill lands in its domain", domain: "billing", wantInDomain: "billing"},
		{
			name:       "an unknown domain falls back to the root content with a warning",
			domain:     "missing",
			wantInRoot: true,
			wantWarn:   "does not exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rec := &testutil.LogRecorder{}
			cfg := &Config{
				Host:            ambient.Host{Log: rec},
				InstalledSkills: []InstalledSkillConfig{{Name: "refunds", Source: "x", Domain: tt.domain}},
				Content: &ContentTree{
					Domains: map[string]*Domain{"billing": {Name: "billing"}},
				},
			}
			resolve := func(context.Context, *Config) ([]ContentFile, error) {
				return []ContentFile{{Name: "refunds", Content: "body"}}, nil
			}

			// Act
			err := resolveInstalledSkillsIfNeeded(logger.WithContext(context.Background(), rec), cfg, resolve)
			require.NoError(t, err)

			// Assert
			if tt.wantInDomain != "" {
				require.Contains(t, cfg.Content.Domains, tt.wantInDomain)
				assert.Equal(t, []string{"refunds"}, contentFileNames(cfg.Content.Domains[tt.wantInDomain].Skills))
			}
			if tt.wantInRoot {
				assert.Equal(t, []string{"refunds"}, contentFileNames(cfg.Content.Skills))
			} else {
				assert.Empty(t, cfg.Content.Skills)
			}
			if tt.wantWarn != "" {
				assert.Contains(t, rec.String(), tt.wantWarn)
			} else {
				assert.NotContains(t, rec.String(), "does not exist")
			}
		})
	}
}

func TestInstalledSkillDomainFollowsProfileSelection(t *testing.T) {
	tests := []struct {
		name        string
		profile     string
		wantSkillIn bool
	}{
		{name: "the selected profile reaches the skill's domain", profile: "billing-team", wantSkillIn: true},
		{name: "a profile that selects the domain but not the skill's scope drops it", profile: "billing-view", wantSkillIn: false},
		{name: "another profile does not", profile: "ui-team", wantSkillIn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: a domain-tagged installed skill scoped to the profile that
			// selects its domain.
			rec := &testutil.LogRecorder{}
			cfg := &Config{
				Host: ambient.Host{Log: rec},
				Profiles: map[string][]string{
					"billing-team": {"billing"},
					"billing-view": {"billing"},
					"ui-team":      {"ui"},
				},
				InstalledSkills: []InstalledSkillConfig{
					{Name: "refunds", Source: "x", Domain: "billing", Profiles: []string{"billing-team"}},
				},
				Content: &ContentTree{Domains: map[string]*Domain{
					"billing": {Name: "billing"},
					"ui":      {Name: "ui"},
				}},
			}
			resolve := func(context.Context, *Config) ([]ContentFile, error) {
				return []ContentFile{{Name: "refunds", Content: "body"}}, nil
			}
			require.NoError(t, resolveInstalledSkillsIfNeeded(context.Background(), cfg, resolve))

			// Act
			selected, err := cfg.SelectContentForProfile(cfg.Content, tt.profile)
			require.NoError(t, err)

			// Assert
			var got []string
			if d := selected.Domains["billing"]; d != nil {
				got = contentFileNames(d.Skills)
			}
			if tt.wantSkillIn {
				assert.Equal(t, []string{"refunds"}, got)
			} else {
				assert.NotContains(t, got, "refunds")
			}
		})
	}
}

// contentFileNames lists the names of the given content files in order.
func contentFileNames(files []ContentFile) []string {
	out := make([]string, 0, len(files))
	for i := range files {
		out = append(out, files[i].Name)
	}
	return out
}

func TestValidateWarnsUnknownResourceDomain(t *testing.T) {
	tests := []struct {
		name      string
		mcpDomain string
		skill     string
		source    string
		wantWarn  string
	}{
		{name: "an MCP server on a known domain is quiet", mcpDomain: "billing"},
		{name: "an MCP server on an unknown domain warns", mcpDomain: "ghost", wantWarn: "mcp server references non-existent domain"},
		{name: "an installed skill on an unknown domain warns", skill: "ghost", wantWarn: "installed skill references non-existent domain"},
		{name: "a skill source on an unknown domain warns", source: "ghost", wantWarn: "skill source references non-existent domain"},
		{name: "no domain set is quiet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rec := &testutil.LogRecorder{}
			cfg := &Config{
				Version: "5.0",
				Name:    "p",
				Presets: []Preset{{BuiltIn: "claude"}},
				Host:    ambient.Host{Log: rec},
				Content: &ContentTree{Domains: map[string]*Domain{"billing": {Name: "billing"}}},
				MCPServersRaw: []MCPServer{
					{Name: "srv", Command: "x", Domain: tt.mcpDomain},
				},
				InstalledSkills: []InstalledSkillConfig{
					{Name: "sk", Source: "x", Domain: tt.skill},
				},
				SkillSources: []SkillSourceConfig{
					{Name: "src", URL: "x", Domain: tt.source},
				},
			}
			cfg.MCPServers = serversToMap(cfg.MCPServersRaw)

			// Act
			err := cfg.Validate()
			require.NoError(t, err)

			// Assert
			if tt.wantWarn != "" {
				assert.Contains(t, rec.String(), tt.wantWarn)
			} else {
				assert.NotContains(t, rec.String(), "non-existent domain")
			}
		})
	}
}
