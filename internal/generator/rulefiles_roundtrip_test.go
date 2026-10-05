package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var unquotedGlobsLine = regexp.MustCompile(`(?m)^globs: [^'"].*$`)

func splitFrontmatter(t *testing.T, text string) (fm map[string]any, rest string) {
	t.Helper()
	if !strings.HasPrefix(text, "---\n") {
		return nil, text
	}
	end := strings.Index(text[4:], "\n---\n")
	require.GreaterOrEqual(t, end, 0)
	// Cursor and Devin write globs as a bare comma list, which is not valid
	// YAML when it starts with "*"; quote it for the parse.
	block := unquotedGlobsLine.ReplaceAllStringFunc(text[4:4+end+1], func(line string) string {
		return "globs: '" + strings.ReplaceAll(strings.TrimPrefix(line, "globs: "), "'", "''") + "'"
	})
	require.NoError(t, yaml.Unmarshal([]byte(block), &fm))
	return fm, text[4+end+5:]
}

func TestRender_HashInjectionRoundTrip(t *testing.T) {
	targets := []rulefiles.Target{
		{Preset: "claude", Dir: ".claude/rules", Ext: ".md", Dialect: rulefiles.DialectClaude, Banner: true},
		{Preset: "cursor", Dir: ".cursor/rules", Ext: ".mdc", Dialect: rulefiles.DialectCursor, Banner: true},
		{Preset: "devin", Dir: ".devin/rules", Ext: ".md", Dialect: rulefiles.DialectTrigger, Banner: true},
		{Preset: "copilot", Dir: ".github/instructions", Ext: ".instructions.md", Dialect: rulefiles.DialectCopilot, Banner: true},
		{Preset: "cline", Dir: ".clinerules", Ext: ".md", Dialect: rulefiles.DialectCline, Banner: true},
		{Preset: "junie", Dir: ".junie/rules", Ext: ".md", Dialect: rulefiles.DialectJunie, Banner: true},
	}
	modes := []config.ActivationMode{
		config.ActivationAlways, config.ActivationGlob, config.ActivationAuto, config.ActivationManual,
	}
	cfg := &config.Config{ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez"}
	const tricky = "Text with an arrow --> and a rule\n\n---\n\nmore after.\n\n-->\n\nend"

	for _, tg := range targets {
		for _, mode := range modes {
			t.Run(tg.Preset+"/"+string(mode), func(t *testing.T) {
				it := rulefiles.Item{
					File: config.ContentFile{
						Name: "Go Style", Path: "/p/.ai-rulez/rules/go.md", Content: tricky,
						Metadata: &config.Metadata{Priority: "high"},
					},
					ID: "Go-Style",
					Activation: config.Activation{
						Mode: mode, Globs: []string{"*.{ts,tsx}", "!vendor/**"}, Description: "use: when\nneeded",
					},
				}
				text, _, err := rulefiles.Render(tg, it, cfg)
				require.NoError(t, err)
				path := filepath.Join(t.TempDir(), filepath.FromSlash(tg.Dir), "Go-Style"+tg.Ext)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))

				body := stripHeader(text, path)
				final := injectHashes(text, path, templates.HashContent(body), "src")

				assert.Equal(t, body, stripHeader(final, path))
				assert.Contains(t, body, tricky)
				require.NoError(t, os.WriteFile(path, []byte(final), 0o600))
				contentHash, sourceHash := extractStoredHashes(path)
				assert.Equal(t, templates.HashContent(body), contentHash)
				assert.Equal(t, "src", sourceHash)

				wantFM, _ := rulefiles.Frontmatter(tg.Dialect, it)
				gotFM, rest := splitFrontmatter(t, final)
				if wantFM == nil {
					assert.Nil(t, gotFM)
				} else {
					wantYAML, err := yaml.Marshal(wantFM)
					require.NoError(t, err)
					var want map[string]any
					require.NoError(t, yaml.Unmarshal(wantYAML, &want))
					assert.Equal(t, want, gotFM)
				}
				assert.NotContains(t, rest[strings.Index(rest, "# Go Style"):], "Content-Hash")

				// Hashes live in the HTML banner; the frontmatter has no comment lines.
				if strings.HasPrefix(final, "---\n") {
					frontmatter := final[:strings.Index(final[4:], "\n---\n")+4]
					assert.NotContains(t, frontmatter, "#", "frontmatter must hold no YAML comments")
					assert.NotContains(t, frontmatter, "Hash")
				}
				banner := rest[:strings.Index(rest, "-->")]
				assert.True(t, strings.HasPrefix(strings.TrimPrefix(rest, "\n"), "<!--"), "banner follows the frontmatter")
				assert.Contains(t, banner, "Content-Hash: "+templates.HashContent(body))
				assert.Contains(t, banner, "Source-Hash: src")

				// A second generation pass renders and injects again: same bytes.
				text2, _, err := rulefiles.Render(tg, it, cfg)
				require.NoError(t, err)
				assert.Equal(t, final, injectHashes(text2, path, templates.HashContent(stripHeader(text2, path)), "src"))
			})
		}
	}
}

