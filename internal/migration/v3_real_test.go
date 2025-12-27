package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealProjectMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real project migration test in short mode")
	}

	// Get the project root directory
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Find project root by looking for ai-rulez.yaml
	projectRoot := filepath.Join(wd, "..", "..")
	v2ConfigPath := filepath.Join(projectRoot, "ai-rulez.yaml")

	// Check if the file exists
	if _, err := os.Stat(v2ConfigPath); os.IsNotExist(err) {
		t.Skip("Skipping real project migration test - ai-rulez.yaml not found")
		return
	}

	// Create a temporary output directory
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	// Run the migration
	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err = migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Verify the output structure was created
	aiRulezDir := filepath.Join(outputDir, ".ai-rulez")
	assert.DirExists(t, aiRulezDir)
	assert.DirExists(t, filepath.Join(aiRulezDir, "rules"))
	assert.DirExists(t, filepath.Join(aiRulezDir, "context"))
	assert.DirExists(t, filepath.Join(aiRulezDir, "skills"))

	// Verify config.yaml was created
	configPath := filepath.Join(aiRulezDir, "config.yaml")
	assert.FileExists(t, configPath)

	// Just verify the structure is valid - content may be empty
	// The real config might not have rules/sections/agents populated
	t.Logf("Successfully migrated real project config to %s", aiRulezDir)

	// Log what was migrated
	rulesDir := filepath.Join(aiRulezDir, "rules")
	contextDir := filepath.Join(aiRulezDir, "context")
	skillsDir := filepath.Join(aiRulezDir, "skills")

	rulesCount := countFiles(t, rulesDir)
	contextCount := countFiles(t, contextDir)
	skillsCount := countFiles(t, skillsDir)

	t.Logf("Migrated: %d rules, %d context files, %d skills", rulesCount, contextCount, skillsCount)
}

func countFiles(t *testing.T, dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	return count
}
