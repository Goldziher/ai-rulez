package settings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/settings"
)

// sampleRules covers every rule form the translators handle.
func sampleRules() *config.Permissions {
	return &config.Permissions{
		Allow: []string{
			"Bash(npm run test:*)", "Bash(git status)", "Read(./src/**)", "Edit(src/**)",
			"WebFetch(domain:example.com)", "mcp__github__create_issue",
		},
		Ask:  []string{"Bash(git push:*)"},
		Deny: []string{"Bash(rm -rf:*)", "Read(./.env)", "WebFetch(domain:evil.com)", "mcp__evil__tool", "Bash(curl * | sh)"},
	}
}

func permConfig(dir string, rules *config.Permissions, presets ...string) *config.Config {
	cfg := &config.Config{BaseDir: dir, Permissions: rules}
	for _, name := range presets {
		cfg.Presets = append(cfg.Presets, config.Preset{BuiltIn: name})
	}
	return cfg
}

type permDialectCase struct {
	dialect string
	path    string
	format  docmerge.Format
}

var permDialectCases = []permDialectCase{
	{"codebuddy", ".codebuddy/settings.json", docmerge.FormatJSON},
	{"commandcode", ".commandcode/settings.json", docmerge.FormatJSON},
	{"qoder", ".qoder/settings.json", docmerge.FormatJSON},
	{"qwen", ".qwen/settings.json", docmerge.FormatJSON},
	{"letta", ".letta/settings.json", docmerge.FormatJSON},
	{"opencode", "opencode.json", docmerge.FormatJSONC},
	{"kilo", "kilo.jsonc", docmerge.FormatJSONC},
	{"mimocode", ".mimocode/mimocode.jsonc", docmerge.FormatJSONC},
	{"gemini", ".gemini/settings.json", docmerge.FormatJSON},
	{"devin", ".devin/config.json", docmerge.FormatJSONC},
	{"grok", ".grok/config.toml", docmerge.FormatTOML},
	{"vibe", ".vibe/config.toml", docmerge.FormatTOML},
	{"poolside", ".poolside/settings.yaml", docmerge.FormatYAML},
	{"omp", ".omp/config.yml", docmerge.FormatYAML},
	{"augment", ".augment/settings.json", docmerge.FormatJSONC},
	{"copilot", ".vscode/settings.json", docmerge.FormatJSONC},
	{"zoocode", ".vscode/settings.json", docmerge.FormatJSONC},
	{"copilot-cli", ".github/copilot/settings.json", docmerge.FormatJSONC},
	{"cursor", ".cursor/cli.json", docmerge.FormatJSON},
	{"zed", ".zed/settings.json", docmerge.FormatJSONC},
	{"hermes", ".hermes/config.yaml", docmerge.FormatYAML},
	{"kimi", ".kimi-code/config.toml", docmerge.FormatTOML},
}

func renderPermDialect(t *testing.T, c permDialectCase, cfg *config.Config, doc string) docmerge.Result {
	t.Helper()
	keys, err := settings.PermissionKeys(cfg, c.dialect, doc)
	require.NoError(t, err)
	result, err := docmerge.Apply(doc, c.format, keys)
	require.NoError(t, err)
	return result
}

func goldenPerm(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "permissions", name+".golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with UPDATE_GOLDEN=1 to create %s", path)
	assert.Equal(t, string(want), got)
}

func TestPermissionDialectsGolden(t *testing.T) {
	for _, c := range permDialectCases {
		t.Run(c.dialect, func(t *testing.T) {
			captureWarnings(t)
			// Arrange
			dir := t.TempDir()
			cfg := permConfig(dir, sampleRules(), "copilot", "zoocode")

			// Act
			result := renderPermDialect(t, c, cfg, filepath.Join(dir, filepath.FromSlash(c.path)))

			// Assert
			goldenPerm(t, c.dialect, result.Body)
		})
	}
}

