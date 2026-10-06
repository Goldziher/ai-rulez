package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_InstalledSkillShipsVerifiers(t *testing.T) {
	// Arrange: a local installed skill with a verifiers/ directory.
	base := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(base, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	write(".ai-rulez/config.toml", "presets = [\"claude\"]\n\n[[installed_skills]]\nname = \"deploy\"\nsource = \"vendor/skills\"\npath = \"deploy\"\n")
	write("vendor/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Use when deploying the service.\n---\nDeploy.\n")
	write("vendor/skills/deploy/verifiers/checks.toml", "[[verifiers]]\nid = \"deploy-has-runbook\"\nskill = \"deploy\"\nwhen_changed = [\"deploy/**\"]\n[verifiers.require.file_exists]\npath = \"RUNBOOK.md\"\n")
	write("vendor/skills/deploy/verifiers/notes.txt", "not a declaration\n")

	// Act
	cfg, err := loadWithResolvers(context.Background(), base)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, cfg.Content)
	require.Len(t, cfg.Content.ImportedVerifiers, 1)
	got := cfg.Content.ImportedVerifiers[0]
	assert.Equal(t, "deploy", got.Include)
	assert.True(t, got.Skill)
	assert.Equal(t, "checks.toml", got.Name)
	assert.Contains(t, got.Data, "deploy-has-runbook")
}
