package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// commandSkillsProject writes a project with the preset, one skill, and the given commands
// (file name to frontmatter targets line, "" for none).
func commandSkillsProject(t *testing.T, preset string, agentsMD bool, commands map[string]string) string {
	t.Helper()
	root := t.TempDir()
	cfg := "version = \"5.0\"\nname = \"cmds\"\ngitignore = false\npresets = [\"" + preset + "\"]\n"
	if !agentsMD {
		cfg += "agents_md = false\n"
	}
	files := map[string]string{
		"config.toml":          cfg,
		"rules/r.md":           "# R\n\nrule\n",
		"skills/sk/SKILL.md":   "---\nname: sk\ndescription: Skill. Use when asked.\n---\nSK\n",
		"skills/same/SKILL.md": "---\nname: same\ndescription: Same id. Use when asked.\n---\nSAME\n",
	}
	for name, targets := range commands {
		files["commands/"+name+".md"] = "---\ndescription: Command " + name + "\n" + targets + "---\nBODY-" + name + "\n"
	}
	for rel, content := range files {
		path := filepath.Join(root, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}

func generateCommandSkills(t *testing.T, root string) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(""))
}

func TestGenerate_CommandSkillsLandInTheSharedTree(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		// own is the preset's own skills directory, which agents_md replaces.
		own    string
		policy bool
	}{
		{name: "codex", preset: "codex", own: ".codex/skills", policy: true},
		{name: "antigravity", preset: "antigravity"},
		{name: "devin", preset: "devin", own: ".devin/skills"},
		{name: "warp", preset: "warp", own: ".warp/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := commandSkillsProject(t, tt.preset, true, map[string]string{"deploy": "", "same": ""})

			// Act
			generateCommandSkills(t, root)

			// Assert
			shared := filepath.Join(root, ".agents", "skills")
			body, err := os.ReadFile(filepath.Join(shared, "deploy", "SKILL.md"))
			require.NoError(t, err, "the command is a skill of the shared tree")
			assert.Contains(t, string(body), "BODY-deploy")
			assert.Contains(t, string(body), "disable-model-invocation: true")
			assert.Equal(t, tt.policy, exists(t, filepath.Join(shared, "deploy", "agents", "openai.yaml")), "codex invocation policy")
			assert.True(t, exists(t, filepath.Join(shared, "sk", "SKILL.md")), "plain skills stay")
			same, err := os.ReadFile(filepath.Join(shared, "same", "SKILL.md"))
			require.NoError(t, err)
			assert.Contains(t, string(same), "SAME", "a skill of the same id wins over the command")
			if tt.own != "" {
				assert.False(t, exists(t, filepath.Join(root, filepath.FromSlash(tt.own), "deploy", "SKILL.md")), "no copy in the preset's own directory")
			}
		})
	}
}

func TestGenerate_TargetedCommandSkillsStayOnThePresetPath(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		targets string
		want    string // path of the command skill, relative to the project
		absent  string
	}{
		{name: "codex targeted at codex", preset: "codex", targets: "targets: [codex]\n", want: ".agents/skills/deploy/SKILL.md"},
		{name: "codex targeted elsewhere", preset: "codex", targets: "targets: [claude]\n", absent: ".agents/skills/deploy/SKILL.md"},
		{name: "warp targeted at warp", preset: "warp", targets: "targets: [warp]\n", want: ".warp/skills/deploy/SKILL.md", absent: ".agents/skills/deploy/SKILL.md"},
		{name: "devin targeted elsewhere", preset: "devin", targets: "targets: [claude]\n", absent: ".devin/skills/deploy/SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := commandSkillsProject(t, tt.preset, true, map[string]string{"deploy": tt.targets})

			// Act
			generateCommandSkills(t, root)

			// Assert
			if tt.want != "" {
				assert.True(t, exists(t, filepath.Join(root, filepath.FromSlash(tt.want))), tt.want)
			}
			if tt.absent != "" {
				assert.False(t, exists(t, filepath.Join(root, filepath.FromSlash(tt.absent))), tt.absent)
			}
		})
	}
}

func TestGenerate_CommandSkillsWithoutTheSharedTreeAreUnchanged(t *testing.T) {
	for _, preset := range []string{"codex", "antigravity", "devin", "warp"} {
		t.Run(preset, func(t *testing.T) {
			// Arrange
			root := commandSkillsProject(t, preset, false, map[string]string{"deploy": ""})

			// Act
			generateCommandSkills(t, root)

			// Assert
			var found int
			for _, dir := range []string{".agents/skills", ".devin/skills", ".warp/skills"} {
				if exists(t, filepath.Join(root, filepath.FromSlash(dir), "deploy", "SKILL.md")) {
					found++
				}
			}
			assert.Equal(t, 1, found, "agents_md = false writes the command skill once, in the preset's own tree")
		})
	}
}
