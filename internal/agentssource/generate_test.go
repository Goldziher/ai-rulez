package agentssource_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

const (
	handAgentsMD = "# Demo\n\nUse tabs. Run `task test` before you push.\n"
	handSkill    = "---\nname: review\ndescription: Review a change\n---\n\nRead the diff first.\n"
	handRule     = "---\ndescription: Go style\nglobs: [\"**/*.go\"]\n---\n\nWrap errors with oops.\n"
)

// sourceFiles is a project that keeps everything in the standards.
var sourceFiles = map[string]string{
	"AGENTS.md":                                 handAgentsMD,
	".agents/skills/review/SKILL.md":            handSkill,
	".agents/skills/review/references/guide.md": "Guide\n",
	".agents/rules/go-style.md":                 handRule,
	".agents/rules/always.md":                   "---\ndescription: Always on\n---\n\nALWAYS_ON_BODY keep functions small.\n",
}

func presetConfig(extra string) string {
	return "version = \"5.0\"\nname = \"demo\"\ngitignore = false\npresets = [\"claude\", \"cursor\", \"codex\"]\n" + extra
}

func setup(t *testing.T, cfgPath, cfgBody string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, cfgPath, cfgBody)
	for rel, content := range sourceFiles {
		write(t, root, rel, content)
	}
	return root
}

func generate(t *testing.T, root string) {
	t.Helper()
	cfg := load(t, root)
	require.NoError(t, generator.NewGenerator(cfg).Generate(""))
}

// snapshot maps every file below root to its content.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			out[filepath.ToSlash(rel)] = "-> " + target
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return out
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// layouts are the two places the configuration can live.
var layouts = []struct {
	name    string
	cfgPath string
	cfgBody string
	state   string
}{
	{"config in .agents", ".agents/ai-rulez.toml", presetConfig(""), ".agents/.ai-rulez/"},
	{"source key in .ai-rulez", ".ai-rulez/config.toml", presetConfig("source = \"agents\"\n"), ".ai-rulez/"},
}

func TestGenerateTranslatesTheSourceAndLeavesItAlone(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			root := setup(t, layout.cfgPath, layout.cfgBody)
			before := snapshot(t, root)

			// Act
			generate(t, root)
			after := snapshot(t, root)

			// Assert: every source file is byte-identical
			for rel := range sourceFiles {
				assert.Equal(t, before[rel], after[rel], "%s must not change", rel)
			}
			// nothing new lands in AGENTS.md's tree or under .agents except ai-rulez's own state
			for rel := range after {
				if _, existed := before[rel]; existed {
					continue
				}
				assert.False(t, rel == "AGENTS.md" || strings.HasSuffix(rel, "/AGENTS.md"), "unexpected %s", rel)
				if strings.HasPrefix(rel, ".agents/") {
					assert.True(t, strings.HasPrefix(rel, ".agents/.ai-rulez/"), "unexpected generated file %s", rel)
				}
			}
			// the other tools are written from the source
			assert.Contains(t, after, "CLAUDE.md")
			assert.Contains(t, after[".claude/rules/always.md"], "ALWAYS_ON_BODY", "an always-on rule reaches Claude (rules mode files)")
			assert.Contains(t, after["CLAUDE.md"], "Use tabs.", "and so does the text of the project's AGENTS.md")
			assert.NotContains(t, after["CLAUDE.md"], "@AGENTS.md", "CLAUDE.md is the full file, not the shim")
			assert.Contains(t, after[".cursor/rules/always.mdc"], "ALWAYS_ON_BODY", "an always-on rule reaches Cursor")
			for rel, content := range after {
				if strings.HasPrefix(rel, ".cursor/") {
					assert.NotContains(t, content, "Use tabs.", "%s: Cursor reads AGENTS.md itself, so it gets no copy", rel)
				}
			}
			assert.Contains(t, keys(after), ".claude/rules/go-style.md", "the scoped rule reaches Claude")
			assert.Contains(t, keys(after), ".cursor/rules/go-style.mdc", "and Cursor")
			assert.Contains(t, after, layout.state+".generated-manifest.json")
		})
	}
}

func TestInlineRulesLandInTheFullClaudeFile(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			root := setup(t, layout.cfgPath, layout.cfgBody+"\n[rules]\nmode = \"inline\"\n")

			// Act
			generate(t, root)
			first := snapshot(t, root)
			generate(t, root)

			// Assert
			assert.Contains(t, first["CLAUDE.md"], "ALWAYS_ON_BODY")
			assert.Contains(t, first["CLAUDE.md"], "Use tabs.")
			assert.Equal(t, first, snapshot(t, root), "byte-stable")
			assert.Equal(t, handAgentsMD, first["AGENTS.md"])
		})
	}
}

func TestExplicitAgentsMDOptInIsRejected(t *testing.T) {
	// Arrange
	root := setup(t, ".agents/ai-rulez.toml", presetConfig("agents_md = true\n"))

	// Act
	err := load(t, root).Validate()

	// Assert
	assert.ErrorContains(t, err, "agents_md = true cannot be combined")
}

