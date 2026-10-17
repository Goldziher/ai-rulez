package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

const lintOnlySkill = "---\nname: a\ndescription: A hand-written skill that ai-rulez only lints\n---\n\nDo the thing.\n"

// lintOnlyProject is a repository with a hand-written skill and a configuration
// that declares no preset, either omitted or an explicit empty list.
func lintOnlyProject(t *testing.T, relConfig, body string) string {
	t.Helper()
	dir := testutil.CreateTempDir(t)
	gitInitAgents(t, dir)
	writeAgentsFile(t, dir, ".claude/skills/a/SKILL.md", lintOnlySkill)
	writeAgentsFile(t, dir, "AGENTS.md", rootHandAgentsMD)
	writeAgentsFile(t, dir, relConfig, body)
	return dir
}

func TestLintOnlyConfiguration(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	const lint = "\n[lint]\n"
	tests := []struct {
		name   string
		config string
		body   string
	}{
		{name: "presets omitted", config: ".ai-rulez/config.toml", body: "version = \"5.0\"\nname = \"lint\"\n" + lint},
		{name: "empty presets", config: ".ai-rulez/config.toml", body: "version = \"5.0\"\nname = \"lint\"\npresets = []\n" + lint},
		{name: "root layout", config: "ai-rulez.toml", body: "version = \"5.0\"\nname = \"lint\"\npresets = []\n" + lint},
		{name: "root layout without presets", config: "ai-rulez.toml", body: "version = \"5.0\"\nname = \"lint\"\n" + lint},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := lintOnlyProject(t, tt.config, tt.body)
			before := rootFiles(t, dir)

			// Act / Assert: the read-only commands accept the configuration
			for _, args := range [][]string{{"validate"}, {"scan"}, {"doctor"}, {"generate", "--check"}} {
				res := testutil.RunCLI(t, dir, args...)
				assert.Equal(t, 0, res.ExitCode, "%v: %s%s", args, res.Stdout, res.Stderr)
			}

			// Act: generate is a one-line no-op
			gen := testutil.RunCLI(t, dir, "generate", "--yes")
			require.Equal(t, 0, gen.ExitCode, gen.Stderr)
			assert.Contains(t, gen.Stdout+gen.Stderr, "No presets are configured")

			clean := testutil.RunCLI(t, dir, "clean", "--yes")
			assert.Equal(t, 0, clean.ExitCode, clean.Stdout+clean.Stderr)

			// Assert: nothing written, nothing removed
			assert.Equal(t, before, rootFiles(t, dir))
			assert.Equal(t, rootHandAgentsMD, readAgentsFile(t, dir, "AGENTS.md"))
			assert.Equal(t, lintOnlySkill, readAgentsFile(t, dir, ".claude/skills/a/SKILL.md"))
		})
	}
}
