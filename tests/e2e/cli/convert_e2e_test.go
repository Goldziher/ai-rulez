package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claudeProject is a hand-written Claude Code setup: a root file, a skill, a
// command and settings with a hook, a deny rule and an allow rule.
func claudeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"CLAUDE.md":                      "# Project\n\nUse pnpm. Run tests before pushing.\n",
		".claude/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Deploy to staging.\n---\n# Deploy\n\nRun make deploy.\n",
		".claude/commands/hi.md":         "---\ndescription: Say hi\n---\nSay hi.\n",
		".claude/settings.json":          `{"permissions":{"deny":["Bash(rm -rf:*)"],"allow":["Bash(make test:*)"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo pre"}]}]}}`,
	})
	return root
}

func TestConvertE2E(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		mutate    func(t *testing.T, root string)
		wantExit  int
		wantTree  bool
		wantInOut string
	}{
		{name: "a script must choose --write or --dry-run", args: []string{"convert"}, wantExit: 1, wantInOut: "--write"},
		{name: "--dry-run writes nothing", args: []string{"convert", "--dry-run", "--format", "json"}},
		{name: "--write converts the tree", args: []string{"convert", "--write", "--format", "json"}, wantTree: true},
		{
			name: "--fail-on needs-action gates on the disabled hook", args: []string{"convert", "--dry-run", "--format", "json", "--fail-on", "needs-action"},
			wantExit: 2,
		},
		{
			name: "a secret in the source blocks the write", args: []string{"convert", "--write", "--format", "json"},
			mutate: func(t *testing.T, root string) {
				writeTree(t, root, map[string]string{"CLAUDE.md": "# Project\n\naws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\nAKIAIOSFODNN7EXAMPLE\n"})
			},
			wantExit: 2,
		},
		{name: "an unknown importer cannot run", args: []string{"convert", "--dry-run", "--from", "nope"}, wantExit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			root := claudeProject(t)
			if tt.mutate != nil {
				tt.mutate(t, root)
			}

			// Act
			res := env.run(root, tt.args...)

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			if tt.wantInOut != "" {
				assert.Contains(t, res.Stdout+res.Stderr, tt.wantInOut)
			}
			if containsArg(tt.args, "json") && tt.wantExit != 1 {
				requireJSONDoc(t, res)
			}
			_, err := os.Stat(filepath.Join(root, ".ai-rulez", "config.toml"))
			assert.Equal(t, tt.wantTree, err == nil, "a .ai-rulez/ tree exists only after a successful --write")
		})
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// TestConvertThenGenerateKeepsTheProjectE2E converts, generates and checks:
// the deny rule stays live, the hook stays disabled, generate is idempotent
// and a second convert is a no-op.
func TestConvertThenGenerateKeepsTheProjectE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root := claudeProject(t)

	// Act
	conv := env.run(root, "convert", "--write")
	require.Equal(t, 0, conv.ExitCode, conv.Stderr)
	gen := env.run(root, "generate", "--yes")
	require.Equal(t, 0, gen.ExitCode, gen.Stderr)
	check := env.run(root, "generate", "--check")
	again := env.run(root, "convert", "--write", "--format", "json")

	// Assert
	cfg, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(cfg), "deny = ['Bash(rm -rf:*)']")
	assert.Contains(t, string(cfg), "# [[hooks]]", "an imported hook is written disabled")
	assert.NotContains(t, uncommented(string(cfg)), "echo pre")
	settings, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(settings), "Bash(rm -rf:*)", "the deny rule survives the round trip")
	assert.FileExists(t, filepath.Join(root, ".claude", "skills", "deploy", "SKILL.md"))
	assert.Equal(t, 0, check.ExitCode, check.Stdout+check.Stderr)
	require.Equal(t, 0, again.ExitCode, again.Stderr)
	requireJSONDoc(t, again)
}

func uncommented(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// TestConvertKeepsClaudeCommandsE2E pins N15: convert, generate and clean must
// not lose .claude/commands/*.md.
func TestConvertKeepsClaudeCommandsE2E(t *testing.T) {
	blockedOn(t, "N15")
	// Arrange
	env := newIsoEnv(t)
	root := claudeProject(t)
	require.Equal(t, 0, env.run(root, "convert", "--write").ExitCode)
	require.Equal(t, 0, env.run(root, "generate", "--yes").ExitCode)

	// Act
	clean := env.run(root, "clean", "--yes")

	// Assert
	require.Equal(t, 0, clean.ExitCode, clean.Stderr)
	assert.FileExists(t, filepath.Join(root, ".claude", "commands", "hi.md"), "clean removed the user's own command")
}
