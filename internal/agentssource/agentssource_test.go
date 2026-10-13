package agentssource_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentssource"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const (
	agentsConfig = "version = \"5.0\"\nname = \"demo\"\npresets = [\"claude\"]\n"
	generatedMD  = "<!--\nAI-RULEZ :: GENERATED FILE — DO NOT EDIT\n-->\n\n# Generated\n"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func load(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, config.WithResolvers(config.Resolvers{Agents: agentssource.Read}))
	require.NoError(t, err)
	return cfg
}

func names(files []config.ContentFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

func TestReadBuildsTheContentTreeFromTheStandards(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantContext []string
		wantSkills  []string
		wantRules   []string
	}{
		{
			name: "AGENTS.md becomes context and skills come from .agents/skills",
			files: map[string]string{
				"AGENTS.md":                       "# Project\n\nUse tabs.\n",
				".agents/skills/review/SKILL.md":  "---\nname: review\ndescription: Review code\n---\nBody\n",
				".agents/rules/style.md":          "# Style\n",
				".agents/context/architecture.md": "# Architecture\n",
			},
			wantContext: []string{"agents", "architecture"},
			wantSkills:  []string{"review"},
			wantRules:   []string{"style"},
		},
		{
			name:        "no AGENTS.md",
			files:       map[string]string{".agents/rules/style.md": "# Style\n"},
			wantContext: []string{},
			wantSkills:  []string{},
			wantRules:   []string{"style"},
		},
		{
			name: "a generated AGENTS.md is never read as source",
			files: map[string]string{
				"AGENTS.md":              generatedMD,
				".agents/rules/style.md": "# Style\n",
			},
			wantContext: []string{},
			wantSkills:  []string{},
			wantRules:   []string{"style"},
		},
		{
			name: "generated files under .agents are dropped",
			files: map[string]string{
				"AGENTS.md":                      "# Mine\n",
				".agents/rules/generated.md":     generatedMD,
				".agents/rules/style.md":         "# Style\n",
				".agents/skills/old/SKILL.md":    "---\nname: old\ndescription: Old\n---\n" + generatedMD,
				".agents/skills/review/SKILL.md": "---\nname: review\ndescription: Review\n---\nBody\n",
			},
			wantContext: []string{"agents"},
			wantSkills:  []string{"review"},
			wantRules:   []string{"style"},
		},
		{
			name: "a context file named agents does not collide with AGENTS.md",
			files: map[string]string{
				"AGENTS.md":                 "# Root\n",
				".agents/context/agents.md": "# Other\n",
			},
			wantContext: []string{"agents-md", "agents"},
			wantSkills:  []string{},
			wantRules:   []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			write(t, root, ".agents/ai-rulez.toml", agentsConfig)
			for rel, content := range tt.files {
				write(t, root, rel, content)
			}

			// Act
			cfg := load(t, root)

			// Assert
			assert.ElementsMatch(t, tt.wantContext, names(cfg.Content.Context))
			assert.ElementsMatch(t, tt.wantSkills, names(cfg.Content.Skills))
			assert.ElementsMatch(t, tt.wantRules, names(cfg.Content.Rules))
		})
	}
}

func TestReadKeepsAGENTSMDFrontmatterOutOfTheBody(t *testing.T) {
	// Arrange
	root := t.TempDir()
	write(t, root, ".agents/ai-rulez.toml", agentsConfig)
	write(t, root, "AGENTS.md", "---\ndescription: Project rules\n---\n# Project\n")

	// Act
	cfg := load(t, root)

	// Assert
	require.Len(t, cfg.Content.Context, 1)
	assert.Equal(t, "# Project\n", cfg.Content.Context[0].Content)
	assert.Equal(t, filepath.Join(cfg.BaseDir, "AGENTS.md"), cfg.Content.Context[0].Path)
}

func TestReadWorksForASourceKeyInTheAIRulezConfig(t *testing.T) {
	// Arrange
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.toml", agentsConfig+"source = \"agents\"\n")
	write(t, root, ".ai-rulez/context/ignored.md", "# ignored\n")
	write(t, root, "AGENTS.md", "# Root\n")

	// Act
	cfg := load(t, root)

	// Assert
	assert.Equal(t, []string{"agents"}, names(cfg.Content.Context))
	assert.False(t, cfg.AgentsLayout)
}

func TestSymlinkedAGENTSMDIsNeverFollowed(t *testing.T) {
	// Arrange: unlike a file under .agents, the root AGENTS.md is not read through a link, even inside the project
	root := t.TempDir()
	write(t, root, ".agents/ai-rulez.toml", "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
	write(t, root, "docs/agents.md", "# Real\n\nText.\n")
	testutil.SymlinkOrSkip(t, "docs/agents.md", filepath.Join(root, "AGENTS.md"))

	// Act
	cfg := load(t, root)

	// Assert
	assert.NotContains(t, names(cfg.Content.Context), "agents")
}
