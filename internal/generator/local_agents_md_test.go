package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

const localSkillBody = "---\ndescription: Mine skill\n---\nMINE_BODY\n"

// agentsMDLocalProject writes an agents_md project with a machine-local skill.
func agentsMDLocalProject(t *testing.T, extraConfig string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"codex", "cursor"}, "", extraConfig))
	writeAgentsMDFile(t, root, ".ai-rulez/local/skills/mine/SKILL.md", localSkillBody)
	return root
}

func TestAgentsMD_LocalOverlayKeepsSharedSourceHash(t *testing.T) {
	shared := []string{"AGENTS.md", ".agents/skills/alpha/SKILL.md", ".agents/skills/beta/SKILL.md"}
	tests := []struct {
		name   string
		extra  string
		shared []string
	}{
		{"root", "", shared},
		{
			"scope", "\n[[scopes]]\npath = \"packages/api\"\npresets = [\"codex\"]\n",
			append([]string{"packages/api/AGENTS.md"}, shared...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the teammate view, generated without local inputs.
			root := agentsMDLocalProject(t, tt.extra)
			generateLocalIn(t, root, "", config.WithoutLocal())
			want := map[string]string{}
			for _, rel := range tt.shared {
				want[rel] = readAgentsMDFile(t, root, rel)
			}

			// Act: generate with the local tree, then without it again.
			generateLocalIn(t, root, "")
			afterLocal := map[string]string{}
			for _, rel := range tt.shared {
				afterLocal[rel] = readAgentsMDFile(t, root, rel)
			}
			generateLocalIn(t, root, "", config.WithoutLocal())

			// Assert: shared files are byte-identical in every run.
			for _, rel := range tt.shared {
				assert.Equal(t, want[rel], afterLocal[rel], rel)
				assert.Equal(t, want[rel], readAgentsMDFile(t, root, rel), rel)
				assert.NotEmpty(t, headerHash(t, want[rel], "Source-Hash"), rel)
			}
		})
	}
}

func TestAgentsMD_LocalSkillsLandInSharedDirOnly(t *testing.T) {
	// Arrange
	root := agentsMDLocalProject(t, "")

	// Act
	generateLocalIn(t, root, "")

	// Assert: the local skill is local-only at .agents/skills, shared skills are
	// not duplicated into a consumer's own directory.
	assert.FileExists(t, filepath.Join(root, ".agents", "skills", "mine", "SKILL.md"))
	_, err := os.Stat(filepath.Join(root, ".codex", "skills", "mine"))
	assert.True(t, os.IsNotExist(err), "the local skill must not be written to codex's own directory")
	files := manifestFiles(t, root, ".generated-manifest.local.json")
	assert.Contains(t, files, ".agents/skills/mine/SKILL.md")
	for _, f := range files {
		assert.False(t, strings.HasPrefix(f, ".codex/skills/"), f)
		assert.False(t, strings.HasPrefix(f, ".agents/skills/alpha"), f)
		assert.False(t, strings.HasPrefix(f, ".agents/skills/beta"), f)
	}
	assert.NotContains(t, files, "AGENTS.md")
}

func TestAgentsMD_LocalItemsRaiseNoSpuriousDroppedWarnings(t *testing.T) {
	// Arrange
	root := agentsMDLocalProject(t, "")
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	g := NewGenerator(cfg)
	shared, err := config.LoadConfig(context.Background(), root, config.WithoutLocal())
	require.NoError(t, err)
	flat, _, err := NewGenerator(shared).collectOutputs("")
	require.NoError(t, err)
	known := map[string]bool{}
	for _, o := range flat {
		known[o.Path] = true
	}

	// Act
	dropped := g.droppedItems(cfg, perItemContent(cfg.LocalContent), known, map[string]bool{"codex": true})

	// Assert
	assert.Empty(t, dropped["codex"])
}
