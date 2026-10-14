package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const generatedBanner = "<!--\nAI-RULEZ :: GENERATED FILE — DO NOT EDIT\n-->\n\n# Generated\n"

// agentsLoader loads dir with a reader for the agents source that adds nothing.
func agentsLoader(dir string) Loader {
	keep := func(_ context.Context, _ *config.Config, tree *config.ContentTree) (*config.ContentTree, error) {
		return tree, nil
	}
	return func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
		return config.LoadConfig(ctx, dir, append(opts, config.WithoutRemote(), config.WithResolvers(config.Resolvers{Agents: keep}))...)
	}
}

func runSource(t *testing.T, dir string) []Finding {
	t.Helper()
	report := Run(context.Background(), Options{
		Load:     agentsLoader(dir),
		LookPath: func(name string) (string, error) { return "/bin/" + name, nil },
	})
	return byCheck(report, CheckSource)
}

func TestCheckSource(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string // substrings, one per finding
	}{
		{
			name:  "an ai-rulez project has nothing to report",
			files: map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/rules/r.md": "# R\n"},
		},
		{
			name: "a clean agents project has nothing to report",
			files: map[string]string{
				".agents/ai-rulez.toml": baseConfig, "AGENTS.md": "# Mine\n", ".agents/skills/a/SKILL.md": "---\nname: a\ndescription: A\n---\nBody\n",
			},
		},
		{
			name: "a config.toml next to .agents/ai-rulez.toml is dead",
			files: map[string]string{
				".agents/ai-rulez.toml": baseConfig, ".ai-rulez/config.toml": baseConfig,
			},
			want: []string{".ai-rulez/config.toml is ignored"},
		},
		{
			name: "the .ai-rulez content tree is not read by source = agents",
			files: map[string]string{
				".ai-rulez/config.toml": baseConfig + "source = \"agents\"\n", ".ai-rulez/rules/r.md": "# R\n", ".ai-rulez/skills/s/SKILL.md": "x",
			},
			want: []string{"2 content file(s) under .ai-rulez are not read"},
		},
		{
			name: "a generated AGENTS.md is output, not source",
			files: map[string]string{
				".agents/ai-rulez.toml": baseConfig, "AGENTS.md": generatedBanner,
			},
			want: []string{"1 generated file(s) in the source are ignored: AGENTS.md"},
		},
		{
			name: "generated files under .agents are named, whatever the kind",
			files: map[string]string{
				".agents/ai-rulez.toml":                       baseConfig,
				".agents/rules/old.md":                        generatedBanner,
				".agents/agents/old.md":                       generatedBanner,
				".agents/domains/backend/commands/old.md":     generatedBanner,
				".agents/domains/backend/skills/a/SKILL.md":   "---\nname: a\ndescription: A\n---\nBody\n",
				".agents/skills/a/references/hand-written.md": "# Mine\n",
			},
			want: []string{"3 generated file(s) in the source are ignored: .agents/agents/old.md, .agents/domains/backend/commands/old.md, .agents/rules/old.md"},
		},
		{
			name: "a project with domains and every kind is clean",
			files: map[string]string{
				".agents/ai-rulez.toml":                     baseConfig,
				".agents/agents/a.md":                       "# A\n",
				".agents/commands/c.md":                     "# C\n",
				".agents/checks/k.md":                       "# K\n",
				".agents/domains/backend/rules/r.md":        "# R\n",
				".agents/skills/a/scripts/run.sh":           "#!/bin/sh\n",
				".agents/skills/a/SKILL.md":                 "---\nname: a\ndescription: A\n---\nBody\n",
				".agents/domains/backend/context/layout.md": "# L\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := project(t, tt.files)
			// Act
			got := runSource(t, dir)
			// Assert
			if len(got) != len(tt.want) {
				t.Fatalf("findings = %+v, want %d", got, len(tt.want))
			}
			for i, want := range tt.want {
				if !strings.Contains(got[i].Message, want) {
					t.Errorf("finding %d = %q, want it to contain %q", i, got[i].Message, want)
				}
			}
		})
	}
}
