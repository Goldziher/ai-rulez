package exposure

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func overridesRepo(t *testing.T, settings map[string]string) string {
	t.Helper()
	files := map[string]string{
		".claude/commands/team/ship.md":      "---\ndescription: Ship it.\n---\n",
		"svc/.claude/skills/nested/SKILL.md": skillMD("nested", "Nested."),
	}
	for _, n := range []string{"keep", "off", "names", "manual", "bogus", "gated"} {
		extra := ""
		if n == "gated" {
			extra = "paths: [src/**]\n"
		}
		files[".claude/skills/"+n+"/SKILL.md"] = skillMD(n, "Description of "+n+".", extra)
	}
	for name, content := range settings {
		files[".claude/"+name] = content
	}
	return writeRepo(t, files)
}

func overridesJSON(t *testing.T, overrides map[string]string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"skillOverrides": overrides})
	require.NoError(t, err)
	return string(data)
}

func TestBuild_SkillOverrides(t *testing.T) {
	// Arrange
	root := overridesRepo(t, map[string]string{settingsShared: overridesJSON(t, map[string]string{
		"off": OverrideOff, "names": OverrideNameOnly, "manual": OverrideUserInvocableOnly, "keep": OverrideOn,
		"bogus": "sometimes", "gated": OverrideNameOnly, "team:ship": OverrideUserInvocableOnly, "nested": OverrideOff,
	})})

	// Act
	inv := build(t, root)

	// Assert
	assert.Equal(t, []string{"bogus", "gated", "keep", "manual", "names", "team:ship"}, names(inv, KindSkill), "off removes the skill, at any scope")
	names := named(t, inv, KindSkill, "names")
	assert.Equal(t, "names", names.ListingText, "name-only blanks the description")
	assert.Equal(t, "names", names.Text)
	assert.Equal(t, PhaseStart, names.Phase)
	assert.Equal(t, PhaseManual, named(t, inv, KindSkill, "manual").Phase)
	assert.Equal(t, VisibilityManualOnly, named(t, inv, KindSkill, "manual").Visibility)
	assert.Equal(t, PhaseManual, named(t, inv, KindSkill, "team:ship").Phase, "a command is a skill to the overrides")
	assert.Equal(t, "keep: Description of keep.", named(t, inv, KindSkill, "keep").Text)
	assert.Equal(t, "bogus: Description of bogus.", named(t, inv, KindSkill, "bogus").Text, "an unknown value is ignored")
	gated := named(t, inv, KindSkill, "gated")
	assert.Equal(t, PhaseOnDemand, gated.Phase, "name-only keeps the gate")
	assert.Equal(t, "gated", gated.Text)
	assert.Equal(t, map[string]string{
		"off": OverrideOff, "names": OverrideNameOnly, "manual": OverrideUserInvocableOnly, "keep": OverrideOn,
		"bogus": "sometimes", "gated": OverrideNameOnly, "team:ship": OverrideUserInvocableOnly, "nested": OverrideOff,
	}, inv.Settings.SkillOverrides)
}

func TestBuild_NameOnlyClearsTruncation(t *testing.T) {
	root := writeRepo(t, map[string]string{
		".claude/skills/long/SKILL.md": skillMD("long", strings.Repeat("x", 2000)),
		".claude/settings.json":        overridesJSON(t, map[string]string{"long": OverrideNameOnly}),
	})

	a := named(t, build(t, root), KindSkill, "long")

	assert.False(t, a.Truncated)
	assert.Equal(t, 4, a.ListingChars)
}

func TestBuild_UserInvocableOnlySettlesAnUnknownPhase(t *testing.T) {
	root := writeRepo(t, map[string]string{
		".claude/skills/s/SKILL.md": "---\ndescription: a: b\npaths: [x]\n---\n",
		".claude/settings.json":     overridesJSON(t, map[string]string{"s": OverrideUserInvocableOnly}),
	})

	assert.Equal(t, PhaseManual, named(t, build(t, root), KindSkill, "s").Phase)
}

