package generator

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// localRuleProject writes a project with the given presets and rules mode, a
// shared and a machine-local rule both named "x", and one local context file.
func localRuleProject(t *testing.T, preset, mode string) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	cfg := "version = \"5.0\"\nname = \"t\"\npresets = [\"" + preset + "\"]\ngitignore = true\nagents_md = false\n"
	if mode != "" {
		cfg += "\n[rules]\nmode = \"" + mode + "\"\n"
	}
	seedLocalFile(t, filepath.Join(cfgDir, "config.toml"), cfg)
	seedLocalFile(t, filepath.Join(cfgDir, "rules", "x.md"), "---\npriority: high\n---\n\nShared rule body.\n")
	seedLocalFile(t, filepath.Join(cfgDir, "local", "rules", "x.md"), "---\npriority: low\n---\n\nPersonal rule body.\n")
	seedLocalFile(t, filepath.Join(cfgDir, "local", "context", "notes.md"), "Personal context body.\n")
	return dir
}

func seedLocalFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func generateIn(t *testing.T, dir string) error {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	_, err = NewGenerator(cfg).GenerateFiles("")
	return err
}

func readRel(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func TestLocalRuleFiles_SplitMode(t *testing.T) {
	tests := []struct {
		name        string
		preset      string
		sharedFile  string
		localFile   string
		localRoot   string // "" when the preset has no local root file
		frontmatter string
		pattern     string
	}{
		{"claude", "claude", ".claude/rules/x.md", ".claude/rules/x.local.md", "CLAUDE.local.md", "", ".claude/rules/*.local.*"},
		{"junie", "junie", ".junie/rules/x.md", ".junie/rules/x.local.md", "", "", ".junie/rules/*.local.*"},
		{"cursor", "cursor", ".cursor/rules/x.mdc", ".cursor/rules/x.local.mdc", "", "alwaysApply: true", ".cursor/rules/*.local.*"},
		{"devin", "devin", ".devin/rules/x.md", ".devin/rules/x.local.md", "", "trigger: always_on", ".devin/rules/*.local.*"},
		{"cline", "cline", ".clinerules/x.md", ".clinerules/x.local.md", "", "", ".clinerules/*.local.*"},
		{"copilot", "copilot", ".github/instructions/x.instructions.md", ".github/instructions/x.local.instructions.md", "", "applyTo", ".github/instructions/*.local.*"},
		{"antigravity", "antigravity", ".agents/rules/x.md", ".agents/rules/x.local.md", "", "trigger: always_on", ".agents/rules/*.local.*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := localRuleProject(t, tt.preset, "split")

			// Act
			require.NoError(t, generateIn(t, dir))

			// Assert: shared and local files are distinct and both exist
			shared := readRel(t, dir, tt.sharedFile)
			local := readRel(t, dir, tt.localFile)
			assert.Contains(t, shared, "Shared rule body.")
			assert.NotContains(t, shared, "Personal")
			assert.Contains(t, local, "Personal rule body.")
			assert.Contains(t, local, "# x")
			assert.Contains(t, local, "ai-rulez", "banner present")
			assert.Contains(t, local, "Content-Hash")
			if tt.frontmatter != "" {
				assert.Contains(t, local, tt.frontmatter)
			}
			assert.Contains(t, readRel(t, dir, ".gitignore"), tt.pattern)
			assert.NotContains(t, readRel(t, dir, ".gitignore"), "x.local")

			// The local root keeps local context only, never the rule.
			if tt.localRoot != "" {
				root := readRel(t, dir, tt.localRoot)
				assert.Contains(t, root, "Personal context body.")
				assert.NotContains(t, root, "Personal rule body.")
			}

			// Deleting the local rule removes its file on the next run.
			require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", "local", "rules", "x.md")))
			require.NoError(t, generateIn(t, dir))
			assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(tt.localFile)))
			assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(tt.sharedFile)))
		})
	}
}

