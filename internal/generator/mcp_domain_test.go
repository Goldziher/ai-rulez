package generator

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCollectMCPServersByDomain(t *testing.T) {
	tests := []struct {
		name     string
		server   config.MCPServer
		profile  string
		tree     *config.ContentTree
		wantKeys []string
	}{
		{
			name:     "a domain-tagged server is included when the tree carries the domain",
			server:   config.MCPServer{Name: "billing-db", Domain: "billing"},
			tree:     &config.ContentTree{Domains: map[string]*config.Domain{"billing": {Name: "billing"}}},
			wantKeys: []string{"billing-db"},
		},
		{
			name:     "a domain-tagged server is excluded when the tree does not carry the domain",
			server:   config.MCPServer{Name: "billing-db", Domain: "billing"},
			tree:     &config.ContentTree{Domains: map[string]*config.Domain{"ui": {Name: "ui"}}},
			wantKeys: nil,
		},
		{
			name:     "an untagged server is included regardless of the domains",
			server:   config.MCPServer{Name: "global"},
			tree:     &config.ContentTree{Domains: map[string]*config.Domain{}},
			wantKeys: []string{"global"},
		},
		{
			name:     "both domain and profiles must pass",
			server:   config.MCPServer{Name: "billing-db", Domain: "billing", Profiles: []string{"backend"}},
			profile:  "backend",
			tree:     &config.ContentTree{Domains: map[string]*config.Domain{"billing": {Name: "billing"}}},
			wantKeys: []string{"billing-db"},
		},
		{
			name:     "a matching domain under a non-matching profile is excluded",
			server:   config.MCPServer{Name: "billing-db", Domain: "billing", Profiles: []string{"backend"}},
			profile:  "frontend",
			tree:     &config.ContentTree{Domains: map[string]*config.Domain{"billing": {Name: "billing"}}},
			wantKeys: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			server := tt.server
			cfg := &config.Config{
				MCPServers: map[string]*config.MCPServer{server.Name: &server},
			}
			gen := &Generator{config: cfg}

			// Act
			got := gen.collectMCPServersForContent(tt.tree, tt.profile)

			// Assert
			assert.Equal(t, tt.wantKeys, sortedKeys(got))
		})
	}
}

func sortedKeys(servers map[string]*config.MCPServer) []string {
	if len(servers) == 0 {
		return nil
	}
	out := make([]string, 0, len(servers))
	for name := range servers {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