func TestBuild_LocalSettingsApplyOverShared(t *testing.T) {
	tests := []struct {
		name          string
		shared, local string
		wantSkills    []string
	}{
		{name: "local turns a skill back on", shared: `{"skillOverrides": {"keep": "off"}}`, local: `{"skillOverrides": {"keep": "on"}}`, wantSkills: []string{"bogus", "gated", "keep", "manual", "names", "nested", "off", "team:ship"}},
		{name: "local turns a skill off", shared: `{}`, local: `{"skillOverrides": {"keep": "off"}}`, wantSkills: []string{"bogus", "gated", "manual", "names", "nested", "off", "team:ship"}},
		{name: "a malformed local file is ignored", shared: `{"skillOverrides": {"keep": "off"}}`, local: `{not json`, wantSkills: []string{"bogus", "gated", "manual", "names", "nested", "off", "team:ship"}},
		{name: "a local file of the wrong shape is ignored whole", shared: `{"skillOverrides": {"keep": "off"}}`, local: `{"skillOverrides": {"keep": "on"}, "claudeMdExcludes": "x"}`, wantSkills: []string{"bogus", "gated", "manual", "names", "nested", "off", "team:ship"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := overridesRepo(t, map[string]string{settingsShared: tt.shared, settingsLocal: tt.local})

			inv := build(t, root)

			assert.Equal(t, tt.wantSkills, names(inv, KindSkill))
		})
	}
}

func TestBuild_UnlistedLocalSettingsAreRead(t *testing.T) {
	root := overridesRepo(t, map[string]string{settingsLocal: `{"skillOverrides": {"keep": "off"}, "enabledPlugins": {"p@m": true}}`})
	var tracked []string
	for _, f := range listing(t, root) {
		if f != ".claude/"+settingsLocal {
			tracked = append(tracked, f)
		}
	}

	inv := build(t, root, func(o *Options) { o.Files = tracked })

	assert.NotContains(t, names(inv, KindSkill), "keep", "git ignores settings.local.json; Claude Code still reads it")
	assert.Equal(t, []string{".claude/" + settingsLocal}, paths(inv.Artifacts, KindSettings))
	assert.Equal(t, []string{"p@m"}, names(inv, KindPlugin))
}

func TestBuild_ClaudeMdExcludes(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{name: "a doublestar names a nested CLAUDE.md", patterns: []string{"**/services/CLAUDE.md"}, want: []string{"CLAUDE.md", ".claude/rules/r.md", "services/.claude/rules/n.md"}},
		{name: "rules are excluded too", patterns: []string{"**/.claude/rules/*.md"}, want: []string{"CLAUDE.md", "services/CLAUDE.md"}},
		{name: "every CLAUDE.md", patterns: []string{"**/CLAUDE.md"}, want: []string{".claude/rules/r.md", "services/.claude/rules/n.md"}},
		{name: "a star stays in its segment", patterns: []string{"/services/*.md"}, want: []string{"CLAUDE.md", ".claude/rules/r.md", "services/.claude/rules/n.md"}},
		{name: "a question mark is one character", patterns: []string{"/servic?s/CLAUDE.md"}, want: []string{"CLAUDE.md", ".claude/rules/r.md", "services/.claude/rules/n.md"}},
		{name: "a doublestar without a slash crosses directories", patterns: []string{"/services**"}, want: []string{"CLAUDE.md", ".claude/rules/r.md"}},
		{name: "a relative pattern matches nothing", patterns: []string{"services/CLAUDE.md"}, want: []string{"CLAUDE.md", ".claude/rules/r.md", "services/CLAUDE.md", "services/.claude/rules/n.md"}},
		{name: "regexp characters are literal", patterns: []string{"/(services)/CLAUDE.md"}, want: []string{"CLAUDE.md", ".claude/rules/r.md", "services/CLAUDE.md", "services/.claude/rules/n.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{
				"CLAUDE.md":                   "root",
				"services/CLAUDE.md":          "services",
				".claude/rules/r.md":          "rule",
				"services/.claude/rules/n.md": "nested rule",
				".claude/skills/s/SKILL.md":   skillMD("s", "d"),
			})
			abs := filepath.ToSlash(root)
			var patterns []string
			for _, p := range tt.patterns {
				switch p[0] {
				case '/':
					p = abs + p
				case '*':
					p = abs + "/" + p
				}
				patterns = append(patterns, p)
			}
			data, err := json.Marshal(map[string]any{"claudeMdExcludes": patterns})
			require.NoError(t, err)
			writeFile(t, root, ".claude/settings.json", string(data))

			inv := build(t, root)

			assert.ElementsMatch(t, tt.want, paths(inv.Artifacts, KindClaudeMD, KindRule))
			assert.Equal(t, []string{"s"}, names(inv, KindSkill), "excludes never touch skills")
			assert.Equal(t, patterns, inv.Settings.ClaudeMdExcludes)
		})
	}
}

