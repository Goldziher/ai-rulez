package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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

// A role's skillOverrides, the top-level [[hooks]] and the [permissions] rules
// all live in .claude/settings.json. Each is owned key by key, so a role run,
// a plain run, a user's own entries and `clean` leave the others alone.
func TestRoleSkillOverridesCoexistWithHooksAndPermissions(t *testing.T) {
	dir := rolesProject(t)
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	base, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	extra := `
[permissions]
allow = ["Bash(go test:*)"]
deny = ["Read(.env)"]

[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
command = "echo guard"
`
	require.NoError(t, os.WriteFile(cfgPath, append(base, []byte(extra)...), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	user := `{"model": "opus", "permissions": {"allow": ["Bash(ls:*)"]}, "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo mine"}]}]}}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(user), 0o644))

	check := func(t *testing.T, s map[string]any, wantOverrides map[string]any) {
		t.Helper()
		assert.Equal(t, "opus", s["model"])
		perms, _ := s["permissions"].(map[string]any)                                   //nolint:errcheck // asserted below
		assert.ElementsMatch(t, []any{"Bash(ls:*)", "Bash(go test:*)"}, perms["allow"]) // the user's rule and ours
		assert.Equal(t, []any{"Read(.env)"}, perms["deny"])
		hooks, _ := s["hooks"].(map[string]any) //nolint:errcheck // asserted below
		assert.Contains(t, hooks, "Stop", "the user's hook survives")
		assert.Contains(t, hooks, "PreToolUse")
		if wantOverrides == nil {
			assert.NotContains(t, s, "skillOverrides")
		} else {
			assert.Equal(t, wantOverrides, s["skillOverrides"])
		}
	}

	require.NoError(t, roleGenerator(t, dir, "").Generate(""))
	check(t, readSettings(t, dir), nil)

	require.NoError(t, roleGenerator(t, dir, "backend").Generate(""))
	check(t, readSettings(t, dir), map[string]any{"migrate": "name-only", "deploy": "off"})

	require.NoError(t, roleGenerator(t, dir, "").Generate(""))
	check(t, readSettings(t, dir), nil)

	require.NoError(t, roleGenerator(t, dir, "backend").Generate(""))
	_, err = roleGenerator(t, dir, "").Clean("", CleanOptions{})
	require.NoError(t, err)
	after := readSettings(t, dir)
	assert.Equal(t, "opus", after["model"])
	assert.NotContains(t, after, "skillOverrides")
	perms, _ := after["permissions"].(map[string]any) //nolint:errcheck // asserted below
	assert.Equal(t, []any{"Bash(ls:*)"}, perms["allow"], "clean removes only the rules we own")
	hooks, _ := after["hooks"].(map[string]any) //nolint:errcheck // asserted below
	assert.Contains(t, hooks, "Stop")
	assert.NotContains(t, hooks, "PreToolUse")
}