// Files written by earlier versions carry the hashes as YAML comments in the
// frontmatter; they must still be recognized as managed and strip to the same body.
func TestRulesDirFile_LegacyFrontmatterHashesStillManaged(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), ".claude", "rules", "legacy.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	legacy := "---\npaths:\n    - '*.go'\n# Content-Hash: abc123\n# Source-Hash: def456\n---\n" +
		"<!--\nGenerated by ai-rulez from .ai-rulez/rules/legacy.md. Edit the source, not this file.\n-->\n\n# Legacy\n\nBody.\n"
	require.NoError(t, os.WriteFile(path, []byte(legacy), 0o600))

	// Act
	contentHash, sourceHash := extractStoredHashes(path)

	// Assert
	assert.Equal(t, "abc123", contentHash)
	assert.Equal(t, "def456", sourceHash)
	assert.Equal(t, "# Legacy\n\nBody.\n", stripHeader(legacy, path))
}

func TestExtractStoredHashes_LongFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter bool
		lines       int
		wantContent string
	}{
		{"hashes after 70 frontmatter lines are found", true, 70, "deadbeef"},
		{"hashes beyond the cap in a file without frontmatter are not scanned", false, 70, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var b strings.Builder
			if tt.frontmatter {
				b.WriteString("---\n")
				for i := 0; i < tt.lines; i++ {
					b.WriteString("key" + strings.Repeat("x", i%3) + strconv.Itoa(i) + ": v\n")
				}
				b.WriteString("# Content-Hash: deadbeef\n---\n")
			} else {
				for i := 0; i < tt.lines; i++ {
					b.WriteString("line\n")
				}
				b.WriteString("Content-Hash: deadbeef\n")
			}
			path := filepath.Join(t.TempDir(), "f.md")
			require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))

			// Act
			contentHash, _ := extractStoredHashes(path)

			// Assert
			assert.Equal(t, tt.wantContent, contentHash)
		})
	}
}

func TestWriteOutput_LegacyFrontmatterHashesMigrateToBanner(t *testing.T) {
	// Arrange: a rules-folder file in the old layout whose hashes still match.
	dir := t.TempDir()
	rel := filepath.Join(".cursor", "rules", "legacy.mdc")
	body := "# Legacy\n\nBody.\n"
	banner := "<!--\nGenerated by ai-rulez from .ai-rulez/rules/legacy.md. Edit the source, not this file.\n-->\n\n"
	rendered := "---\nalwaysApply: true\n---\n" + banner + body
	hash := templates.HashContent(stripHeader(rendered, rel))
	legacy := "---\nalwaysApply: true\n# Content-Hash: " + hash + "\n# Source-Hash: src\n---\n" + banner + body
	abs := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o750))
	require.NoError(t, os.WriteFile(abs, []byte(legacy), 0o600))
	gen := NewGenerator(&config.Config{BaseDir: dir, SourceHash: "src"})
	out := config.OutputFile{Path: rel, Content: rendered}

	// Act
	require.NoError(t, gen.writeOutputs([]config.OutputFile{out}))
	migrated, err := os.ReadFile(abs)
	require.NoError(t, err)
	require.NoError(t, gen.writeOutputs([]config.OutputFile{out}))
	again, err := os.ReadFile(abs)
	require.NoError(t, err)

	// Assert
	fm, rest := splitFrontmatter(t, string(migrated))
	assert.Equal(t, map[string]any{"alwaysApply": true}, fm)
	assert.NotContains(t, string(migrated[:len(migrated)-len(rest)]), "Hash")
	assert.Contains(t, rest, "Content-Hash: "+hash)
	assert.Contains(t, rest, "Source-Hash: src")
	assert.Equal(t, string(migrated), string(again), "second run is a no-op")
	info1, err := os.Stat(abs)
	require.NoError(t, err)
	require.NoError(t, gen.writeOutputs([]config.OutputFile{out}))
	info2, err := os.Stat(abs)
	require.NoError(t, err)
	assert.Equal(t, info1.ModTime(), info2.ModTime())
}