func TestBuild_ARelativeRootIsMadeAbsolute(t *testing.T) {
	// Arrange
	root := writeRepo(t, map[string]string{"CLAUDE.md": "root", "a/CLAUDE.md": "a"})
	abs, err := filepath.Abs(root)
	require.NoError(t, err)
	writeFile(t, root, ".claude/settings.json", `{"claudeMdExcludes": ["`+filepath.ToSlash(abs)+`/a/CLAUDE.md"]}`)
	files := listing(t, root)
	t.Chdir(filepath.Dir(root))

	// Act
	inv, err := Build(context.Background(), Options{Root: filepath.Base(root), Files: files})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, filepath.ToSlash(abs), inv.Root)
	assert.Equal(t, []string{"CLAUDE.md"}, paths(inv.Artifacts, KindClaudeMD), "an absolute exclude matches below a relative root")
}

func TestBuild_ClaudeMdExcludesConcatenateLocalAndShared(t *testing.T) {
	root := writeRepo(t, map[string]string{"CLAUDE.md": "root", "a/CLAUDE.md": "a", "b/CLAUDE.md": "b"})
	abs := filepath.ToSlash(root)
	writeFile(t, root, ".claude/settings.json", `{"claudeMdExcludes": ["`+abs+`/a/CLAUDE.md"]}`)
	writeFile(t, root, ".claude/settings.local.json", `{"claudeMdExcludes": ["`+abs+`/b/CLAUDE.md"]}`)

	inv := build(t, root)

	assert.Equal(t, []string{"CLAUDE.md"}, paths(inv.Artifacts, KindClaudeMD))
	assert.Equal(t, []string{abs + "/a/CLAUDE.md", abs + "/b/CLAUDE.md"}, inv.Settings.ClaudeMdExcludes)
}

func TestBuild_NestedSettingsDoNotApplyToTheRoot(t *testing.T) {
	root := overridesRepo(t, nil)
	writeFile(t, root, "svc/.claude/settings.json", `{"skillOverrides": {"keep": "off"}, "enabledPlugins": {"p@m": true}}`)

	inv := build(t, root)

	assert.Contains(t, names(inv, KindSkill), "keep")
	plugin := named(t, inv, KindPlugin, "p@m")
	assert.Equal(t, "svc", plugin.Scope)
	assert.Equal(t, PhaseOnDemand, plugin.Phase)
	assert.Empty(t, inv.Settings.EnabledPlugins)
}