func TestEveryDialectIsTested(t *testing.T) {
	var tested []string
	for _, c := range permDialectCases {
		tested = append(tested, c.dialect)
	}
	assert.ElementsMatch(t, settings.PermissionDialectNames(), tested)
}

func TestPermissionKeysIgnoresEmptyScopedAndUnknown(t *testing.T) {
	dir := t.TempDir()
	keys, err := settings.PermissionKeys(permConfig(dir, nil), "grok", "")
	require.NoError(t, err)
	assert.Empty(t, keys)

	scoped := permConfig(dir, sampleRules())
	scoped.Run = config.NewRunState().ForScope(&config.ScopeRun{Path: "pkg"})
	keys, err = settings.PermissionKeys(scoped, "grok", "")
	require.NoError(t, err)
	assert.Empty(t, keys, "a [[scopes]] subdirectory renders no project-wide settings")

	_, err = settings.PermissionKeys(permConfig(dir, sampleRules()), "nope", "")
	assert.ErrorContains(t, err, "unknown permissions dialect")
}

// hand-authored documents, one per format, with comments the merge must keep.
var handAuthoredDocs = map[string]string{
	"opencode": `{
  // my settings
  "model": "x/y",
  "permission": {
    "bash": {
      "make test": "allow" // keep
    }
  }
}
`,
	"grok": `# my config
model = "grok-4"

[permission]
# keep me
allow = ["Bash(make test)"]
`,
	"poolside": `# my settings
tools:
  shell:
    allow:
      - make test # keep
`,
}

func TestPermissionsMergeKeepsHandAuthoredContentAndRoundTrips(t *testing.T) {
	for _, c := range permDialectCases {
		original, ok := handAuthoredDocs[c.dialect]
		if !ok {
			continue
		}
		t.Run(c.dialect, func(t *testing.T) {
			captureWarnings(t)
			// Arrange
			dir := t.TempDir()
			doc := filepath.Join(dir, filepath.FromSlash(c.path))
			require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
			require.NoError(t, os.WriteFile(doc, []byte(original), 0o644))
			cfg := permConfig(dir, &config.Permissions{Allow: []string{"Bash(npm run test:*)"}, Deny: []string{"Bash(rm -rf:*)"}})

			// Act
			merged := renderPermDialect(t, c, cfg, doc)

			// Assert: theirs and ours are both there, comments included.
			assert.True(t, merged.PartiallyOwned)
			for _, kept := range []string{"make test", "keep"} {
				assert.Contains(t, merged.Body, kept)
			}
			assert.Contains(t, merged.Body, "npm run test")
			assert.Contains(t, merged.Body, "rm -rf")

			// A second run over its own output changes nothing.
			require.NoError(t, os.WriteFile(doc, []byte(merged.Body), 0o644))
			assert.Equal(t, merged.Body, renderPermDialect(t, c, cfg, doc).Body, "generate is idempotent")

			// clean takes back exactly what was claimed: the original, byte for byte.
			clean, err := docmerge.Unmerge(doc, c.format, merged.Claims)
			require.NoError(t, err)
			require.True(t, clean.Changed)
			assert.Equal(t, original, clean.Body)
		})
	}
}

func TestPermissionsDropRemovedRulesButKeepTheirs(t *testing.T) {
	captureWarnings(t)
	c := permDialectCase{"grok", ".grok/config.toml", docmerge.FormatTOML}
	dir := t.TempDir()
	doc := filepath.Join(dir, c.path)
	require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
	require.NoError(t, os.WriteFile(doc, []byte(handAuthoredDocs["grok"]), 0o644))

	first := renderPermDialect(t, c, permConfig(dir, &config.Permissions{Allow: []string{"Bash(git status)"}}), doc)
	require.NoError(t, os.WriteFile(doc, []byte(first.Body), 0o644))

	next := permConfig(dir, &config.Permissions{Allow: []string{"Bash(git log)"}})
	next.Run = config.NewRunState()
	next.Run.SetPreviousMerged(map[string][]docmerge.Claim{".grok/config.toml": first.Claims})
	second := renderPermDialect(t, c, next, doc)

	assert.Contains(t, second.Body, "git log")
	assert.NotContains(t, second.Body, "git status")
	assert.Contains(t, second.Body, "make test")
}