func TestLocalRuleFiles_InlineModeUnchanged(t *testing.T) {
	tests := []struct {
		name      string
		preset    string
		localFile string
		localRoot string
	}{
		{"claude", "claude", ".claude/rules/x.local.md", "CLAUDE.local.md"},
		{"junie", "junie", ".junie/rules/x.local.md", ".junie/rules/ai-rulez.local.md"},
		{"copilot", "copilot", ".github/instructions/x.local.instructions.md", ".github/instructions/ai-rulez.local.instructions.md"},
		{"antigravity", "antigravity", ".agents/rules/x.local.md", ".agents/rules/ai-rulez.local.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := localRuleProject(t, tt.preset, "inline")

			// Act
			require.NoError(t, generateIn(t, dir))

			// Assert
			assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(tt.localFile)))
			if tt.localRoot != "" {
				root := readRel(t, dir, tt.localRoot)
				assert.Contains(t, root, "Personal rule body.")
				assert.Contains(t, root, "Personal context body.")
			}
			if !config.InRulesDir(tt.localRoot) {
				assert.NotContains(t, readRel(t, dir, ".gitignore"), "*.local.*")
			}
		})
	}
}

func TestLocalRuleFiles_AlwaysFilePresetsIgnoreInlineMode(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "cursor", "inline")

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert
	assert.Contains(t, readRel(t, dir, ".cursor/rules/x.local.mdc"), "Personal rule body.")
}

func TestLocalRuleFiles_NoLocalContentChangesNothing(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "local")))

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "rules", "x.local.md"))
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.local.md"))
	assert.NotContains(t, readRel(t, dir, ".gitignore"), ".local")
}

func TestLocalRuleFiles_OnlyLocalRulesSkipsEmptyRoot(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "local", "context")))

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert
	assert.FileExists(t, filepath.Join(dir, ".claude", "rules", "x.local.md"))
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.local.md"))
}

func TestLocalRuleFiles_CollisionsAreDisambiguated(t *testing.T) {
	tests := []struct {
		name  string
		first string
		other string
	}{
		{"same id after sanitizing", "foo bar.md", "foo_bar.md"},
		{"differs only by case", "Foo-Bar.md", "foo bar.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := localRuleProject(t, "cursor", "split")
			seedLocalFile(t, filepath.Join(dir, ".ai-rulez", "local", "rules", tt.first), "A\n")
			seedLocalFile(t, filepath.Join(dir, ".ai-rulez", "local", "rules", tt.other), "B\n")

			// Act
			err := generateIn(t, dir)

			// Assert
			require.NoError(t, err)
			matches, globErr := filepath.Glob(filepath.Join(dir, ".cursor", "rules", "[Ff]oo-[Bb]ar*.local.mdc"))
			require.NoError(t, globErr)
			assert.Len(t, matches, 2, "both local rules are written, one under a suffixed name")
		})
	}
}

func setGitignore(t *testing.T, dir string, enabled bool) {
	t.Helper()
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	want := "gitignore = false"
	if enabled {
		want = "gitignore = true"
	}
	out := strings.Replace(strings.Replace(string(data), "gitignore = true", want, 1), "gitignore = false", want, 1)
	require.NoError(t, os.WriteFile(cfgPath, []byte(out), 0o600))
}

func TestLocalRuleFiles_LocalManifestKeepsCommittedManifestClean(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	setGitignore(t, dir, false)

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert: the committed manifest never names machine-local outputs
	committed := readRel(t, dir, ".ai-rulez/.generated-manifest.json")
	assert.NotContains(t, committed, ".local")
	assert.Contains(t, committed, ".claude/rules/x.md")
	local := readRel(t, dir, ".ai-rulez/.generated-manifest.local.json")
	assert.Contains(t, local, ".claude/rules/x.local.md")
	assert.Contains(t, local, "CLAUDE.local.md")
	assert.Contains(t, readRel(t, dir, ".gitignore"), ".ai-rulez/.generated-manifest.local.json")
}

