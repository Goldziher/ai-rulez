package importer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

func TestConvert_WriteRecordsTheImportedNativeFiles(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	files := map[string]string{}
	for p, c := range sampleProject {
		files[p] = c
	}
	files["GEMINI.md"] = "@CLAUDE.md\n"
	writeTree(t, dir, files)

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	data, rerr := os.ReadFile(filepath.Join(dir, ".ai-rulez", generator.ConvertRecordName))
	require.NoError(t, rerr)
	var rec struct{ Files map[string]string }
	require.NoError(t, json.Unmarshal(data, &rec))
	for _, want := range []string{"CLAUDE.md", ".cursor/rules/ts.mdc", ".claude/skills/lint/SKILL.md", "GEMINI.md"} {
		assert.Contains(t, rec.Files, want)
	}
	assert.NotContains(t, rec.Files, ".mcp.json", "a merged settings file is never adopted")
}

func TestConvert_DryRunLeavesNoRecord(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, sampleProject)

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir})

	// Assert
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", generator.ConvertRecordName))
}
