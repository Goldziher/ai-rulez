package mcp

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoSkillsMessage(t *testing.T) {
	t.Parallel()
	served := []generator.ServedSkill{
		{ID: "alpha", Domain: "ops"}, {ID: "beta", Domain: ""}, {ID: "gamma", Domain: "ops"},
	}
	tests := []struct {
		name   string
		served []generator.ServedSkill
		filter SkillFilter
		want   []string
	}{
		{"nothing delivered", nil, SkillFilter{}, []string{"nothing has delivery served or both", "--include-static"}},
		{"domain removed everything", served, SkillFilter{Domains: []string{"other"}}, []string{"3 skill(s)", "--domain other keeps 0"}},
		{"deny removed everything", served, SkillFilter{Deny: []string{"*"}}, []string{"--deny * leaves 0"}},
		{"allow removed everything", served, SkillFilter{Allow: []string{"zeta"}}, []string{"--allow zeta keeps 0"}},
		{"filters combine", served, SkillFilter{Domains: []string{"ops"}, Deny: []string{"alpha", "gamma"}},
			[]string{"--domain ops keeps 2", "--deny alpha,gamma leaves 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := noSkillsMessage(tt.served, tt.filter)
			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}
		})
	}
}

func TestServeSetup_UnknownDomainIsAnErrorAtStart(t *testing.T) {
	root := project(t, baseConfig+"\n[skills]\ndelivery = \"served\"\n", map[string]string{
		"skills/core/SKILL.md":               skillFile("core", "Core conventions", ""),
		"domains/ops/skills/deploy/SKILL.md": skillFile("deploy", "Deploy things", ""),
	})

	_, err := (&ServeSetup{WorkDir: root, NoWatch: true, Filter: SkillFilter{Domains: []string{"opz"}}}).NewServer(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown domain "opz"`)

	srv, err := (&ServeSetup{WorkDir: root, NoWatch: true, Filter: SkillFilter{Domains: []string{"root", "ops"}}}).NewServer(context.Background())
	require.NoError(t, err)
	assert.Len(t, srv.Catalog().Skills(), 2)
}
