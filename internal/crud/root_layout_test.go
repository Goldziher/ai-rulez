package crud

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRootProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ai-rulez.toml"),
		[]byte("version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n"), 0o644))
	return dir
}

func TestRootLayoutOperatorWritesAiRulezToml(t *testing.T) {
	tests := []struct {
		name  string
		local bool
		file  string
	}{
		{"shared config", false, "ai-rulez.toml"},
		{"local overlay", true, "ai-rulez.local.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := setupRootProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)
			if tt.local {
				op = op.Local()
			}

			// Act
			err = op.InstallSkill(context.Background(), &InstallSkillRequest{Name: "demo", Source: "https://github.com/example/repo"})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, tt.file), op.ConfigFile())
			body, readErr := os.ReadFile(filepath.Join(dir, tt.file))
			require.NoError(t, readErr)
			assert.Contains(t, string(body), "demo")
			assert.NoFileExists(t, filepath.Join(dir, "config.toml"))
			assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
		})
	}
}

func TestRootLayoutOperatorRefusesContentWrites(t *testing.T) {
	// Arrange
	dir := setupRootProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// Act
	_, ruleErr := op.AddRule(ctx, &AddFileRequest{Name: "style"})
	_, skillErr := op.AddSkill(ctx, &AddFileRequest{Name: "s", Description: "d"})
	_, domainErr := op.AddDomain(ctx, &AddDomainRequest{Name: "backend"})

	// Assert
	for _, got := range []error{ruleErr, skillErr, domainErr} {
		require.Error(t, got)
		assert.Contains(t, got.Error(), "no .ai-rulez/ directory")
	}
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Len(t, entries, 1, "only ai-rulez.toml is in the project")
}