func hasWarning(warnings []string, parts ...string) bool {
	for _, w := range warnings {
		all := true
		for _, p := range parts {
			all = all && strings.Contains(w, p)
		}
		if all {
			return true
		}
	}
	return false
}

func TestDenyThatCannotBeExpressedIsReportedAsSecurity(t *testing.T) {
	tests := []struct {
		dialect string
		rule    string
		reason  string
	}{
		{"gemini", "Read(./.env)", "can only allow or block a whole tool"},
		{"gemini", "Bash(curl * | sh)", "wildcards inside a command"},
		{"codebuddy", "Skill(deploy)", "no equivalent"},
		{"letta", "mcp__evil__tool", "no equivalent"},
		{"copilot-cli", "Bash(rm -rf:*)", "no repository-level setting"},
		{"poolside", "WebFetch(domain:evil.com)", "no documented equivalent"},
		{"augment", "Read(./.env)", "path or a domain"},
		{"omp", "Read(./.env)", "per-path"},
		{"cursor", "Bash(curl * | sh)", "multi-word, exact or wildcard"},
		{"zed", "Read(./.env)", "does not gate"},
		{"vibe", "Bash(curl * | sh)", "wildcards"},
		{"copilot", "Read(./.env)", "no setting"},
		{"devin", "Bash(curl * | sh)", "wildcards inside a command"},
		{"opencode", "mcp__evil__tool", "not a documented permission key"},
	}
	for _, tt := range tests {
		t.Run(tt.dialect+" "+tt.rule, func(t *testing.T) {
			warnings := captureWarnings(t)
			dir := t.TempDir()
			cfg := permConfig(dir, &config.Permissions{Deny: []string{tt.rule}}, "copilot")

			_, err := settings.PermissionKeys(cfg, tt.dialect, "")
			require.NoError(t, err)

			assert.True(t, hasWarning(*warnings, "SECURITY", "deny rule", tt.rule, "NOT enforced", tt.reason), "got %v", *warnings)
		})
	}
}

func TestCopilotDenyIsOnlyAPrompt(t *testing.T) {
	warnings := captureWarnings(t)
	dir := t.TempDir()
	cfg := permConfig(dir, &config.Permissions{Deny: []string{"Bash(rm:*)"}}, "copilot")

	keys, err := settings.PermissionKeys(cfg, "copilot", "")
	require.NoError(t, err)

	require.Len(t, keys, 1)
	assert.Equal(t, map[string]any{"rm": false}, keys[0].Value)
	assert.True(t, hasWarning(*warnings, "SECURITY", "no hard deny"), "got %v", *warnings)
}

func TestAllowIsNeverWidened(t *testing.T) {
	tests := []struct {
		dialect string
		rule    string
	}{
		{"gemini", "Bash(git status)"},     // prefix matching would also allow `git status --force`
		{"cursor", "Bash(npm run test:*)"}, // Shell() takes a command name
		{"devin", "Bash(git status)"},
		{"codebuddy", "Skill(x)"},
		{"vibe", "Read(./src/**)"},
		{"opencode", "Write(./src/**)"},
		{"qoder", "Write(./src/**)"},
		{"grok", "Write(./src/**)"},
		{"copilot", "Write(./src/**)"},
		{"zed", "Edit(./src/**)"},
		{"opencode", "Bash(ls?)"}, // `?` is a literal for Claude and a wildcard for OpenCode
	}
	for _, tt := range tests {
		t.Run(tt.dialect+" "+tt.rule, func(t *testing.T) {
			warnings := captureWarnings(t)
			dir := t.TempDir()
			cfg := permConfig(dir, &config.Permissions{Allow: []string{tt.rule}}, "copilot")

			keys, err := settings.PermissionKeys(cfg, tt.dialect, "")
			require.NoError(t, err)

			assert.Empty(t, keys)
			assert.True(t, hasWarning(*warnings, "allow rule", tt.rule, "not generated"), "got %v", *warnings)
		})
	}
}

