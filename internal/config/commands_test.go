package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanCommands_FlatForm tests that flat .md command files still work
func TestScanCommands_FlatForm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commandsDir := filepath.Join(dir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	// Create flat command files
	require.NoError(t, os.WriteFile(
		filepath.Join(commandsDir, "quick.md"),
		[]byte("# Quick command\n\nDoes something quickly.\n"),
		0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(commandsDir, "slow.md"),
		[]byte("# Slow command\n\nDoes something slowly.\n"),
		0o644))

	commands, err := scanCommands(commandsDir)
	require.NoError(t, err)
	require.Len(t, commands, 2)

	// Names should be based on filename without extension
	names := []string{commands[0].Name, commands[1].Name}
	assert.Contains(t, names, "quick")
	assert.Contains(t, names, "slow")
}

// TestScanCommands_DirectoryForm tests the new directory form with COMMAND.md
func TestScanCommands_DirectoryForm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commandsDir := filepath.Join(dir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	// Create directory-form command
	cmdDir := filepath.Join(commandsDir, "playwright-rca")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(cmdDir, "COMMAND.md"),
		[]byte("# Playwright RCA\n\nAnalyze test failures.\n"),
		0o644))

	// Create references subdirectory
	refDir := filepath.Join(cmdDir, "references")
	require.NoError(t, os.MkdirAll(refDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(refDir, "rca-matrix.md"),
		[]byte("# RCA Matrix\n\nFailure patterns.\n"),
		0o644))

	commands, err := scanCommands(commandsDir)
	require.NoError(t, err)
	require.Len(t, commands, 1)

	// Name should be the directory name, not the filename
	assert.Equal(t, "playwright-rca", commands[0].Name)
	assert.Contains(t, commands[0].Content, "Analyze test failures")

	// Resources should be populated
	require.NotEmpty(t, commands[0].Resources)
	assert.Equal(t, "references/rca-matrix.md", commands[0].Resources[0].RelPath)
}

// TestScanCommands_MixedForms tests that both forms work together
func TestScanCommands_MixedForms(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commandsDir := filepath.Join(dir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	// Flat form
	require.NoError(t, os.WriteFile(
		filepath.Join(commandsDir, "quick.md"),
		[]byte("Quick command\n"),
		0o644))

	// Directory form
	cmdDir := filepath.Join(commandsDir, "complex")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(cmdDir, "COMMAND.md"),
		[]byte("Complex command\n"),
		0o644))

	commands, err := scanCommands(commandsDir)
	require.NoError(t, err)
	require.Len(t, commands, 2)

	names := []string{commands[0].Name, commands[1].Name}
	assert.Contains(t, names, "quick")
	assert.Contains(t, names, "complex")
}

// TestScanCommands_DirectoryWithoutMarkerFile tests that directories without COMMAND.md are skipped
func TestScanCommands_DirectoryWithoutMarkerFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commandsDir := filepath.Join(dir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	// Directory without COMMAND.md (should be skipped)
	emptyDir := filepath.Join(commandsDir, "empty-dir")
	require.NoError(t, os.MkdirAll(emptyDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(emptyDir, "other.md"),
		[]byte("Not a command marker\n"),
		0o644))

	// Valid flat command
	require.NoError(t, os.WriteFile(
		filepath.Join(commandsDir, "valid.md"),
		[]byte("Valid command\n"),
		0o644))

	commands, err := scanCommands(commandsDir)
	require.NoError(t, err)
	require.Len(t, commands, 1)
	assert.Equal(t, "valid", commands[0].Name)
}

// TestScanCommands_NonExistentDirectory tests that missing directory returns empty slice
func TestScanCommands_NonExistentDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commandsDir := filepath.Join(dir, "nonexistent-commands")

	commands, err := scanCommands(commandsDir)
	require.NoError(t, err)
	assert.Empty(t, commands)
}

// TestScanCommands_DomainCommands tests that domain commands work with directory form
func TestScanCommands_DomainCommands(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	domainDir := filepath.Join(dir, "domains", "qa")
	commandsDir := filepath.Join(domainDir, "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	// Directory-form command in a domain
	cmdDir := filepath.Join(commandsDir, "playwright-write-test")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(cmdDir, "COMMAND.md"),
		[]byte("Write Playwright test\n"),
		0o644))

	// Add references
	refDir := filepath.Join(cmdDir, "references")
	require.NoError(t, os.MkdirAll(refDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(refDir, "patterns.md"),
		[]byte("Test patterns\n"),
		0o644))

	commands, err := scanCommands(commandsDir)
	require.NoError(t, err)
	require.Len(t, commands, 1)
	assert.Equal(t, "playwright-write-test", commands[0].Name)
	require.NotEmpty(t, commands[0].Resources)
}
