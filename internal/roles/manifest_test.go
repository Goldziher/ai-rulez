package roles

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

func fixtureConfig(t *testing.T) *config.Config {
	t.Helper()
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".ai-rulez")
	write := func(rel, body string) string {
		p := filepath.Join(cfgDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		return p
	}
	rule := config.ContentFile{Name: "style", Path: write("rules/style.md", "# Style\nUse tabs.\n")}
	migrate := config.ContentFile{Name: "migrate", Path: write("domains/backend/skills/migrate/SKILL.md", "---\nname: migrate\nowner: db-team\nversion: 2.0.0\n---\nMigrate.\n"),
		Metadata:  &config.Metadata{Extra: map[string]string{"owner": "db-team", "version": "2.0.0"}},
		Resources: []config.SkillResource{{RelPath: "references/x.md", Content: []byte("12345")}}}
	deploy := config.ContentFile{Name: "deploy", Path: write("domains/backend/skills/deploy/SKILL.md", "deploy\n")}
	return &config.Config{
		ConfigDir: cfgDir, BaseDir: root,
		Content: &config.ContentTree{
			Rules:   []config.ContentFile{rule},
			Domains: map[string]*config.Domain{"backend": {Name: "backend", Skills: []config.ContentFile{migrate, deploy}}},
		},
		Roles: []config.RoleConfig{
			{Name: "zeta", Domains: []string{"backend"}, Skills: &config.RoleSelector{Exclude: []string{"deploy"}},
				SkillMode: map[string]string{"migrate": "name-only"}, Match: &config.RoleMatch{Groups: []string{"okta:dev"}}},
			{Name: "alpha", Domains: []string{"backend"}},
		},
	}
}

func TestBuildIsDeterministicAndMatchesSchema(t *testing.T) {
	cfg := fixtureConfig(t)
	counter, err := tokens.New("")
	require.NoError(t, err)
	first, err := Build(cfg, counter).Marshal()
	require.NoError(t, err)

	cfg.Roles[0], cfg.Roles[1] = cfg.Roles[1], cfg.Roles[0]
	second, err := Build(cfg, counter).Marshal()
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "declaration order does not matter")

	var m Manifest
	require.NoError(t, json.Unmarshal(first, &m))
	require.Len(t, m.Roles, 2)
	assert.Equal(t, "alpha", m.Roles[0].Name)
	zeta := m.Roles[1]
	assert.Equal(t, "okta:dev", zeta.Match.Groups[0])
	assert.Equal(t, map[string]string{"migrate": "name-only"}, zeta.SkillModes)
	assert.Equal(t, 2, zeta.Totals.Items, "the root rule and the migrate skill; deploy is excluded")
	var migrate Item
	for _, it := range zeta.Items {
		if it.ID == "migrate" {
			migrate = it
		}
	}
	assert.Equal(t, "db-team", migrate.Owner)
	assert.Equal(t, "name-only", migrate.Mode)
	assert.Equal(t, len("---\nname: migrate\nowner: db-team\nversion: 2.0.0\n---\nMigrate.\n")+5, migrate.Bytes, "resources count towards bytes")
	assert.Positive(t, migrate.Tokens)
	assert.NotContains(t, string(first), "generated_at")

	schemaBytes, err := os.ReadFile("../../schema/roles-manifest.schema.json")
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.NewDecoder(bytes.NewReader(first)).Decode(&doc))
	result := compiled.Validate(doc)
	assert.True(t, result.IsValid(), "manifest violates its schema: %v", result.Errors)
}
