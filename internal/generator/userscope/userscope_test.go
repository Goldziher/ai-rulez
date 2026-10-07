package userscope_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/userscope"

	_ "github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	_ "github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
)

// native turns a slash path that starts at the root into an absolute path of the
// running OS: on Windows that needs the volume of the temp directory, because a
// bare \home\probe is not absolute there.
func native(p string) string {
	if strings.HasPrefix(p, "/") {
		return filepath.VolumeName(os.TempDir()) + filepath.FromSlash(p)
	}
	return filepath.FromSlash(p)
}

var home = native("/home/probe")

func noEnv(string) string { return "" }

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func abs(rel string) string { return filepath.Join(home, filepath.FromSlash(rel)) }

func TestMap(t *testing.T) {
	tests := []struct {
		name, preset, rel string
		getenv            func(string) string
		want              string
		ok                bool
	}{
		{"claude root file", "claude", "CLAUDE.md", noEnv, abs(".claude/CLAUDE.md"), true},
		{"claude skill file", "claude", ".claude/skills/x/SKILL.md", noEnv, abs(".claude/skills/x/SKILL.md"), true},
		{"claude skill resource", "claude", ".claude/skills/x/scripts/run.sh", noEnv, abs(".claude/skills/x/scripts/run.sh"), true},
		{"skills directory marker", "claude", ".claude/skills", noEnv, abs(".claude/skills"), true},
		{"claude settings", "claude", ".claude/settings.json", noEnv, abs(".claude/settings.json"), true},
		{"claude plugins sidecar has no user location", "claude", ".claude/plugins.json", noEnv, "", false},
		{"the parent directory marker is not mapped", "claude", ".claude", noEnv, "", false},
		{"codex AGENTS.md", "codex", "AGENTS.md", noEnv, abs(".codex/AGENTS.md"), true},
		{"codex skills stay in the shared directory", "codex", ".agents/skills/x/SKILL.md", noEnv, abs(".agents/skills/x/SKILL.md"), true},
		{"codex hooks", "codex", ".codex/hooks.json", noEnv, abs(".codex/hooks.json"), true},
		{"opencode AGENTS.md", "opencode", "AGENTS.md", noEnv, abs(".config/opencode/AGENTS.md"), true},
		{"opencode skills", "opencode", ".opencode/skills/x/SKILL.md", noEnv, abs(".config/opencode/skills/x/SKILL.md"), true},
		{"copilot skills", "copilot", ".github/skills/x/SKILL.md", noEnv, abs(".copilot/skills/x/SKILL.md"), true},
		{"copilot hooks", "copilot", ".github/hooks/ai-rulez.json", noEnv, abs(".copilot/hooks/ai-rulez.json"), true},
		{"pi skills", "pi", ".agents/skills/x/SKILL.md", noEnv, abs(".pi/agent/skills/x/SKILL.md"), true},
		{"a longer row wins: cline workflows sit inside the rules folder", "cline", ".clinerules/workflows/ship.md", noEnv,
			abs("Documents/Cline/Workflows/ship.md"), true},
		{"cline rules", "cline", ".clinerules/style.md", noEnv, abs("Documents/Cline/Rules/style.md"), true},
		{"claude commands leave with the skills", "claude", ".claude/skills/ship/SKILL.md", noEnv, abs(".claude/skills/ship/SKILL.md"), true},
		{"a prefix is not a path prefix", "claude", ".claude/skills-extra/x.md", noEnv, "", false},
		{"unsupported preset", "baz", "AGENTS.md", noEnv, "", false},
		{"backslashes are normalised", "claude", `.claude\skills\x\SKILL.md`, noEnv, abs(".claude/skills/x/SKILL.md"), true},
		{"CODEX_HOME relocates the codex files", "codex", "AGENTS.md", env(map[string]string{"CODEX_HOME": native("/tools/codex")}),
			native("/tools/codex/AGENTS.md"), true},
		{"CODEX_HOME leaves the shared skills directory", "codex", ".agents/skills/x/SKILL.md", env(map[string]string{"CODEX_HOME": native("/tools/codex")}),
			abs(".agents/skills/x/SKILL.md"), true},
		{"a relative home override is ignored", "codex", "AGENTS.md", env(map[string]string{"CODEX_HOME": "tools/codex"}),
			abs(".codex/AGENTS.md"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset := tt.preset
			layout, err := userscope.Resolve(preset, home, tt.getenv)
			if err != nil {
				require.True(t, userscope.IsUnsupported(err))
				assert.False(t, tt.ok)
				return
			}
			got, row, ok := layout.Map(tt.rel)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
			if ok {
				assert.NotEmpty(t, row.Kind)
			}
		})
	}
}

func TestResolve_RequiresAbsoluteHome(t *testing.T) {
	_, err := userscope.Resolve("claude", "relative/home", noEnv)
	require.Error(t, err)
	assert.False(t, userscope.IsUnsupported(err))
	assert.Contains(t, err.Error(), "absolute")
}

func TestResolve_UnsupportedPresetsGiveReasons(t *testing.T) {
	for _, name := range []string{"baz", "xum", "no-such-preset"} {
		_, err := userscope.Resolve(name, home, noEnv)
		var unsupported *userscope.UnsupportedError
		require.ErrorAs(t, err, &unsupported, name)
		assert.Equal(t, name, unsupported.Preset)
		assert.NotEmpty(t, unsupported.Reason)
	}
}