func TestInjectHashes_RulesDirWithoutBannerGetsHashBanner(t *testing.T) {
	tests := []struct {
		name, content, path string
	}{
		{"frontmatter and no banner", "---\nalwaysApply: true\n---\n# T\n\nBody.\n", ".cursor/rules/x.mdc"},
		{"frontmatter, blank line, no banner", "---\nalwaysApply: true\n---\n\n# T\n\nBody.\n", ".cursor/rules/x.mdc"},
		{"no frontmatter and no banner", "# T\n\nBody.\n", ".claude/rules/x.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectHashes(tt.content, tt.path, "abc", "def")

			fm, rest := splitFrontmatter(t, got)
			if strings.HasPrefix(tt.content, "---") {
				assert.Equal(t, map[string]any{"alwaysApply": true}, fm, "frontmatter keeps its fields and no hash comments")
				assert.NotContains(t, got[:len(got)-len(rest)], "Hash")
			}
			assert.Contains(t, rest, "Content-Hash: abc")
			assert.Contains(t, rest, "Source-Hash: def")
			assert.True(t, hasGeneratedBanner(tt.path, []byte(got)))
			assert.Equal(t, "# T\n\nBody.\n", stripHeader(got, tt.path), "the banner is not part of the body")
			assert.Equal(t, got, injectHashes(tt.content, tt.path, "abc", "def"), "deterministic")
		})
	}

	t.Run("non markdown extension takes the generic fallback", func(t *testing.T) {
		assert.Equal(t, "plain\n", injectHashes("plain\n", ".claude/rules/x.txt", "abc", "def"))
	})
}

func TestWriteOutput_BannerlessRulesDirFile_IsStableAndManaged(t *testing.T) {
	// Arrange: a legacy provider output in a rules folder, rendered without a banner.
	dir := t.TempDir()
	rel := ".claude/rules/legacy.md"
	gen := NewGenerator(&config.Config{BaseDir: dir, SourceHash: "src"})
	out := config.OutputFile{Path: rel, Content: "# Legacy\n\nBody.\n"}

	// Act
	require.NoError(t, gen.writeOutputs([]config.OutputFile{out}))
	abs := filepath.Join(dir, rel)
	old := time.Unix(1_600_000_000, 0)
	require.NoError(t, os.Chtimes(abs, old, old))
	require.NoError(t, gen.writeOutputs([]config.OutputFile{out}))
	info, err := os.Stat(abs)
	require.NoError(t, err)

	// Assert
	assert.True(t, info.ModTime().Equal(old), "second run does not rewrite the file")
	assert.False(t, gen.skippedPaths[rel], "recognized as ours even with no manifest")
	assert.True(t, looksGenerated(abs))
}

func TestExtractStoredHashes_Robustness(t *testing.T) {
	long := strings.Repeat("x", 200*1024)
	tests := []struct {
		name, content, wantContent, wantSource string
	}{
		{"line longer than 64KB before the banner", "---\nkey: " + long + "\n---\n<!--\nContent-Hash: aaa\nSource-Hash: bbb\n-->\n", "aaa", "bbb"},
		{"CRLF line endings", "---\r\nkey: v\r\n---\r\n<!--\r\nContent-Hash: aaa\r\nSource-Hash: bbb\r\n-->\r\n", "aaa", "bbb"},
		{"no trailing newline", "<!--\nContent-Hash: aaa", "aaa", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.md")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			c, s := extractStoredHashes(path)

			assert.Equal(t, tt.wantContent, c)
			assert.Equal(t, tt.wantSource, s)
		})
	}
}
