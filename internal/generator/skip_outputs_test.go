package generator

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

const skipMCPTable = `
[[mcp_servers]]
name = "docs"
command = "echo"
args = ["hi"]
`

var skipHandWritten = map[string]string{
	"CLAUDE.md": "HAND CLAUDE\n",
	"AGENTS.md": "HAND AGENTS\n",
	"GEMINI.md": "HAND GEMINI\n",
}

func skipProject(t *testing.T, skip string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "gemini", "codex"}, "agents_md = true\n"+skip, skipMCPTable))
	return root
}

func skipGenerator(t *testing.T, root string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, config.WithResolvers(includes.Resolvers("", nil)))
	require.NoError(t, err)
	return NewGenerator(cfg)
}

func writeHandWritten(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, rel := range files {
		if content, ok := skipHandWritten[rel]; ok {
			writeAgentsMDFile(t, root, rel, content)
		}
	}
	writeAgentsMDFile(t, root, ".claude/skills/mine/SKILL.md", "HAND SKILL\n")
}

func TestSkipOutputs_ClassifiesKinds(t *testing.T) {
	tests := []struct {
		name string
		skip []string
		rel  string
		dir  bool
		want bool
	}{
		{"instructions skips CLAUDE.md", []string{"instructions"}, "CLAUDE.md", false, true},
		{"instructions skips AGENTS.md", []string{"instructions"}, "AGENTS.md", false, true},
		{"instructions skips GEMINI.md", []string{"instructions"}, "GEMINI.md", false, true},
		{"instructions skips local variant", []string{"instructions"}, "CLAUDE.local.md", false, true},
		{"instructions skips override", []string{"instructions"}, "AGENTS.override.md", false, true},
		{"instructions skips scope root", []string{"instructions"}, "packages/api/AGENTS.md", false, true},
		{"instructions skips copilot root", []string{"instructions"}, ".github/copilot-instructions.md", false, true},
		{"instructions keeps skills", []string{"instructions"}, ".claude/skills/a/SKILL.md", false, false},
		{"instructions keeps mcp", []string{"instructions"}, ".mcp.json", false, false},
		{"claude_md skips only CLAUDE.md", []string{"claude_md"}, "AGENTS.md", false, false},
		{"claude_md skips CLAUDE.md", []string{"claude_md"}, "CLAUDE.md", false, true},
		{"claude_md skips scope CLAUDE.md", []string{"claude_md"}, "packages/api/CLAUDE.md", false, true},
		{"agents_md skips AGENTS.md", []string{"agents_md"}, "AGENTS.md", false, true},
		{"agents_md keeps CLAUDE.md", []string{"agents_md"}, "CLAUDE.md", false, false},
		{"agents_md skips shared skills", []string{"agents_md"}, ".agents/skills/a/SKILL.md", false, true},
		{"agents_md skips shared skills dir", []string{"agents_md"}, ".agents/skills", true, true},
		{"agents_md keeps claude skills", []string{"agents_md"}, ".claude/skills/a/SKILL.md", false, false},
		{"rules skips rule file", []string{"rules"}, ".claude/rules/a.md", false, true},
		{"rules keeps skills", []string{"rules"}, ".claude/skills/a/SKILL.md", false, false},
		{"skills skips skill", []string{"skills"}, ".claude/skills/a/SKILL.md", false, true},
		{"skills skips resource", []string{"skills"}, ".claude/skills/a/references/r.md", false, true},
		{"agents skips agent", []string{"agents"}, ".claude/agents/a.md", false, true},
		{"commands skips command", []string{"commands"}, ".claude/commands/a.md", false, true},
		{"empty skips nothing", nil, "CLAUDE.md", false, false},
		{"skill resource named like a root file stays", []string{"instructions"}, ".claude/skills/x/references/CLAUDE.md", false, false},
		{"shared skill resource named like a root file stays", []string{"instructions"}, ".agents/skills/x/references/AGENTS.md", false, false},
		{"rule named like a root file stays", []string{"instructions"}, ".cursor/rules/AGENTS.md", false, false},
		{"docs file named like a root file stays", []string{"instructions"}, "docs/GEMINI.md", false, false},
		{"unconfigured directory root file stays", []string{"instructions"}, "packages/other/CLAUDE.md", false, false},
		{"scope local root is skipped", []string{"instructions"}, "packages/api/CLAUDE.local.md", false, true},
		{"skills skip leaves a resource named like a root file skipped", []string{"skills"}, ".claude/skills/x/references/CLAUDE.md", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := newTestRegistryConfig(t, tc.skip)

			// Act
			got := skipOutputsMatcher(cfg).skips(tc.rel, tc.dir)

			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func newTestRegistryConfig(t *testing.T, skip []string) *config.Config {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "gemini", "codex", "copilot"}, "", ""))
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	cfg.SkipOutputs = skip
	cfg.Scopes = []config.ScopeConfig{{Path: "packages/api"}}
	return cfg
}

