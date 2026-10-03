package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentsOverride_NeverOverwritesAHandWrittenFile(t *testing.T) {
	// Arrange
	warned := captureWarnings(t)
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
	writeAgentsMDFile(t, root, "AGENTS.override.md", "MY OWN OVERRIDE\n")

	// Act
	runAgentsMDGenerate(t, root)
	_, cleanErr := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, cleanErr)
	assert.Equal(t, "MY OWN OVERRIDE\n", readAgentsMDFile(t, root, "AGENTS.override.md"), "not overwritten, not deleted")
	assert.Positive(t, countContaining(*warned, "AGENTS.override.md"), *warned)
}

func TestAgentsOverride_EmbedsTheAgentsMDWrittenToDisk(t *testing.T) {
	// Arrange: codex and xum render different AGENTS.md files; the last preset in
	// name order wins on disk, and that is the body the override has to repeat.
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex", "xum"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/codex-only.md", "---\ntargets: [\"codex\"]\n---\nCODEX_ONLY_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")

	// Act
	runAgentsMDGenerate(t, root)

	// Assert
	agents := readAgentsMDFile(t, root, "AGENTS.md")
	override := readAgentsMDFile(t, root, "AGENTS.override.md")
	assert.Contains(t, override, strings.TrimSpace(stripHeader(agents, "AGENTS.md")))
	assert.Equal(t, strings.Contains(agents, "CODEX_ONLY_BODY"), strings.Contains(override, "CODEX_ONLY_BODY"),
		"the override repeats exactly what AGENTS.md holds")
}

func TestAgentsOverride_EmbedsTheBodyWithoutItsBannerAndIsStable(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", "[header]\ntimestamp = true\nstyle = \"detailed\"\n"))
	writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
	generateAt := func(at time.Time) {
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		cfg.GeneratedAt = at
		require.NoError(t, NewGenerator(cfg).Generate(""))
	}

	// Act
	generateAt(time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC))
	first := readAgentsMDFile(t, root, "AGENTS.override.md")
	info, err := os.Stat(filepath.Join(root, "AGENTS.override.md"))
	require.NoError(t, err)
	generateAt(time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC))

	// Assert
	assert.Equal(t, 1, strings.Count(first, "Generated:"), "one banner, not the AGENTS.md one nested inside it")
	assert.Equal(t, first, readAgentsMDFile(t, root, "AGENTS.override.md"), "a new timestamp does not rewrite the file")
	again, err := os.Stat(filepath.Join(root, "AGENTS.override.md"))
	require.NoError(t, err)
	assert.Equal(t, info.ModTime(), again.ModTime())
}

func TestAgentsOverride_WarnsWhenNoAgentsMDIsProduced(t *testing.T) {
	// Arrange: codex reads the override but nothing renders an AGENTS.md.
	warned := captureWarnings(t)
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
	cfg := loadAgentsMDConfig(t, root)
	allOutputs := map[string][]config.OutputFile{}

	// Act
	NewGenerator(cfg).appendAgentsOverride(allOutputs, cfg.LocalContent, presets.AllInlineRules(cfg.LocalContent), cfg)

	// Assert
	assert.Empty(t, allOutputs, "nothing to embed, nothing written")
	assert.Positive(t, countContaining(*warned, "no AGENTS.md"), *warned)
}
