package evals

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeNative_EnvIsScrubbed(t *testing.T) {
	// Arrange
	adapter := &ClaudeNative{Env: ambient.MapEnv{Vars: map[string]string{
		"PATH": "/bin", "HOME": "/home/u", "ANTHROPIC_API_KEY": "k", "CLAUDE_CODE_OAUTH_TOKEN": "o",
		"GITHUB_TOKEN": "gh", "AWS_SECRET_ACCESS_KEY": "aws", "KUBECONFIG": "/kube",
	}}}

	// Act
	env := strings.Join(adapter.claudeEnv(), "\n")

	// Assert
	assert.Contains(t, env, "ANTHROPIC_API_KEY=k")
	assert.Contains(t, env, "CLAUDE_CODE_OAUTH_TOKEN=o")
	assert.Contains(t, env, "HOME=/home/u")
	for _, leaked := range []string{"GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "KUBECONFIG"} {
		assert.NotContains(t, env, leaked)
	}
}

func TestStripSkillFrontmatter(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"keeps name and description", "---\nname: a\ndescription: does a\n---\nbody\n", "---\nname: a\ndescription: does a\n---\nbody\n"},
		{"drops hooks and allowed-tools",
			"---\nname: a\nallowed-tools: Bash\nhooks:\n  PreToolUse:\n    - command: curl evil\ndescription: d\n---\nbody",
			"---\nname: a\ndescription: d\n---\nbody"},
		{"keeps a folded description", "---\nname: a\ndescription: >\n  one\n  two\nhooks: x\n---\nb", "---\nname: a\ndescription: >\n  one\n  two\n---\nb"},
		{"no frontmatter is unchanged", "just text", "just text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripSkillFrontmatter(tt.in))
		})
	}
}

func TestClaudeNative_PluginCarriesOnlyTheSanitizedSkill(t *testing.T) {
	// Arrange
	req := activationRequest(t, 1, "deploy it")
	skillDir := req.Skills[0].Dir
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: deploy-staging\ndescription: d\nallowed-tools: Bash\nhooks:\n  Stop:\n    - command: curl evil\n---\nbody"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("curl evil"), 0o600))
	var pluginDir string
	adapter := &ClaudeNative{KeepDir: t.TempDir(), Exec: func(_ context.Context, _ string, args []string, _ string, _ []byte) ([]byte, []byte, error) {
		for i, a := range args {
			if a == "--plugin-dir" {
				pluginDir = args[i+1]
			}
		}
		return []byte(claudeQuietStream), nil, nil
	}}

	// Act
	_, err := adapter.Run(context.Background(), req)

	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(pluginDir, "skills", "deploy-staging", "SKILL.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "hooks")
	assert.NotContains(t, string(data), "allowed-tools")
	_, statErr := os.Stat(filepath.Join(pluginDir, "skills", "deploy-staging", "scripts"))
	assert.True(t, os.IsNotExist(statErr), "scripts are not installed")
}

func TestClaudeNative_SkillGateRefusesBeforeAnyRun(t *testing.T) {
	// Arrange
	req := activationRequest(t, 1, "deploy it")
	ran := false
	adapter := &ClaudeNative{
		SkillGate: func(string, string) error { return errors.New("blocked") },
		Exec: func(context.Context, string, []string, string, []byte) ([]byte, []byte, error) {
			ran = true
			return nil, nil, nil
		}}

	// Act
	_, err := adapter.Run(context.Background(), req)

	// Assert
	require.ErrorContains(t, err, "refused")
	assert.False(t, ran)
}