func TestSkipOutputs_EveryPresetRootFileIsSkipped(t *testing.T) {
	// Arrange
	cfg := newTestRegistryConfig(t, []string{"instructions"})
	NewGenerator(cfg)
	require.NotNil(t, cfg.Registry)
	matcher := skipOutputsMatcher(cfg)

	// Act / Assert
	checked := 0
	cfg.Registry.Each(func(name string, gen config.PresetGenerator) {
		for _, p := range gen.GetOutputPaths("") {
			rel := filepath.ToSlash(p)
			if config.InferOutputKind(rel, "") != config.OutputKindRoot {
				continue
			}
			checked++
			assert.True(t, matcher.skips(rel, false), "%s root file %s", name, rel)
			assert.True(t, matcher.skips("packages/api/"+rel, false), "%s scope root file %s", name, rel)
		}
	})
	assert.Greater(t, checked, 10, "the registry carries the builtin presets")
}

func TestSkipOutputs_RejectsUnknownValue(t *testing.T) {
	// Arrange
	root := skipProject(t, "skip_outputs = [\"instructions\", \"bogus\"]\n")

	// Act
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	err = cfg.Validate()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestSkipOutputs_LeavesHandWrittenFilesAlone(t *testing.T) {
	tests := []struct {
		name         string
		skip         string
		untouched    []string
		written      []string
		notWritten   []string
		skillWritten bool
	}{
		{
			name:       "instructions",
			skip:       "skip_outputs = [\"instructions\", \"skills\"]\n",
			untouched:  []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md", ".claude/skills/mine/SKILL.md"},
			written:    []string{".mcp.json", ".claude/agents/helper.md"},
			notWritten: []string{".claude/skills/alpha/SKILL.md", ".agents/skills/alpha/SKILL.md"},
		},
		{
			name:       "claude_md",
			skip:       "skip_outputs = [\"claude_md\"]\n",
			untouched:  []string{"CLAUDE.md"},
			written:    []string{".mcp.json", ".claude/skills/alpha/SKILL.md"},
			notWritten: nil,
		},
		{
			name:       "agents_md",
			skip:       "skip_outputs = [\"agents_md\"]\n",
			untouched:  []string{"AGENTS.md"},
			written:    []string{".mcp.json", ".claude/skills/alpha/SKILL.md"},
			notWritten: []string{".agents/skills/alpha/SKILL.md"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			quietWarnings(t)
			root := skipProject(t, tc.skip)
			writeHandWritten(t, root, tc.untouched...)

			// Act
			gen := skipGenerator(t, root)
			require.NoError(t, gen.Generate(""))
			require.NoError(t, skipGenerator(t, root).Generate("")) // idempotent second run
			drift, err := skipGenerator(t, root).CheckDrift("")
			require.NoError(t, err)

			// Assert
			for _, rel := range tc.untouched {
				assert.Equal(t, readHand(t, rel), readAgentsMDFile(t, root, rel), rel)
			}
			for _, rel := range tc.written {
				assert.FileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
			}
			for _, rel := range tc.notWritten {
				assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
			}
			assert.Empty(t, drift)

			// Act: clean never removes the hand-written files either.
			_, err = skipGenerator(t, root).Clean("", CleanOptions{})
			require.NoError(t, err)

			// Assert
			for _, rel := range tc.untouched {
				assert.Equal(t, readHand(t, rel), readAgentsMDFile(t, root, rel), "after clean: "+rel)
			}
		})
	}
}

func readHand(t *testing.T, rel string) string {
	t.Helper()
	if content, ok := skipHandWritten[rel]; ok {
		return content
	}
	return "HAND SKILL\n"
}

func TestSkipOutputs_PreviouslyGeneratedFileStaysAndWarns(t *testing.T) {
	// Arrange
	warned := quietWarnings(t)
	root := skipProject(t, "")
	require.NoError(t, skipGenerator(t, root).Generate(""))
	before := readAgentsMDFile(t, root, "CLAUDE.md")
	require.NotEmpty(t, before)
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "gemini", "codex"}, "agents_md = true\nskip_outputs = [\"instructions\"]\n", skipMCPTable))

	// Act
	require.NoError(t, skipGenerator(t, root).Generate(""))

	// Assert
	assert.Equal(t, before, readAgentsMDFile(t, root, "CLAUDE.md"), "the earlier file is left in place")
	assert.FileExists(t, filepath.Join(root, "AGENTS.md"))
	assert.Positive(t, countContaining(*warned, "CLAUDE.md"), "warns once that it is unmanaged: %v", *warned)
	manifest, err := os.ReadFile(filepath.Join(root, ".ai-rulez", ".generated-manifest.json"))
	if err == nil {
		assert.NotContains(t, string(manifest), "CLAUDE.md")
	}
}

