package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

const rolesProjectConfig = `version = "4.0"
name = "roles"
presets = ["claude"]
gitignore = false

[role_manifest]
enabled = true

[[roles]]
name = "backend"
domains = ["backend"]
[roles.skills]
exclude = ["deploy-*"]
[roles.skill_mode]
migrate = "name-only"
"deploy*" = "off"

[[roles]]
name = "frontend"
domains = ["frontend"]
[roles.skill_mode]
"*" = "user-invocable-only"
`

func rolesProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".ai-rulez")
	write := func(rel, body string) {
		p := filepath.Join(cfg, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("config.toml", rolesProjectConfig)
	write("rules/style.md", "# Style\n\nUse tabs.\n")
	skill := func(name string) string {
		return "---\nname: " + name + "\ndescription: Use when you need " + name + ".\n---\n# " + name + "\n"
	}
	write("domains/backend/skills/migrate/SKILL.md", skill("migrate"))
	write("domains/backend/skills/deploy-prod/SKILL.md", skill("deploy-prod"))
	write("domains/backend/skills/deploy/SKILL.md", skill("deploy"))
	write("domains/frontend/skills/ui/SKILL.md", skill("ui"))
	return dir
}

func roleGenerator(t *testing.T, dir, role string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	gen := NewGenerator(cfg)
	if role != "" {
		require.NoError(t, gen.SetRole(role))
	}
	return gen
}

func readSettings(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func TestGenerateRoleFiltersContentAndMergesSkillOverrides(t *testing.T) {
	dir := rolesProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	handAuthored := `{"model": "opus", "skillOverrides": {"handmade": "off"}}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(handAuthored), 0o644))

	require.NoError(t, roleGenerator(t, dir, "backend").Generate(""))

	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "migrate", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "deploy", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "deploy-prod", "SKILL.md"), "excluded by the role")
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "ui", "SKILL.md"), "another role's domain")

	settings := readSettings(t, dir)
	assert.Equal(t, "opus", settings["model"], "unrelated keys survive")
	assert.Equal(t, map[string]any{"handmade": "off", "migrate": "name-only", "deploy": "off"}, settings["skillOverrides"])

	// idempotent: a second run changes nothing
	before, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	require.NoError(t, roleGenerator(t, dir, "backend").Generate(""))
	after, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))

	// the manifest lists every role whichever one was generated
	manifest, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "roles.json"))
	require.NoError(t, err)
	assert.Contains(t, string(manifest), `"name": "frontend"`)

	// clean takes back only what the role claimed
	_, err = roleGenerator(t, dir, "").Clean("", CleanOptions{})
	require.NoError(t, err)
	remaining := readSettings(t, dir)
	assert.Equal(t, "opus", remaining["model"])
	assert.Equal(t, map[string]any{"handmade": "off"}, remaining["skillOverrides"])
}

func TestGenerateRoleSwitchDropsTheOtherRolesOverrides(t *testing.T) {
	dir := rolesProject(t)
	require.NoError(t, roleGenerator(t, dir, "backend").Generate(""))
	require.NoError(t, roleGenerator(t, dir, "frontend").Generate(""))
	settings := readSettings(t, dir)
	assert.Equal(t, map[string]any{"ui": "user-invocable-only"}, settings["skillOverrides"])
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "migrate", "SKILL.md"), "the backend role's skills are stale")
}

func TestSetRoleUnknown(t *testing.T) {
	dir := rolesProject(t)
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	err = NewGenerator(cfg).SetRole("ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestSetRoleDoesNotLeakOverridesIntoTheSharedConfig(t *testing.T) {
	dir := rolesProject(t)
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).SetRole("backend"))
	assert.Nil(t, cfg.ManagedClaudeSettings(), "the loaded config is untouched")
}
