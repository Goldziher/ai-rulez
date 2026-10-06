package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitignorePattern_RulesDirPerFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		isDir  bool
		expect string
	}{
		{"claude rules file", ".claude/rules/x.md", false, ".claude/rules/x.md"},
		{"cursor rules file", ".cursor/rules/x.mdc", false, ".cursor/rules/x.mdc"},
		{"devin rules file", ".devin/rules/x.md", false, ".devin/rules/x.md"},
		{"cline rules file", ".clinerules/x.md", false, ".clinerules/x.md"},
		{"agents rules file", ".agents/rules/x.md", false, ".agents/rules/x.md"},
		{"junie rules file", ".junie/rules/x.md", false, ".junie/rules/x.md"},
		{"copilot instructions file", ".github/instructions/x.instructions.md", false, ".github/instructions/x.instructions.md"},
		{"nested subproject", "apps/web/.claude/rules/x.md", false, "apps/web/.claude/rules/x.md"},
		{"claude rules dir itself stays unignored", ".claude/rules", true, ""},
		{"claude skills unchanged", ".claude/skills/foo/SKILL.md", false, ".claude/skills/"},
		{"claude agents unchanged", ".claude/agents/a.md", false, ".claude/agents/"},
		{"cursor non-rules dir unchanged", ".cursor/commands/c.md", false, ".cursor/commands/"},
		{"nested skills unchanged", "apps/web/.claude/skills/foo/SKILL.md", false, "apps/web/.claude/skills/"},
		{"github agents unchanged", ".github/agents/a.md", false, ".github/agents/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, gitignorePatternForOutput(nil, tt.path, tt.isDir))
		})
	}
}

func writeManifest(t *testing.T, dir string, files ...string) {
	t.Helper()
	data, err := json.Marshal(generatedManifest{Version: "1", Files: files})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", generatedManifestName), data, 0o644))
}

func seedRuleFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	abs := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	return abs
}

func TestWriteOutput_SkipsHandAuthoredRuleFile(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{".claude/rules/x.md", ".cursor/rules/x.mdc", ".clinerules/x.md", ".github/instructions/x.instructions.md"} {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			abs := seedRuleFile(t, dir, rel, "my own rule\n")
			gen := &Generator{config: &config.Config{BaseDir: dir, SourceHash: "blake3:test"}}

			require.NoError(t, gen.writeOutput(config.OutputFile{Path: rel, Content: "generated body\n"}))

			got, err := os.ReadFile(abs)
			require.NoError(t, err)
			assert.Equal(t, "my own rule\n", string(got))
		})
	}
}

func TestWriteOutput_OverwritesManagedRuleFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rel := ".claude/rules/x.md"
	abs := seedRuleFile(t, dir, rel, "old body without any header\n")
	writeManifest(t, dir, rel)
	gen := &Generator{config: &config.Config{BaseDir: dir, SourceHash: "blake3:test"}}

	require.NoError(t, gen.writeOutput(config.OutputFile{Path: rel, Content: "new body\n"}))

	got, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Contains(t, string(got), "new body")
}

func TestWriteOutput_OverwritesFileWithGeneratedHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing string
	}{
		{"banner only", "<!--\n🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT\n-->\nold\n"},
		{"stored hash only", "<!--\nContent-Hash: blake3:abc\n-->\nold\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			rel := ".claude/rules/x.md"
			abs := seedRuleFile(t, dir, rel, tt.existing)
			gen := &Generator{config: &config.Config{BaseDir: dir, SourceHash: "blake3:test"}}

			require.NoError(t, gen.writeOutput(config.OutputFile{Path: rel, Content: "new body\n"}))

			got, err := os.ReadFile(abs)
			require.NoError(t, err)
			assert.Contains(t, string(got), "new body")
		})
	}
}

func TestWriteOutput_NonRulesDirOverwritesUnmanagedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	abs := seedRuleFile(t, dir, "docs/x.md", "hand written\n")
	gen := &Generator{config: &config.Config{BaseDir: dir, SourceHash: "blake3:test"}}

	require.NoError(t, gen.writeOutput(config.OutputFile{Path: "docs/x.md", Content: "new body\n"}))

	got, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Contains(t, string(got), "new body")
}

func newDevinProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyFixture(t, filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic"), dir)
	cfgYAML := "version = \"4.0\"\nname = \"x\"\npresets = [\"devin\"]\ngitignore = true\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(cfgYAML), 0o644))
	return dir
}

func newDevinGenerator(t *testing.T, dir string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return NewGenerator(cfg)
}