func TestGenerateTwiceIsByteStableAndCheckIsClean(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			root := setup(t, layout.cfgPath, layout.cfgBody)
			generate(t, root)
			first := snapshot(t, root)

			// Act
			generate(t, root)
			second := snapshot(t, root)
			drift, err := generator.NewGenerator(load(t, root)).CheckDrift("")

			// Assert
			require.NoError(t, err)
			assert.Equal(t, first, second, "a second run must not change a byte: the source is never re-read as output")
			assert.Empty(t, drift, "generate --check is clean")
		})
	}
}

func TestGeneratedAGENTSMDIsNeverTheSource(t *testing.T) {
	// Arrange: a project that used source = "ai-rulez" first, so AGENTS.md is output.
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.toml", presetConfig("agents_md = true\n"))
	write(t, root, ".ai-rulez/context/overview.md", "# Overview\n\nORIGINAL_CONTEXT\n")
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).Generate(""))
	generatedMD, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(generatedMD), "ORIGINAL_CONTEXT")

	// Act: switch the source
	write(t, root, ".ai-rulez/config.toml", presetConfig("source = \"agents\"\n"))
	loaded := load(t, root)

	// Assert: the generated file feeds nothing back
	assert.Empty(t, loaded.Content.Context, "the generated AGENTS.md is not read as context")
	require.NoError(t, generator.NewGenerator(loaded).Generate(""))
	assert.NoFileExists(t, filepath.Join(root, "AGENTS.md"), "it is retired as a leftover instead of being rewritten")
	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(claude), "ORIGINAL_CONTEXT")
}

func TestCleanNeverRemovesTheSource(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			root := setup(t, layout.cfgPath, layout.cfgBody)
			generate(t, root)
			before := snapshot(t, root)

			// Act
			_, err := generator.NewGenerator(load(t, root)).Clean("", generator.CleanOptions{RemoveEdited: true})

			// Assert
			require.NoError(t, err)
			after := snapshot(t, root)
			for rel, content := range sourceFiles {
				assert.Equal(t, content, after[rel], "clean --include-edited must keep %s", rel)
			}
			assert.Contains(t, after, layout.cfgPath)
			assert.NotContains(t, after, "CLAUDE.md", "the generated shim is removed")
			assert.NotEqual(t, keys(before), keys(after))
		})
	}
}

func TestCleanKeepsSourceListedInAnOldManifest(t *testing.T) {
	// Arrange: a manifest from an earlier run lists the hand-written files.
	root := setup(t, ".ai-rulez/config.toml", presetConfig("source = \"agents\"\n"))
	write(t, root, ".ai-rulez/.generated-manifest.json", "{\"files\": [\"AGENTS.md\", \".agents/skills/review/SKILL.md\"]}\n")

	// Act
	_, err := generator.NewGenerator(load(t, root)).Clean("", generator.CleanOptions{RemoveEdited: true})

	// Assert
	require.NoError(t, err)
	after := snapshot(t, root)
	for rel, content := range sourceFiles {
		assert.Equal(t, content, after[rel], "%s survives", rel)
	}
}

func TestToolsThatReadAgentsMDGetNoCopyOfIt(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			body := strings.Replace(layout.cfgBody, `["claude", "cursor", "codex"]`, `["claude", "copilot", "gemini"]`, 1)
			root := setup(t, layout.cfgPath, body)

			// Act
			generate(t, root)
			after := snapshot(t, root)

			// Assert
			assert.Contains(t, after["CLAUDE.md"], "Use tabs.", "Claude does not read AGENTS.md")
			assert.Contains(t, after["GEMINI.md"], "Use tabs.", "Gemini's settings list only GEMINI.md in this mode")
			assert.NotContains(t, after[".github/copilot-instructions.md"], "Use tabs.", "Copilot reads AGENTS.md itself")
		})
	}
}

func TestOutputsThroughASymlinkedDirectoryNeverReachTheSource(t *testing.T) {
	tests := []struct {
		name   string
		link   string
		target string
		source string
	}{
		{"rules folder", ".claude/rules", "../.agents/rules", ".agents/rules/always.md"},
		{"one skill", ".claude/skills/review", "../../.agents/skills/review", ".agents/skills/review/SKILL.md"},
	}
	for _, tt := range tests {
		for _, layout := range layouts {
			t.Run(tt.name+"/"+layout.name, func(t *testing.T) {
				// Arrange
				root := setup(t, layout.cfgPath, layout.cfgBody)
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, tt.link)), 0o755))
				require.NoError(t, os.Symlink(tt.target, filepath.Join(root, tt.link)))
				before := snapshot(t, root)

				// Act
				generate(t, root)
				after := snapshot(t, root)

				// Assert
				for rel := range sourceFiles {
					assert.Equal(t, before[rel], after[rel], "%s must not change", rel)
				}
				for rel := range after {
					if _, existed := before[rel]; !existed && strings.HasPrefix(rel, ".agents/") {
						assert.True(t, strings.HasPrefix(rel, ".agents/.ai-rulez/"), "unexpected %s", rel)
					}
				}
			})
		}
	}
}