func TestLocalRuleFiles_TeammateRunLeavesLocalFilesAndManifestAlone(t *testing.T) {
	// Arrange: the developer generated with local content; a teammate has the
	// committed files only, plus a hand-written file with a .local name.
	dir := localRuleProject(t, "claude", "split")
	setGitignore(t, dir, false)
	require.NoError(t, generateIn(t, dir))
	before := readRel(t, dir, ".ai-rulez/.generated-manifest.json")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "local")))
	require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", ".generated-manifest.local.json")))
	handWritten := "my own notes\n"
	seedLocalFile(t, filepath.Join(dir, ".claude", "rules", "x.local.md"), handWritten)

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert
	assert.Equal(t, before, readRel(t, dir, ".ai-rulez/.generated-manifest.json"))
	assert.Equal(t, handWritten, readRel(t, dir, ".claude/rules/x.local.md"))
}

func TestLocalRuleFiles_LegacyCommittedManifestCannotDeleteLocalFiles(t *testing.T) {
	// Arrange: an older version recorded a local rule file in the committed manifest.
	dir := localRuleProject(t, "claude", "split")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "local")))
	seedLocalFile(t, filepath.Join(dir, ".claude", "rules", "x.local.md"), "mine\n")
	writeManifest(t, dir, ".claude/rules/x.local.md", "CLAUDE.local.md")

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert
	assert.Equal(t, "mine\n", readRel(t, dir, ".claude/rules/x.local.md"))
}

func TestStaleManifest_RulesDirFileWithoutGeneratedMarkerIsKept(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "local")))
	seedLocalFile(t, filepath.Join(dir, ".claude", "rules", "old.md"), "hand written\n")
	seedLocalFile(t, filepath.Join(dir, ".claude", "rules", "gone.md"), hashedFixture(".claude/rules/gone.md", "<!--\nGenerated by ai-rulez from x. Edit the source, not this file.\n-->\n\nold\n"))
	writeManifest(t, dir, ".claude/rules/old.md", ".claude/rules/gone.md")

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert
	assert.FileExists(t, filepath.Join(dir, ".claude", "rules", "old.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "rules", "gone.md"))
}

func TestLocalRuleFiles_CleanRemovesLocalFilesViaLocalManifest(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	require.NoError(t, generateIn(t, dir))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "local")))
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)

	// Act
	plan, err := NewGenerator(cfg).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, plan.LocalManifestPath)
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "rules", "x.local.md"))
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.local.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", ".generated-manifest.local.json"))
}

func TestLocalRuleFiles_UnwritableGitignoreAbortsBeforeWriting(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	gi := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gi, []byte("node_modules/\n"), 0o400))
	t.Cleanup(func() { _ = os.Chmod(gi, 0o600) })
	if f, err := os.OpenFile(gi, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		t.Skip("running with privileges that ignore file modes")
	}

	// Act
	err := generateIn(t, dir)

	// Assert
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "rules", "x.local.md"))
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.local.md"))
}

