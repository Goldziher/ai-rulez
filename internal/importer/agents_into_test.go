package importer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentssource"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

const (
	intoAgentsMD = "# Project\n\nUse tabs.\n"
	intoSkill    = "---\nname: review\ndescription: Review code\n---\nRead the diff.\n"
)

func writeInto(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// A project that keeps AGENTS.md and .agents/skills and has some other tool files
// converts into .agents without copying what is already there.
func TestConvertIntoAgentsMigratesToTheAgentsSource(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeInto(t, root, "AGENTS.md", intoAgentsMD)
	writeInto(t, root, ".agents/skills/review/SKILL.md", intoSkill)
	writeInto(t, root, ".cursor/rules/style.mdc", "---\nglobs: \"**/*.go\"\ndescription: Go style\n---\nWrap errors.\n")
	writeInto(t, root, ".claude/agents/reviewer.md", "---\ndescription: Reviews\n---\nReview the change.\n")

	// Act
	report, err := importer.Convert(context.Background(), importer.ConvertOptions{Source: root, Into: ".agents", Write: true})

	// Assert
	require.NoError(t, err)
	assert.True(t, report.Written)
	assert.FileExists(t, filepath.Join(root, ".agents", "ai-rulez.toml"))
	assert.NoFileExists(t, filepath.Join(root, ".agents", "config.toml"))
	assert.FileExists(t, filepath.Join(root, ".agents", "rules", "style.md"), "other tools' rules move into the agents layout")
	assert.FileExists(t, filepath.Join(root, ".agents", "agents", "reviewer.md"))
	assert.NoFileExists(t, filepath.Join(root, ".agents", "context", "agents.md"), "AGENTS.md is the source in place, not copied")
	assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "review-imported"))
	assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"), "nothing is written to the .ai-rulez tree")
	agentsMD, readErr := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, readErr)
	assert.Equal(t, intoAgentsMD, string(agentsMD), "the source files are never modified")
	assert.FileExists(t, filepath.Join(root, ".agents", ".ai-rulez", ".converted.json"), "the record sits in the state subfolder")

	// the result loads as an agents project that sees everything
	cfg, loadErr := config.LoadConfig(context.Background(), root, config.WithResolvers(config.Resolvers{Agents: agentssource.Read}))
	require.NoError(t, loadErr)
	assert.True(t, cfg.AgentsLayout)
	var rules, skills, contexts, agents []string
	for _, f := range cfg.Content.Rules {
		rules = append(rules, f.Name)
	}
	for _, f := range cfg.Content.Skills {
		skills = append(skills, f.Name)
	}
	for _, f := range cfg.Content.Context {
		contexts = append(contexts, f.Name)
	}
	for _, f := range cfg.Content.Agents {
		agents = append(agents, f.Name)
	}
	assert.Equal(t, []string{"style"}, rules)
	assert.Equal(t, []string{"review"}, skills)
	assert.Equal(t, []string{"agents"}, contexts, "AGENTS.md is read as context")
	assert.Equal(t, []string{"reviewer"}, agents)
	require.NoError(t, cfg.Validate())
}

func TestConvertIntoAgentsAcceptsTheStateDirectoryAndRerunsCleanly(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeInto(t, root, ".cursor/rules/style.mdc", "---\nglobs: \"**/*.go\"\n---\nWrap errors.\n")
	opts := importer.ConvertOptions{Source: root, Into: ".agents/.ai-rulez", Write: true}

	// Act
	_, err := importer.Convert(context.Background(), opts)
	require.NoError(t, err)
	second, secondErr := importer.Convert(context.Background(), opts)

	// Assert
	require.NoError(t, secondErr)
	assert.FileExists(t, filepath.Join(root, ".agents", "ai-rulez.toml"))
	assert.FileExists(t, filepath.Join(root, ".agents", "rules", "style.md"))
	for _, f := range second.Files {
		assert.Equal(t, importer.ActionUnchanged, f.Action, "%s is unchanged on a rerun", f.Path)
	}
}