func TestOpencodeNeverRelaxesAnOverlappingDeny(t *testing.T) {
	captureWarnings(t)
	dir := t.TempDir()
	c := permDialectCase{"opencode", "opencode.json", docmerge.FormatJSONC}
	cfg := permConfig(dir, &config.Permissions{
		// `git *` sorts before `git status`, and the last match wins: the allow
		// would override the deny.
		Allow: []string{"Bash(git status)"},
		Deny:  []string{"Bash(git *)"},
	})

	result := renderPermDialect(t, c, cfg, filepath.Join(dir, c.path))

	assert.Contains(t, result.Body, `"git *": "deny"`)
	assert.NotContains(t, result.Body, "git status")
}

func TestOpencodeKeepsADenyTheUserWrote(t *testing.T) {
	captureWarnings(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(doc, []byte(`{"permission": {"bash": {"git *": "deny"}}}`+"\n"), 0o644))
	c := permDialectCase{"opencode", "opencode.json", docmerge.FormatJSONC}

	result := renderPermDialect(t, c, permConfig(dir, &config.Permissions{Allow: []string{"Bash(git status)"}}), doc)

	assert.NotContains(t, result.Body, "git status", "an allow that would override the user's deny is not written")
	assert.Contains(t, result.Body, `"git *": "deny"`)
}

func TestMembersTheUserSetDifferentlyAreKept(t *testing.T) {
	warnings := captureWarnings(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(doc, []byte(`{"permission": {"bash": {"ls": "deny"}}}`+"\n"), 0o644))
	c := permDialectCase{"opencode", "opencode.json", docmerge.FormatJSONC}

	result := renderPermDialect(t, c, permConfig(dir, &config.Permissions{Allow: []string{"Bash(ls)"}}), doc)

	assert.Contains(t, result.Body, `"ls": "deny"`)
	assert.True(t, hasWarning(*warnings, "already sets permission.bash.ls"), "got %v", *warnings)
}

func TestVSCodeDocumentIsTheSameForCopilotAndZooCode(t *testing.T) {
	captureWarnings(t)
	dir := t.TempDir()
	cfg := permConfig(dir, sampleRules(), "copilot", "zoocode")

	a := renderPermDialect(t, permDialectCase{"copilot", ".vscode/settings.json", docmerge.FormatJSONC}, cfg, "")
	b := renderPermDialect(t, permDialectCase{"zoocode", ".vscode/settings.json", docmerge.FormatJSONC}, cfg, "")

	assert.Equal(t, a.Body, b.Body, "two presets sharing one file must render the same document")
	assert.Contains(t, a.Body, "chat.tools.terminal.autoApprove")
	assert.Contains(t, a.Body, "zoo-code.allowedCommands")
}

func TestZooCodeAllowLongerThanDenyIsDropped(t *testing.T) {
	captureWarnings(t)
	dir := t.TempDir()
	cfg := permConfig(dir, &config.Permissions{Allow: []string{"Bash(git status:*)", "Bash(npm test:*)"}, Deny: []string{"Bash(git:*)"}}, "zoocode")

	keys, err := settings.PermissionKeys(cfg, "zoocode", "")
	require.NoError(t, err)

	require.Len(t, keys, 2)
	assert.Equal(t, []any{"npm test "}, keys[0].Value, "the longer allow prefix would override the shorter deny")
	assert.Equal(t, []any{"git"}, keys[1].Value)
}