func TestSkipOutputs_SkipsScopeRoots(t *testing.T) {
	// Arrange
	quietWarnings(t)
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "agents_md = true\nskip_outputs = [\"instructions\"]\n",
		claudeMDScopeBlock))
	writeAgentsMDFile(t, root, ".ai-rulez/domains/api/rules/api-style.md", "# Api Style\n\nAPI_STYLE_BODY\n")
	writeAgentsMDFile(t, root, "packages/api/AGENTS.md", "HAND SCOPE\n")

	// Act
	require.NoError(t, skipGenerator(t, root).Generate(""))

	// Assert
	assert.Equal(t, "HAND SCOPE\n", readAgentsMDFile(t, root, "packages/api/AGENTS.md"))
	assert.NoFileExists(t, filepath.Join(root, "packages", "api", "CLAUDE.md"))
	assert.NoFileExists(t, filepath.Join(root, "CLAUDE.md"))
}

func TestSkipOutputs_StillEmitsInstalledSkillAndMCP(t *testing.T) {
	// Arrange
	quietWarnings(t)
	root := t.TempDir()
	// A local installed skill must stay inside the project.
	remote := filepath.Join(root, "vendor")
	writeAgentsMDFile(t, remote, "skills/ext/SKILL.md", "---\ndescription: External skill\n---\nEXT_BODY\n")
	extra := skipMCPTable + "\n[[installed_skills]]\nname = \"ext\"\nsource = " + strconv.Quote(remote) + "\n"
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "codex"}, "agents_md = true\nskip_outputs = [\"instructions\"]\n", extra))
	writeAgentsMDFile(t, root, "CLAUDE.md", "HAND CLAUDE\n")
	writeAgentsMDFile(t, root, "AGENTS.md", "HAND AGENTS\n")

	// Act
	require.NoError(t, skipGenerator(t, root).Generate(""))

	// Assert
	assert.Equal(t, "HAND CLAUDE\n", readAgentsMDFile(t, root, "CLAUDE.md"))
	assert.Equal(t, "HAND AGENTS\n", readAgentsMDFile(t, root, "AGENTS.md"))
	assert.Contains(t, readAgentsMDFile(t, root, ".claude/skills/ext/SKILL.md"), "EXT_BODY")
	assert.FileExists(t, filepath.Join(root, ".mcp.json"))
}

func TestSkipOutputs_StillEmitsSkillResourceNamedLikeARootFile(t *testing.T) {
	// Arrange
	quietWarnings(t)
	root := t.TempDir()
	extra := "\n[[installed_skills]]\nname = \"ext\"\nsource = \"./vendor\"\n"
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "agents_md = true\nskip_outputs = [\"instructions\"]\n", extra))
	writeAgentsMDFile(t, root, "vendor/skills/ext/SKILL.md", "---\ndescription: External skill\n---\nEXT_BODY\n")
	writeAgentsMDFile(t, root, "vendor/skills/ext/references/CLAUDE.md", "# nested doc\nNESTED_DOC\n")

	// Act
	require.NoError(t, skipGenerator(t, root).Generate(""))

	// Assert
	assert.Contains(t, readAgentsMDFile(t, root, ".claude/skills/ext/references/CLAUDE.md"), "NESTED_DOC")
	assert.NoFileExists(t, filepath.Join(root, "CLAUDE.md"))
}