func TestGenerate_HandAuthoredRuleFile_SurvivesRepeatRunsAndClean(t *testing.T) {
	t.Parallel()

	dir := newDevinProject(t)
	rel := ".devin/rules/coding-style.md"
	abs := seedRuleFile(t, dir, rel, "my own rule\n")

	for run := 1; run <= 2; run++ {
		require.NoError(t, newDevinGenerator(t, dir).Generate("default"), "run %d", run)

		got, err := os.ReadFile(abs)
		require.NoError(t, err)
		assert.Equal(t, "my own rule\n", string(got), "run %d", run)
	}
	assert.FileExists(t, filepath.Join(dir, ".devin/rules/context-project-info.md"))

	manifest, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", generatedManifestName))
	require.NoError(t, err)
	assert.NotContains(t, string(manifest), rel)
	assert.Contains(t, string(manifest), ".devin/rules/context-project-info.md")

	ignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.NotContains(t, string(ignore), rel)
	assert.Contains(t, string(ignore), ".devin/rules/context-project-info.md")

	plan, err := newDevinGenerator(t, dir).Clean("default", CleanOptions{})
	require.NoError(t, err)
	assert.NotContains(t, plan.Files, abs)
	assert.FileExists(t, abs)
	assert.NoFileExists(t, filepath.Join(dir, ".devin/rules/context-project-info.md"))
}

func TestGenerate_RewritesDirectoryPatternInManagedGitignoreBlock(t *testing.T) {
	t.Parallel()

	dir := newDevinProject(t)
	seed := "node_modules/\n" + gitignore.BeginMarker + "\n.claude/rules/\n.devin/rules/\n" + gitignore.EndMarker + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(seed), 0o644))

	require.NoError(t, newDevinGenerator(t, dir).Generate("default"))

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	got := string(data)
	assert.Contains(t, got, "node_modules/")
	assert.NotRegexp(t, `(?m)^\.claude/rules/$`, got)
	assert.NotRegexp(t, `(?m)^\.devin/rules/$`, got)
	assert.Contains(t, got, ".devin/rules/coding-style.md")
	assert.Contains(t, got, ".devin/rules/context-project-info.md")
}

func TestWriteOutput_RuleFileOwnershipDetection(t *testing.T) {
	t.Parallel()

	longPrefix := strings.Repeat("filler line\n", 200)
	tests := []struct {
		name      string
		hashes    string
		headerTxt string
		existing  func(g *Generator, out config.OutputFile) string
		skipped   bool
	}{
		{"hand written", "", "", func(*Generator, config.OutputFile) string { return "mine\n" }, true},
		{"bytes equal rendered content, hashes none and custom header", config.HeaderHashesNone, "custom",
			func(g *Generator, out config.OutputFile) string { return g.finalContent(out) }, false},
		{"frontmatter-only file equal to rendered content", config.HeaderHashesNone, "",
			func(g *Generator, out config.OutputFile) string { return g.finalContent(out) }, false},
		{"rule banner", "", "", func(*Generator, config.OutputFile) string {
			return "---\npaths: x\n---\n<!-- Generated by ai-rulez from rules/x.md. Edit the source, not this file. -->\nold\n"
		}, false},
		{"banner marker deep in the body is not a banner", "", "", func(*Generator, config.OutputFile) string {
			return longPrefix + "Generated by ai-rulez from rules/x.md.\n"
		}, true},
		{"hand-written rule that quotes the marker", "", "", func(*Generator, config.OutputFile) string {
			return "# Docs\n\nGenerated files say GENERATED FILE at the top.\n"
		}, true},
		{"generated file banner", "", "", func(*Generator, config.OutputFile) string {
			return "<!--\n🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT\n-->\nold\n"
		}, false},
		{"banner after blank lines following frontmatter", "", "", func(*Generator, config.OutputFile) string {
			return "---\nk: v\n---\n\n\n<!--\nGenerated by ai-rulez. Edit the source.\n-->\nold\n"
		}, false},
		{"different content no marker with hashes none", config.HeaderHashesNone, "custom",
			func(*Generator, config.OutputFile) string { return "stale hand edit\n" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			cfg := &config.Config{BaseDir: dir, SourceHash: "blake3:test"}
			if tt.hashes != "" || tt.headerTxt != "" {
				cfg.Header = &config.HeaderConfig{Hashes: tt.hashes, Text: tt.headerTxt}
			}
			gen := &Generator{config: cfg, skippedPaths: map[string]bool{}}
			rel := ".claude/rules/x.md"
			out := config.OutputFile{Path: rel, Content: "new body\n"}
			seedRuleFile(t, dir, rel, tt.existing(gen, out))

			require.NoError(t, gen.writeOutput(out))

			assert.Equal(t, tt.skipped, gen.skippedPaths[rel])
		})
	}
}

func TestUnmanagedHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header *config.HeaderConfig
		hint   bool
	}{
		{"default header", nil, false},
		{"hashes none", &config.HeaderConfig{Hashes: config.HeaderHashesNone}, true},
		{"custom text", &config.HeaderConfig{Text: "mine"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gen := &Generator{config: &config.Config{Header: tt.header}}
			assert.Equal(t, tt.hint, gen.unmanagedHint() != "")
		})
	}
}