// TestEveryPresetIsResolved: each built-in preset is either supported with
// absolute destinations under the home directory or unsupported with a reason.
func TestEveryPresetIsResolved(t *testing.T) {
	layouts, unsupported, err := userscope.All(home, noEnv)
	require.NoError(t, err)

	names := config.IndividualPresetNames()
	assert.Len(t, layouts, len(names)-len(unsupported))
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			layout, ok := layouts[name]
			if !ok {
				assert.NotEmpty(t, unsupported[name], "%s is unsupported without a reason", name)
				return
			}
			assert.Empty(t, unsupported[name])
			assert.Positive(t, len(layout.Kinds()))
			seen := map[string]bool{}
			for _, row := range layout.Rows {
				key := row.From + "|" + string(row.Kind)
				assert.False(t, seen[key], "duplicate row %s", key)
				seen[key] = true
				assert.NotContains(t, row.From, "..")
				assert.False(t, filepath.IsAbs(row.From), "%s is project-relative", row.From)
				if row.To == "" {
					continue
				}
				assert.True(t, filepath.IsAbs(row.To), "%s is absolute", row.To)
				rel, relErr := filepath.Rel(home, row.To)
				require.NoError(t, relErr)
				assert.False(t, strings.HasPrefix(rel, ".."), "%s lies below the home directory", row.To)
				assert.NotEqual(t, ".", rel)
			}
			for _, root := range layout.Roots() {
				assert.NotEqual(t, home, root, "the home directory is never an owned root")
			}
			for _, reader := range layout.SkillReaders {
				assert.True(t, filepath.IsAbs(reader), reader)
			}
		})
	}
	assert.GreaterOrEqual(t, len(layouts), 45, "nearly every preset documents a user scope")
}

func TestRelocatedHome(t *testing.T) {
	layout, err := userscope.Resolve("codex", home, env(map[string]string{"CODEX_HOME": native("/tools/codex")}))
	require.NoError(t, err)
	assert.Equal(t, native("/tools/codex"), layout.RelocatedHome)

	layout, err = userscope.Resolve("codex", home, noEnv)
	require.NoError(t, err)
	assert.Empty(t, layout.RelocatedHome)

	layout, err = userscope.Resolve("codex", home, env(map[string]string{"CODEX_HOME": "relative"}))
	require.NoError(t, err)
	assert.Empty(t, layout.RelocatedHome, "a relative override is not honored")
}

func TestPrecedenceAndReaders(t *testing.T) {
	claude, err := userscope.Resolve("claude", home, noEnv)
	require.NoError(t, err)
	assert.Contains(t, claude.Precedence(), "user-level skill")

	gemini, err := userscope.Resolve("gemini", home, noEnv)
	require.NoError(t, err)
	assert.Equal(t, []string{abs(".gemini/skills"), abs(".agents/skills")}, gemini.SkillReaders)

	pi, err := userscope.Resolve("pi", home, noEnv)
	require.NoError(t, err)
	assert.Equal(t, "pi does not document a precedence; keep names unique", pi.Precedence())
}

const (
	tableStart = "<!-- user-scope-table:start -->"
	tableEnd   = "<!-- user-scope-table:end -->"
)

// userScopeTable renders the "where things go" table from the layouts.
func userScopeTable(t *testing.T) string {
	t.Helper()
	layouts, _, err := userscope.All(home, noEnv)
	require.NoError(t, err)
	var b strings.Builder
	b.WriteString("| Harness | Output | Project path | User-level path |\n| ------- | ------ | ------------ | --------------- |\n")
	for _, name := range userscope.Supported(layouts) {
		for _, row := range layouts[name].Rows {
			if row.To == "" {
				continue
			}
			rel, relErr := filepath.Rel(home, row.To)
			require.NoError(t, relErr)
			suffix := ""
			if row.Dir {
				suffix = "/"
			}
			fmt.Fprintf(&b, "| `%s` | %s | `%s%s` | `~/%s%s` |\n", name, row.Kind, row.From, suffix, filepath.ToSlash(rel), suffix)
		}
	}
	return b.String()
}

// TestDocsTable pins the "where things go" table of docs/user-scope.md to the
// layouts the presets declare. Run with UPDATE_DOCS=1 to rewrite the table.
func TestDocsTable(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "user-scope.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	doc := string(data)
	start := strings.Index(doc, tableStart)
	end := strings.Index(doc, tableEnd)
	require.True(t, start >= 0 && end > start, "docs/user-scope.md needs the %s markers", tableStart)

	want := userScopeTable(t)
	got := strings.TrimPrefix(doc[start+len(tableStart):end], "\n")
	if os.Getenv("UPDATE_DOCS") != "" && got != want {
		updated := doc[:start+len(tableStart)] + "\n" + want + doc[end:]
		require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))
		return
	}
	assert.Equal(t, want, got, "docs/user-scope.md is out of date; run UPDATE_DOCS=1 go test ./internal/generator/userscope")
}

func TestResolve_RefusesDangerousHomeOverrides(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		refused bool
	}{
		{"filesystem root", "/", true},
		{"an ancestor of the home directory", "/home", true},
		{"the home directory itself", "/home/probe", true},
		{"a relative value is ignored", "relative/codex", false},
		{"a directory elsewhere", "/tools/codex", false},
		{"a directory below the home directory", "/home/probe/tools/codex", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			layout, err := userscope.Resolve("codex", home, env(map[string]string{"CODEX_HOME": native(tt.value)}))

			// Assert
			if tt.refused {
				require.Error(t, err)
				assert.True(t, userscope.IsUnsupported(err))
				assert.Contains(t, err.Error(), "home variable")
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, layout)
		})
	}
}