func TestLocalRuleFiles_RoutingMatchesSharedRules(t *testing.T) {
	scoped := "---\nactivation: glob\npaths:\n  - \"**/*.go\"\n---\n\nGo only.\n"
	manual := "---\nactivation: manual\n---\n\nManual only.\n"
	tests := []struct {
		name       string
		preset     string
		mode       string
		byPreset   string // appended verbatim to the config
		rule       string
		wantFile   string // "" means no local rule file
		wantInRoot string // text expected in the local root, when one exists
	}{
		{"claude inline scoped becomes file", "claude", "inline", "", scoped, ".claude/rules/y.local.md", ""},
		{"claude inline unscoped stays inline", "claude", "inline", "", "Plain.\n", "", "Plain."},
		{"claude mode_by_preset split", "claude", "inline", "\n[rules.mode_by_preset]\nclaude = \"split\"\n", "Plain.\n", ".claude/rules/y.local.md", ""},
		{"copilot manual stays inline", "copilot", "split", "", manual, "", ""},
		{"copilot negated-only stays inline", "copilot", "split", "",
			"---\nactivation: glob\npaths:\n  - \"!vendor/**\"\n---\n\nNeg.\n", "", ""},
		{"copilot scoped in inline mode becomes file", "copilot", "inline", "", scoped, ".github/instructions/y.local.instructions.md", ""},
		{"antigravity split", "antigravity", "split", "", "Plain.\n", ".agents/rules/y.local.md", ""},
		{"antigravity demoted by gemini", "antigravity,gemini", "split", "", "Plain.\n", "", ""},
		{"antigravity explicit with gemini", "antigravity,gemini", "split", "\n[rules.mode_by_preset]\nantigravity = \"split\"\n", "Plain.\n", ".agents/rules/y.local.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, ".ai-rulez")
			presets := make([]string, 0, 2)
			for _, p := range strings.Split(tt.preset, ",") {
				presets = append(presets, strconv.Quote(p))
			}
			seedLocalFile(t, filepath.Join(cfgDir, "config.toml"),
				"version = \"5.0\"\nname = \"t\"\npresets = ["+strings.Join(presets, ", ")+"]\ngitignore = true\nagents_md = false\n\n[rules]\nmode = \""+tt.mode+"\"\n"+tt.byPreset)
			seedLocalFile(t, filepath.Join(cfgDir, "local", "rules", "y.md"), tt.rule)

			// Act
			require.NoError(t, generateIn(t, dir))

			// Assert
			files, _ := filepath.Glob(filepath.Join(dir, "*", "*", "y.local*"))
			ghFiles, _ := filepath.Glob(filepath.Join(dir, ".github", "instructions", "y.local*"))
			found := append(files, ghFiles...)
			if tt.wantFile == "" {
				assert.Empty(t, found)
			} else {
				assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(tt.wantFile)))
			}
			if tt.wantInRoot != "" {
				assert.Contains(t, readRel(t, dir, "CLAUDE.local.md"), tt.wantInRoot)
			}
		})
	}
}

func TestLocalRuleFiles_SecondRunDoesNotRewrite(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	require.NoError(t, generateIn(t, dir))
	target := filepath.Join(dir, ".claude", "rules", "x.local.md")
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(target, old, old))

	// Act
	require.NoError(t, generateIn(t, dir))

	// Assert
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old.Truncate(time.Second)) || info.ModTime().Equal(old), "file was rewritten")
}

func TestLocalRuleFiles_SharedFilesIdenticalWithAndWithoutLocalContent(t *testing.T) {
	// Arrange
	withLocal := localRuleProject(t, "claude", "split")
	without := localRuleProject(t, "claude", "split")
	require.NoError(t, os.RemoveAll(filepath.Join(without, ".ai-rulez", "local")))

	// Act
	require.NoError(t, generateIn(t, withLocal))
	require.NoError(t, generateIn(t, without))

	// Assert
	for _, rel := range []string{".claude/rules/x.md", "CLAUDE.md"} {
		assert.Equal(t, readRel(t, without, rel), readRel(t, withLocal, rel), rel)
	}
}

func TestDroppedLocalItems(t *testing.T) {
	// Arrange
	rules := []config.ContentFile{{Name: "a"}}
	ctx := []config.ContentFile{{Name: "b"}}

	// Act / Assert
	assert.Equal(t, []string{"rule a", "context b"}, droppedLocalItems(rules, ctx))
	assert.Empty(t, droppedLocalItems(nil, nil))
}

func TestTokenReport_LocalRuleFilesAreMachineLocalRuleFiles(t *testing.T) {
	// Arrange
	dir := localRuleProject(t, "claude", "split")
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)

	// Act
	report, err := NewGenerator(cfg).TokenReport(TokenReportOptions{Counter: tokens.CL100KBase()})
	require.NoError(t, err)

	// Assert
	runtime := findRuntime(t, report, "claude")
	local := findEntry(t, runtime, "machine-local rule files")
	assert.Equal(t, BucketConditional, local.Bucket)
	assert.Equal(t, 1, local.Artifacts)
	shared := findEntry(t, runtime, "provider rule files")
	assert.Equal(t, 1, shared.Artifacts)
}
