package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateShouldUseOKFSkillIdentityWhenNativeNameIsAbsent(t *testing.T) {
	source := "---\ntype: Playbook\ntitle: Deploy\ndescription: Deploy the service.\n" +
		"x-ai-rulez:\n  kind: skill\n  id: deploy\n  metadata:\n    priority: medium\n---\n\nDeploy.\n"
	for _, runtime := range []string{"claude", "cursor", "codex", "kimi", "opencode", "hermes", "agent-plugins", "copilot", "pi"} {
		t.Run(runtime, func(t *testing.T) {
			skill := writeSkill(t, "deploy", source, map[string]string{"references/guide.md": "A guide.\n"})
			skill.Metadata = &config.Metadata{OKFType: "Playbook"}
			outputs, err := Generate(&Manifest{
				Name: "probe", Version: "1.0.0", Runtimes: []string{runtime}, Skills: []config.ContentFile{skill},
			}, t.TempDir())
			require.NoError(t, err)
			var skills int
			for _, out := range outputs {
				if filepath.Base(out.Path) == skillManifestName {
					skills++
					assert.Contains(t, string(outputBytes(out)), "name: deploy\n")
					assert.Contains(t, string(outputBytes(out)), "description: Deploy the service.\n")
					assert.Contains(t, string(outputBytes(out)), "\n\nDeploy.\n")
					assert.NotContains(t, string(outputBytes(out)), "x-ai-rulez:")
				}
			}
			expected := 1
			if runtime == "hermes" {
				expected = 2
			}
			assert.Equal(t, expected, skills, "the runtime must publish the authored skill")
			assert.Contains(t, string(outputBytes(outputByPath(t, outputs, "references/guide.md"))), "A guide.\n")
			actual, err := os.ReadFile(skill.Path)
			require.NoError(t, err)
			assert.Equal(t, source, string(actual))
		})
	}
}

func TestGenerateShouldRejectInvalidOKFSkillProjection(t *testing.T) {
	for _, metadata := range []string{
		"    name: ''\n    description: Deploy.\n",
		"    name: other\n    description: Deploy.\n",
		"    name: Invalid Name\n    description: Deploy.\n",
		"    name: deploy\n    description: false\n",
	} {
		for _, runtime := range []string{"agent-plugins", "pi"} {
			t.Run(runtime+"/"+metadata, func(t *testing.T) {
				source := "---\ntype: Playbook\ntitle: Deploy\nx-ai-rulez:\n  kind: skill\n" +
					"  id: deploy\n  metadata:\n" + metadata + "---\nDeploy.\n"
				skill := writeSkill(t, "deploy", source, nil)
				skill.Metadata = &config.Metadata{OKFType: "Playbook"}
				_, err := Generate(&Manifest{
					Name: "probe", Version: "1.0.0", Runtimes: []string{runtime}, Skills: []config.ContentFile{skill},
				}, t.TempDir())
				require.ErrorContains(t, err, "invalid Agent Skills")
				actual, readErr := os.ReadFile(skill.Path)
				require.NoError(t, readErr)
				assert.Equal(t, source, string(actual))
			})
		}
	}
}
