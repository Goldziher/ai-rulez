package generator

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	_ "github.com/Goldziher/ai-rulez/v5/internal/includes" // registers the includes resolver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const scopedRulesConfig = `version = "4.0"
name = "mono"
presets = ["claude", "cursor", "copilot", "antigravity", "devin"]
gitignore = false

[rules]
mode = "%s"

[profiles]
api = ["api"]
web = ["web"]

[[scopes]]
path = "packages/api"
profile = "api"
presets = ["claude", "cursor", "copilot", "antigravity", "devin"]

[[scopes]]
path = "packages/web"
profile = "web"
presets = ["claude", "cursor", "copilot", "antigravity", "devin"]
`

// scopedRulesFixture is a root project with two scopes whose rules come from
// the api and web domains.
var scopedRulesFixture = map[string]string{
	"rules/root-rule.md":                      "# Root Rule\n\nROOT_BODY\n",
	"domains/api/rules/api-style.md":          "# Api Style\n\nAPI_STYLE_BODY\n",
	"domains/api/rules/api-go.md":             "---\npaths:\n  - \"**/*.go\"\n---\n# Api Go\n\nAPI_GO_BODY\n",
	"domains/api/rules/api-hint.md":           "---\nactivation: auto\ndescription: Use when touching handlers\n---\n# Api Hint\n\nAPI_HINT_BODY\n",
	"domains/api/context/api-notes.md":        "# Api Notes\n\nAPI_NOTES_BODY\n",
	"domains/web/rules/web-style.md":          "# Web Style\n\nWEB_STYLE_BODY\n",
	"domains/web/rules/web-ts.md":             "---\npaths:\n  - \"src/**/*.{ts,tsx}\"\n---\n# Web Ts\n\nWEB_TS_BODY\n",
	"domains/web/context/web-glob-context.md": "---\nglobs:\n  - \"src/**\"\n---\n# Web Glob Context\n\nWEB_CTX_BODY\n",
}

func writeScopedRulesProject(t *testing.T, mode string, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".ai-rulez")
	files := map[string]string{"config.toml": strings.Replace(scopedRulesConfig, "%s", mode, 1)}
	for k, v := range scopedRulesFixture {
		files[k] = v
	}
	for k, v := range extra {
		files[k] = v
	}
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}

// writeProject writes a project with the given config.toml and files below
// .ai-rulez (plus the shared domain fixture), returning its root.
func writeProject(t *testing.T, configTOML string, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{".ai-rulez/config.toml": configTOML}
	for k, v := range scopedRulesFixture {
		files[".ai-rulez/"+k] = v
	}
	for k, v := range extra {
		files[k] = v
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}

func generateScopedProfile(t *testing.T, root, profile string) error {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return NewGenerator(cfg).Generate(profile)
}

// copiesOf counts the rules-folder files containing body.
func copiesOf(t *testing.T, root, body string) int {
	t.Helper()
	n := 0
	for _, f := range ruleFilesUnder(t, root) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		require.NoError(t, err)
		if strings.Contains(string(b), body) {
			n++
		}
	}
	return n
}

func generateScopedProject(t *testing.T, root string) error {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return NewGenerator(cfg).Generate("default")
}

// ruleFilesUnder lists the generated files below root that sit in rules folders.
func ruleFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if config.InRulesDir(filepath.ToSlash(rel)) {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	}))
	sort.Strings(out)
	return out
}

func readAll(t *testing.T, root string, files []string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(files))
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		require.NoError(t, err)
		out[f] = string(b)
	}
	return out
}

func TestScopedRules_Placement(t *testing.T) {
	tests := []struct {
		name  string
		mode  string
		want  []string
		lines map[string][]string // file -> lines the file must contain
		// inlineIn maps a scope root file to bodies that must stay inlined in it
		inlineIn map[string][]string
	}{
		{
			name: "split mode routes every scoped rule to the root folders",
			mode: "split",
			want: []string{
				".agents/rules/context-packages-web--web-glob-context.md",
				".agents/rules/packages-api--api-go.md",
				".agents/rules/packages-api--api-hint.md",
				".agents/rules/packages-api--api-style.md",
				".agents/rules/packages-web--web-style.md",
				".agents/rules/packages-web--web-ts.md",
				".agents/rules/root-rule.md",
				".claude/rules/packages-api/api-go.md",
				".claude/rules/packages-api/api-hint.md",
				".claude/rules/packages-api/api-style.md",
				".claude/rules/packages-web/context-web-glob-context.md",
				".claude/rules/packages-web/web-style.md",
				".claude/rules/packages-web/web-ts.md",
				".claude/rules/root-rule.md",
				".cursor/rules/packages-api/api-go.mdc",
				".cursor/rules/packages-api/api-hint.mdc",
				".cursor/rules/packages-api/api-style.mdc",
				".cursor/rules/packages-api/context-api-notes.mdc",
				".cursor/rules/packages-web/context-web-glob-context.mdc",
				".cursor/rules/packages-web/web-style.mdc",
				".cursor/rules/packages-web/web-ts.mdc",
				".cursor/rules/root-rule.mdc",
				".devin/rules/context-packages-api--api-notes.md",
				".devin/rules/context-packages-web--web-glob-context.md",
				".devin/rules/packages-api--api-go.md",
				".devin/rules/packages-api--api-hint.md",
				".devin/rules/packages-api--api-style.md",
				".devin/rules/packages-web--web-style.md",
				".devin/rules/packages-web--web-ts.md",
				".devin/rules/root-rule.md",
				".github/instructions/packages-api/api-go.instructions.md",
				".github/instructions/packages-api/api-style.instructions.md",
				".github/instructions/packages-web/context-web-glob-context.instructions.md",
				".github/instructions/packages-web/web-style.instructions.md",
				".github/instructions/packages-web/web-ts.instructions.md",
				".github/instructions/root-rule.instructions.md",
			},
			lines: map[string][]string{
				".claude/rules/packages-api/api-go.md":                        {"    - packages/api/**/*.go"},
				".claude/rules/packages-api/api-style.md":                     {"    - packages/api/**"},
				".claude/rules/packages-web/web-ts.md":                        {"    - packages/web/src/**/*.{ts,tsx}"},
				".cursor/rules/packages-api/api-go.mdc":                       {"alwaysApply: false", "globs: packages/api/**/*.go"},
				".cursor/rules/packages-api/api-style.mdc":                    {"alwaysApply: false", "globs: packages/api/**"},
				".cursor/rules/packages-api/api-hint.mdc":                     {"description: Use when touching handlers"},
				".cursor/rules/root-rule.mdc":                                 {"alwaysApply: true"},
				".github/instructions/packages-api/api-style.instructions.md": {"applyTo: packages/api/**"},
				".github/instructions/packages-web/web-ts.instructions.md":    {"applyTo: packages/web/src/**/*.ts,packages/web/src/**/*.tsx"},
				".github/instructions/root-rule.instructions.md":              {"applyTo: '**'"},
				".agents/rules/packages-api--api-style.md":                    {"trigger: glob", "globs: packages/api/**"},
				".agents/rules/packages-api--api-hint.md":                     {"trigger: model_decision", "description: Use when touching handlers"},
				".devin/rules/packages-web--web-ts.md":                        {"trigger: glob", "globs: packages/web/src/**/*.ts,packages/web/src/**/*.tsx"},
				".devin/rules/context-packages-web--web-glob-context.md":      {"globs: packages/web/src/**"},
			},
			inlineIn: map[string][]string{
				"packages/api/.github/copilot-instructions.md": {"API_HINT_BODY", "API_NOTES_BODY"},
			},
		},
		{
			name: "inline mode routes only path-scoped items and inlines the rest in the scope root file",
			mode: "inline",
			want: []string{
				".agents/rules/context-packages-web--web-glob-context.md",
				".agents/rules/packages-api--api-go.md",
				".agents/rules/packages-web--web-ts.md",
				".claude/rules/packages-api/api-go.md",
				".claude/rules/packages-web/context-web-glob-context.md",
				".claude/rules/packages-web/web-ts.md",
				".cursor/rules/packages-api/api-go.mdc",
				".cursor/rules/packages-api/api-hint.mdc",
				".cursor/rules/packages-api/api-style.mdc",
				".cursor/rules/packages-api/context-api-notes.mdc",
				".cursor/rules/packages-web/context-web-glob-context.mdc",
				".cursor/rules/packages-web/web-style.mdc",
				".cursor/rules/packages-web/web-ts.mdc",
				".cursor/rules/root-rule.mdc",
				".devin/rules/context-packages-api--api-notes.md",
				".devin/rules/context-packages-web--web-glob-context.md",
				".devin/rules/packages-api--api-go.md",
				".devin/rules/packages-api--api-hint.md",
				".devin/rules/packages-api--api-style.md",
				".devin/rules/packages-web--web-style.md",
				".devin/rules/packages-web--web-ts.md",
				".devin/rules/root-rule.md",
				".github/instructions/packages-api/api-go.instructions.md",
				".github/instructions/packages-web/context-web-glob-context.instructions.md",
				".github/instructions/packages-web/web-ts.instructions.md",
			},
			lines: map[string][]string{
				".claude/rules/packages-api/api-go.md": {"    - packages/api/**/*.go"},
			},
			inlineIn: map[string][]string{
				"packages/api/CLAUDE.md":                       {"API_STYLE_BODY", "API_HINT_BODY"},
				"packages/web/CLAUDE.md":                       {"WEB_STYLE_BODY"},
				"packages/api/.github/copilot-instructions.md": {"API_STYLE_BODY", "API_HINT_BODY"},
				"packages/api/GEMINI.md":                       {"API_STYLE_BODY", "API_HINT_BODY"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := writeScopedRulesProject(t, tt.mode, nil)

			// Act
			require.NoError(t, generateScopedProject(t, root))

			// Assert
			got := ruleFilesUnder(t, root)
			assert.Equal(t, tt.want, got)
			for _, f := range got {
				assert.False(t, strings.HasPrefix(f, "packages/"), "rule file written inside a scope: %s", f)
			}
			files := readAll(t, root, got)
			for file, lines := range tt.lines {
				for _, line := range lines {
					assert.Contains(t, strings.Split(files[file], "\n"), line, "%s", file)
				}
			}
			for file, bodies := range tt.inlineIn {
				b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
				require.NoError(t, err, file)
				for _, body := range bodies {
					assert.Contains(t, string(b), body, file)
				}
			}
		})
	}
}

func TestScopedRules_IdempotentAndStaleCleanup(t *testing.T) {
	// Arrange
	root := writeScopedRulesProject(t, "split", nil)
	require.NoError(t, generateScopedProject(t, root))
	first := ruleFilesUnder(t, root)
	firstContent := readAll(t, root, first)
	manifest1, err := os.ReadFile(filepath.Join(root, ".ai-rulez", generatedManifestName))
	require.NoError(t, err)

	// Act: a second run with unchanged sources
	require.NoError(t, generateScopedProject(t, root))

	// Assert: same files, same bytes (including each Source-Hash), same manifest
	second := ruleFilesUnder(t, root)
	assert.Equal(t, first, second)
	assert.Equal(t, firstContent, readAll(t, root, second))
	manifest2, err := os.ReadFile(filepath.Join(root, ".ai-rulez", generatedManifestName))
	require.NoError(t, err)
	assert.Equal(t, string(manifest1), string(manifest2))
	for _, f := range first {
		assert.Contains(t, string(manifest1), f, "scoped rule file missing from the manifest")
	}

	t.Run("removing a scoped rule deletes its files only", func(t *testing.T) {
		require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "domains", "api", "rules", "api-go.md")))
		require.NoError(t, generateScopedProject(t, root))

		after := ruleFilesUnder(t, root)
		for _, gone := range []string{
			".claude/rules/packages-api/api-go.md", ".cursor/rules/packages-api/api-go.mdc",
			".github/instructions/packages-api/api-go.instructions.md", ".agents/rules/packages-api--api-go.md",
			".devin/rules/packages-api--api-go.md",
		} {
			assert.NotContains(t, after, gone)
		}
		assert.Contains(t, after, ".claude/rules/packages-api/api-style.md")
		assert.Contains(t, after, ".claude/rules/root-rule.md")
		assert.Len(t, after, len(first)-5)
	})

	t.Run("removing a scope deletes its files and empty subfolders", func(t *testing.T) {
		cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
		data, err := os.ReadFile(cfgPath)
		require.NoError(t, err)
		text := string(data)
		idx := strings.Index(text, "[[scopes]]\npath = \"packages/api\"")
		end := strings.Index(text[idx+1:], "[[scopes]]")
		require.True(t, idx >= 0 && end >= 0)
		require.NoError(t, os.WriteFile(cfgPath, []byte(text[:idx]+text[idx+1+end:]), 0o644))

		require.NoError(t, generateScopedProject(t, root))

		for _, f := range ruleFilesUnder(t, root) {
			assert.NotContains(t, f, "packages-api", f)
		}
		assert.NoDirExists(t, filepath.Join(root, ".claude", "rules", "packages-api"))
		assert.NoDirExists(t, filepath.Join(root, ".cursor", "rules", "packages-api"))
		assert.FileExists(t, filepath.Join(root, ".claude", "rules", "packages-web", "web-ts.md"))
	})
}

func TestScopedRules_CleanRemovesScopedFiles(t *testing.T) {
	// Arrange
	root := writeScopedRulesProject(t, "split", nil)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate("default"))
	require.NotEmpty(t, ruleFilesUnder(t, root))

	// Act
	_, err = gen.Clean("default", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, ruleFilesUnder(t, root))
	assert.NoDirExists(t, filepath.Join(root, ".claude", "rules"))
	assert.NoDirExists(t, filepath.Join(root, ".github", "instructions"))
}

func TestScopedRules_CollisionIsDisambiguated(t *testing.T) {
	// Arrange: a root rule named like the file of a prefixed scoped rule, differing in case.
	root := writeScopedRulesProject(t, "split", map[string]string{"rules/Packages-API--api-style.md": "# Clash\n\nCLASH\n"})

	// Act
	err := generateScopedProject(t, root)

	// Assert: devin's folder is flat, so the clash happens there only
	require.NoError(t, err)
	var clashing []string
	for _, f := range ruleFilesUnder(t, root) {
		if strings.HasPrefix(strings.ToLower(filepath.Base(f)), "packages-api--api-style") && strings.HasPrefix(f, ".devin/") {
			clashing = append(clashing, f)
		}
	}
	assert.Len(t, clashing, 2, "both rules are written, one under a suffixed name: %v", clashing)
}

func TestScopedRules_ScopeSlugCollisionIsAnError(t *testing.T) {
	// Arrange
	root := writeScopedRulesProject(t, "split", nil)
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(data, []byte("\n[[scopes]]\npath = \"packages-api\"\n")...), 0o644))

	// Act
	err = generateScopedProject(t, root)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "share the rule file qualifier")
}

const presetCount = 5 // claude, cursor, copilot, antigravity, devin

func TestScopedRules_DomainsInRootAreNotRepeatedInScopes(t *testing.T) {
	noProfiles := strings.Replace(strings.Replace(scopedRulesConfig, "[profiles]\napi = [\"api\"]\nweb = [\"web\"]\n", "", 1),
		`profile = "api"`+"\n", "", 1)
	noProfiles = strings.Replace(noProfiles, `profile = "web"`+"\n", "", 1)
	withBuiltin := strings.Replace(scopedRulesConfig, "gitignore = false", "gitignore = false\nbuiltins = [\"security\"]", 1)
	withInclude := scopedRulesConfig + "\n[[includes]]\nname = \"shared\"\nsource = \"./shared-inc\"\ninclude = [\"rules\"]\n" +
		"install_to = \"domains/shared\"\n"
	withInclude = strings.Replace(withInclude, "mode = \"%s\"", "mode = \"split\"", 1)
	includeFiles := map[string]string{"shared-inc/.ai-rulez/rules/shared-rule.md": "# Shared Rule\n\nSHARED_BODY\n"}

	tests := []struct {
		name    string
		config  string
		extra   map[string]string
		profile string
		// bodies maps a rule body to the number of copies expected (per preset)
		bodies map[string]int
		// absent lists files that must not exist
		absent []string
	}{
		{
			name: "no profiles: root renders every domain, scopes add nothing", config: noProfiles, profile: "default",
			bodies: map[string]int{"API_STYLE_BODY": presetCount, "WEB_STYLE_BODY": presetCount},
			absent: []string{"packages/api/CLAUDE.md", "packages/web/CLAUDE.md", "packages/api/AGENTS.md"},
		},
		{
			name: "profile run: a scope whose profile equals the root profile is skipped", config: scopedRulesConfig, profile: "web",
			bodies: map[string]int{"WEB_STYLE_BODY": presetCount, "API_STYLE_BODY": presetCount},
			absent: []string{"packages/web/CLAUDE.md"},
		},
		{
			name: "builtin domain stays at the root", config: withBuiltin, profile: "default",
			bodies: map[string]int{"API_STYLE_BODY": presetCount, "WEB_STYLE_BODY": presetCount},
		},
		{
			name: "include domain stays at the root", config: withInclude, extra: includeFiles, profile: "default",
			bodies: map[string]int{"API_STYLE_BODY": presetCount, "SHARED_BODY": presetCount},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfgText := strings.Replace(tt.config, "%s", "split", 1)
			root := writeProject(t, cfgText, tt.extra)

			// Act
			require.NoError(t, generateScopedProfile(t, root, tt.profile))

			// Assert
			for body, want := range tt.bodies {
				assert.Equal(t, want, copiesOf(t, root, body), body)
			}
			for _, f := range tt.absent {
				assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(f)))
			}
			for _, f := range ruleFilesUnder(t, root) {
				if !strings.Contains(f, "packages-") {
					continue
				}
				b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
				require.NoError(t, err)
				assert.Regexp(t, `from \.ai-rulez/domains/(api|web)/`, string(b), "scoped file %s must come from the scope's own domain", f)
			}
		})
	}
}

func TestScopedRules_DuplicatePresetsAreHarmless(t *testing.T) {
	// Arrange
	cfgText := strings.ReplaceAll(scopedRulesConfig, `presets = ["claude", "cursor", "copilot", "antigravity", "devin"]`,
		`presets = ["claude", "claude", "cursor"]`)
	root := writeScopedRulesProject(t, "split", nil)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"),
		[]byte(strings.Replace(cfgText, "%s", "split", 1)), 0o644))

	// Act
	err := generateScopedProject(t, root)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, ruleFilesUnder(t, root), ".claude/rules/packages-api/api-style.md")
}

func TestScopedRules_GlobEscapingScopeSkipsOnlyThatItem(t *testing.T) {
	// Arrange
	root := writeScopedRulesProject(t, "split", map[string]string{
		"domains/api/rules/api-escape.md": "---\npaths:\n  - \"{src,../other}/*.go\"\n---\n# Api Escape\n\nESCAPE_BODY\n",
	})

	// Act
	err := generateScopedProject(t, root)

	// Assert
	require.NoError(t, err)
	assert.Zero(t, copiesOf(t, root, "ESCAPE_BODY"))
	assert.Equal(t, presetCount, copiesOf(t, root, "API_STYLE_BODY"))
}

func TestScopedRules_InvalidScopePathsAreRejected(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"parent segment", "../outside"},
		{"nested parent segment", "packages/../../outside"},
		{"absolute", "/etc"},
		{"glob star", "packages/*"},
		{"brace", "packages/{a,b}"},
		{"negation", "!packages"},
		{"dot", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := writeScopedRulesProject(t, "split", nil)
			cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
			data, err := os.ReadFile(cfgPath)
			require.NoError(t, err)
			text := strings.Replace(string(data), `path = "packages/api"`, `path = "`+tt.path+`"`, 1)
			require.NoError(t, os.WriteFile(cfgPath, []byte(text), 0o644))

			// Act
			cfg, loadErr := config.LoadConfig(context.Background(), root)
			var validateErr error
			if loadErr == nil {
				validateErr = cfg.Validate()
			}

			// Assert: rejected by validation (or already at load) and by generation
			assert.True(t, loadErr != nil || validateErr != nil, "validate must reject %q", tt.path)
			if loadErr == nil {
				assert.Error(t, NewGenerator(cfg).Generate("default"))
			}
		})
	}
}

func TestScopedRules_CleanKeepsUserFilesAndScopeDirs(t *testing.T) {
	// Arrange
	root := writeScopedRulesProject(t, "split", nil)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate("default"))
	mine := filepath.Join(root, ".claude", "rules", "packages-api", "mine.md")
	require.NoError(t, os.WriteFile(mine, []byte("# mine\n"), 0o644))

	// Act
	plan, err := gen.Clean("default", CleanOptions{DryRun: true})
	require.NoError(t, err)
	_, err = gen.Clean("default", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, mine)
	assert.NoFileExists(t, filepath.Join(root, ".claude", "rules", "packages-api", "api-style.md"))
	assert.NoDirExists(t, filepath.Join(root, ".claude", "rules", "packages-web"))
	assert.DirExists(t, filepath.Join(root, "packages", "api"), "a scope directory is source, never pruned")
	assert.Contains(t, plan.Dirs, filepath.Join(root, ".claude", "rules", "packages-web"),
		"the dry run lists directories the clean empties")
	assert.NotContains(t, plan.Dirs, filepath.Join(root, ".claude", "rules", "packages-api"))
}

func TestScopedRules_TokenAttributionOfRootFolderFiles(t *testing.T) {
	// Arrange
	root := writeScopedRulesProject(t, "split", nil)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	collector := config.NewAnalysisCollector()
	cfg.Analysis = collector
	gen := NewGenerator(cfg)

	// Act
	_, _, err = gen.collectOutputs("default")

	// Assert
	require.NoError(t, err)
	scoped := collector.Get(filepath.Join(root, ".claude", "rules", "packages-api", "api-style.md"))
	require.NotNil(t, scoped)
	assert.Equal(t, config.OutputKindRuleFile, scoped.Kind)
	assert.Equal(t, "packages/api", scoped.Scope)
	rootFile := collector.Get(filepath.Join(root, ".claude", "rules", "root-rule.md"))
	require.NotNil(t, rootFile)
	assert.Equal(t, "", rootFile.Scope)
}

func TestValidateScopePath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"packages/api", false},
		{"apps/web/", false},
		{"my.app/sub_dir", false},
		{"", true},
		{"..", true},
		{"a/../b", true},
		{"/abs", true},
		{`\\abs`, true},
		{"a*", true},
		{"a?b", true},
		{"a[b]", true},
		{"a{b}", true},
		{"a,b", true},
		{"!a", true},
		{".", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := config.ValidateScopePath(tt.path)
			assert.Equal(t, tt.wantErr, err != nil, "%v", err)
		})
	}
}

func TestScopedRules_TargetsApplyToScopedItems(t *testing.T) {
	// Arrange
	root := writeScopedRulesProject(t, "split", map[string]string{
		"domains/api/rules/api-only-cursor.md": "---\ntargets: [cursor]\n---\n# Api Only Cursor\n\nONLY_CURSOR_BODY\n",
	})

	// Act
	require.NoError(t, generateScopedProject(t, root))

	// Assert
	var holders []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(p, ".ai-rulez") {
			return err
		}
		if b, readErr := os.ReadFile(p); readErr == nil && strings.Contains(string(b), "ONLY_CURSOR_BODY") {
			rel, _ := filepath.Rel(root, p)
			holders = append(holders, filepath.ToSlash(rel))
		}
		return nil
	}))
	assert.Equal(t, []string{".cursor/rules/packages-api/api-only-cursor.mdc"}, holders)
}