func TestCodexRules(t *testing.T) {
	warnings := captureWarnings(t)
	cfg := permConfig(t.TempDir(), &config.Permissions{
		Allow: []string{"Bash(npm run test:*)", "Bash(git status)", "Read(./src/**)"},
		Ask:   []string{"Bash(git push:*)"},
		Deny:  []string{"Bash(rm -rf:*)", "Bash(curl * | sh)"},
	})

	body, ok := settings.CodexRules(cfg)

	require.True(t, ok)
	goldenPerm(t, "codex", body)
	assert.True(t, hasWarning(*warnings, "SECURITY", "curl * | sh", "wildcards inside a command"))
	assert.True(t, hasWarning(*warnings, "allow rule", "git status", "exact-command allow"))
	assert.True(t, hasWarning(*warnings, "allow rule", "Read(./src/**)"))

	_, ok = settings.CodexRules(permConfig(t.TempDir(), &config.Permissions{Allow: []string{"Read(x)"}}))
	assert.False(t, ok, "no expressible rule, no file")
}

func TestUnparsableRuleIsDroppedWithAWarning(t *testing.T) {
	warnings := captureWarnings(t)
	cfg := permConfig(t.TempDir(), &config.Permissions{Allow: []string{"Bash(", "Bash(ls)"}, Deny: []string{"Read("}})

	keys, err := settings.PermissionKeys(cfg, "codebuddy", "")
	require.NoError(t, err)

	require.Len(t, keys, 1)
	assert.Equal(t, []any{"Bash(ls)"}, keys[0].Value)
	assert.True(t, hasWarning(*warnings, "SECURITY", "Read("), "got %v", *warnings)
}

func TestUnsupportedDiagnosticsNamePresetsWithoutEnforcedPermissions(t *testing.T) {
	cfg := &config.Config{
		Presets:     []config.Preset{{BuiltIn: "claude"}, {BuiltIn: "codex"}, {BuiltIn: "cline"}, {BuiltIn: "pi"}, {BuiltIn: "zed"}, {BuiltIn: "mcp"}},
		Permissions: &config.Permissions{Deny: []string{"Bash(rm -rf:*)"}},
	}

	var got []string
	for _, d := range settings.UnsupportedDiagnostics(cfg) {
		if strings.Contains(d, "[permissions]") {
			got = append(got, d)
		}
	}

	require.Len(t, got, 2)
	assert.Contains(t, got[0], "not generated for cline, pi")
	assert.Contains(t, got[0], "deny rules are NOT enforced")
	assert.Contains(t, got[1], "zed with --user only")

	cfg.UserScope = true
	for _, d := range settings.UnsupportedDiagnostics(cfg) {
		assert.NotContains(t, d, "with --user only")
	}
}

// TestDocsPermissionsTable pins the harness table of docs/permissions.md to the
// translators: every harness listed is translated, and every translated harness is listed.
func TestDocsPermissionsTable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "permissions.md"))
	require.NoError(t, err)

	var listed []string
	inTable := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "| Harness | File |") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if strings.HasPrefix(cells[0], "---") {
			continue
		}
		listed = append(listed, strings.Trim(cells[0], "`"))
	}

	assert.ElementsMatch(t, settings.PermissionHarnesses(), listed)
}

func TestZooCodeAllowEqualToADenyPrefixIsDropped(t *testing.T) {
	captureWarnings(t)
	cfg := permConfig(t.TempDir(), &config.Permissions{Allow: []string{"Bash(git:*)", "Bash(GIT push:*)"}, Deny: []string{"Bash(git:*)"}}, "zoocode")

	keys, err := settings.PermissionKeys(cfg, "zoocode", "")
	require.NoError(t, err)

	require.Len(t, keys, 1, "the space-terminated allow would be the longer match and override the deny")
	assert.Equal(t, []any{"git"}, keys[0].Value)
}
