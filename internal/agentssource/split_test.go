package agentssource_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const splitAgentsMD = "# Demo\n\nIntro text.\n\n## Setup Steps\n\nRun `task setup`.\n\n## Code Style\n\nUse tabs.\n\n" +
	"## Fences\n\n```sh\n## not a heading\n```\n"

func TestAgentsMDStaysOneItemByDefault(t *testing.T) {
	// Arrange
	root := t.TempDir()
	write(t, root, ".agents/ai-rulez.toml", agentsConfig)
	write(t, root, "AGENTS.md", splitAgentsMD)

	// Act
	cfg := load(t, root)

	// Assert
	assert.Equal(t, []string{"agents"}, names(cfg.Content.Context))
}

func TestAgentsMDSplitMakesOneItemPerHeading(t *testing.T) {
	for _, layout := range []struct{ name, path, body string }{
		{"config in .agents", ".agents/ai-rulez.toml", agentsConfig + "agents_md_split = true\n"},
		{"source key in .ai-rulez", ".ai-rulez/config.toml", agentsConfig + "source = \"agents\"\nagents_md_split = true\n"},
	} {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			write(t, root, layout.path, layout.body)
			write(t, root, "AGENTS.md", splitAgentsMD)
			write(t, root, ".agents/context/architecture.md", "# Architecture\n")

			// Act
			cfg := load(t, root)

			// Assert
			assert.Equal(t, []string{"agents", "agents-setup-steps", "agents-code-style", "agents-fences", "architecture"}, names(cfg.Content.Context))
			byName := map[string]string{}
			for _, f := range cfg.Content.Context {
				byName[f.Name] = f.Content
			}
			assert.Contains(t, byName["agents"], "Intro text.")
			assert.NotContains(t, byName["agents"], "Setup")
			assert.Contains(t, byName["agents-setup-steps"], "## Setup Steps")
			assert.Contains(t, byName["agents-setup-steps"], "task setup")
			assert.Contains(t, byName["agents-fences"], "## not a heading", "a heading inside a code fence does not split")
		})
	}
}

func TestAgentsMDSplitGivesRepeatedHeadingsDistinctNames(t *testing.T) {
	// Arrange
	root := t.TempDir()
	write(t, root, ".agents/ai-rulez.toml", agentsConfig+"agents_md_split = true\n")
	write(t, root, "AGENTS.md", "## Notes\n\nFirst.\n\n## Notes\n\nSecond.\n")

	// Act
	cfg := load(t, root)

	// Assert
	assert.Equal(t, []string{"agents-notes", "agents-notes-2"}, names(cfg.Content.Context))
}

func TestAgentsMDSplitRendersEverySection(t *testing.T) {
	// Arrange
	root := t.TempDir()
	write(t, root, ".agents/ai-rulez.toml", presetConfig("agents_md_split = true\n"))
	write(t, root, "AGENTS.md", splitAgentsMD)

	// Act
	generate(t, root)

	// Assert
	out := snapshot(t, root)
	for _, text := range []string{"Intro text.", "task setup", "Use tabs.", "## not a heading"} {
		assert.Contains(t, out["CLAUDE.md"], text)
		assert.Equal(t, 1, strings.Count(out["CLAUDE.md"], text), "%q is rendered once", text)
	}
	assert.Equal(t, splitAgentsMD, out["AGENTS.md"], "AGENTS.md is never written")
}