func TestBuild_EnabledPluginArtifacts(t *testing.T) {
	tests := []struct {
		name          string
		shared, local string
		want          []string // "path name"
		wantEnabled   []string
	}{
		{name: "only true values", shared: `{"enabledPlugins": {"one@m": true, "off@m": false}}`, want: []string{".claude/settings.json one@m"}, wantEnabled: []string{"one@m"}},
		{name: "local disables a shared plugin", shared: `{"enabledPlugins": {"one@m": true, "two@m": true}}`, local: `{"enabledPlugins": {"two@m": false}}`, want: []string{".claude/settings.json one@m"}, wantEnabled: []string{"one@m"}},
		{name: "a plugin both enable is listed once, from the tracked shared file", shared: `{"enabledPlugins": {"one@m": true}}`, local: `{"enabledPlugins": {"one@m": true}}`, want: []string{".claude/settings.json one@m"}, wantEnabled: []string{"one@m"}},
		{name: "only local enables a plugin the shared file does not name", shared: `{"enabledPlugins": {"one@m": true}}`, local: `{"enabledPlugins": {"two@m": true, "one@m": true}}`, want: []string{".claude/settings.json one@m", ".claude/settings.local.json two@m"}, wantEnabled: []string{"one@m", "two@m"}},
		{name: "local enables a plugin", shared: `{"enabledPlugins": {"one@m": false}}`, local: `{"enabledPlugins": {"one@m": true}}`, want: []string{".claude/settings.local.json one@m"}, wantEnabled: []string{"one@m"}},
		{name: "a non-boolean value makes the file unreadable", shared: `{"enabledPlugins": {"one@m": "yes"}}`},
		{name: "keys keep their order", shared: `{"enabledPlugins": {"z@m": true, "a@m": true}}`, want: []string{".claude/settings.json z@m", ".claude/settings.json a@m"}, wantEnabled: []string{"a@m", "z@m"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{".claude/settings.json": tt.shared}
			if tt.local != "" {
				files[".claude/settings.local.json"] = tt.local
			}
			root := writeRepo(t, files)

			inv := build(t, root)

			var got []string
			for _, a := range inv.Artifacts {
				if a.Kind == KindPlugin {
					assert.Equal(t, PhaseStart, a.Phase)
					assert.Equal(t, a.Name, a.Text)
					got = append(got, a.Path+" "+a.Name)
				}
			}
			assert.Equal(t, tt.want, got)
			wantEnabled := tt.wantEnabled
			if wantEnabled == nil {
				wantEnabled = []string{}
			}
			assert.Equal(t, wantEnabled, inv.Settings.EnabledPlugins)
		})
	}
}

func TestBuild_UnreadableSettingsAreAProblem(t *testing.T) {
	root := overridesRepo(t, map[string]string{settingsShared: `{"skillOverrides": {"keep": "off"}, "enabledPlugins": []}`})

	inv := build(t, root)

	assert.Contains(t, names(inv, KindSkill), "keep", "none of an unreadable file applies")
	assert.Equal(t, []string{".claude/settings.json"}, paths(inv.Artifacts, KindSettings), "the file is still listed")
	require.Len(t, inv.Problems, 1)
	assert.Equal(t, ".claude/settings.json", inv.Problems[0].Path)
}

// Claude Code reads the settings and .mcp.json keys as written: a key in
// another case is an unknown key and is ignored.
func TestBuild_SettingsKeysAreCaseSensitive(t *testing.T) {
	tests := []struct {
		name, file, content string
	}{
		{name: "SkillOverrides", file: ".claude/" + settingsLocal, content: `{"SkillOverrides": {"off": "off"}}`},
		{name: "SKILLOVERRIDES in the shared file", file: ".claude/" + settingsShared, content: `{"SKILLOVERRIDES": {"off": "off"}}`},
		{name: "EnabledPlugins", file: ".claude/" + settingsShared, content: `{"EnabledPlugins": {"p@m": true}}`},
		{name: "ClaudeMdExcludes", file: ".claude/" + settingsShared, content: `{"ClaudeMdExcludes": ["**/CLAUDE.md"]}`},
		{name: "MCPSERVERS", file: ".mcp.json", content: `{"MCPSERVERS": {"srv": {}}}`},
		{name: "McpServers in a nested file", file: "svc/.mcp.json", content: `{"McpServers": {"srv": {}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := overridesRepo(t, nil)
			writeFile(t, root, "CLAUDE.md", "root")
			writeFile(t, root, tt.file, tt.content)

			// Act
			inv := build(t, root)

			// Assert
			assert.Empty(t, inv.Problems, "an unknown key is not an error")
			assert.Contains(t, names(inv, KindSkill), "off")
			assert.Empty(t, inv.Settings.SkillOverrides)
			assert.Empty(t, inv.Settings.EnabledPlugins)
			assert.Empty(t, inv.Settings.ClaudeMdExcludes)
			assert.Equal(t, []string{"CLAUDE.md"}, paths(inv.Artifacts, KindClaudeMD))
			assert.Empty(t, paths(inv.Artifacts, KindMCPServer, KindPlugin))
		})
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o600))
}
