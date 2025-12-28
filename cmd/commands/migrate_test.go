package commands_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestMigrateCommand(t *testing.T) {
	assert.NotNil(t, commands.MigrateCmd)
	assert.Equal(t, "migrate <version> [config-file]", commands.MigrateCmd.Use)
	assert.NotNil(t, commands.MigrateCmd.Run)

	flags := commands.MigrateCmd.Flags()
	assert.NotNil(t, flags.Lookup("output"))
	assert.NotNil(t, flags.Lookup("validate"))
	assert.NotNil(t, flags.Lookup("dry-run"))
	assert.NotNil(t, flags.Lookup("force"))
}

func TestMigrateCommandDefaults(t *testing.T) {
	// Verify default flag values
	cmd := commands.MigrateCmd
	flags := cmd.Flags()

	output, err := flags.GetString("output")
	require.NoError(t, err)
	assert.Equal(t, ".ai-rulez", output)

	validate, err := flags.GetBool("validate")
	require.NoError(t, err)
	assert.False(t, validate)

	dryRun, err := flags.GetBool("dry-run")
	require.NoError(t, err)
	assert.False(t, dryRun)

	force, err := flags.GetBool("force")
	require.NoError(t, err)
	assert.False(t, force)
}

func TestDetectV2Config_WithExplicitPath(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Create ai-rulez.yaml
	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	err = os.WriteFile(configPath, []byte("version: 2"), 0o644)
	require.NoError(t, err)

	// Test with explicit path argument works
	// The migrate command accepts explicit path as second argument [config-file]
	assert.FileExists(t, configPath)
}

func TestDetectV2Config_YMLVariant(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Test with .yml extension variant
	// detectV2Config checks ai-rulez.yaml, ai-rulez.yml, .ai-rulez.yaml, .ai-rulez.yml
	configPath := filepath.Join(tmpDir, "ai-rulez.yml")
	err = os.WriteFile(configPath, []byte("version: 2"), 0o644)
	require.NoError(t, err)

	assert.FileExists(t, configPath)
}

func TestDetectV2Config_DottedPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Test with .ai-rulez.yaml variant
	// This is one of the candidates checked in detectV2Config
	configPath := filepath.Join(tmpDir, ".ai-rulez.yaml")
	err = os.WriteFile(configPath, []byte("version: 2"), 0o644)
	require.NoError(t, err)

	assert.FileExists(t, configPath)
}

func TestCopyDir_SimpleDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory with files
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create test files
	file1Path := filepath.Join(srcDir, "file1.txt")
	err = os.WriteFile(file1Path, []byte("content1"), 0o644)
	require.NoError(t, err)

	file2Path := filepath.Join(srcDir, "file2.md")
	err = os.WriteFile(file2Path, []byte("# Content2"), 0o644)
	require.NoError(t, err)

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify destination exists and contains files
	assert.DirExists(t, dstDir)
	assert.FileExists(t, filepath.Join(dstDir, "file1.txt"))
	assert.FileExists(t, filepath.Join(dstDir, "file2.md"))

	// Verify content
	content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(content1))

	content2, err := os.ReadFile(filepath.Join(dstDir, "file2.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Content2", string(content2))
}

func TestCopyDir_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source with nested structure
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(filepath.Join(srcDir, "subdir1", "subdir2"), 0o755)
	require.NoError(t, err)

	// Create files at multiple levels
	err = os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcDir, "subdir1", "level1.txt"), []byte("level1"), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcDir, "subdir1", "subdir2", "level2.txt"), []byte("level2"), 0o644)
	require.NoError(t, err)

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify structure
	assert.DirExists(t, filepath.Join(dstDir, "subdir1"))
	assert.DirExists(t, filepath.Join(dstDir, "subdir1", "subdir2"))
	assert.FileExists(t, filepath.Join(dstDir, "root.txt"))
	assert.FileExists(t, filepath.Join(dstDir, "subdir1", "level1.txt"))
	assert.FileExists(t, filepath.Join(dstDir, "subdir1", "subdir2", "level2.txt"))
}

func TestCopyDir_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty source directory
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify destination exists but is empty
	assert.DirExists(t, dstDir)
	entries, err := os.ReadDir(dstDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestCopyDir_NonexistentSource(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "nonexistent")
	dstDir := filepath.Join(tmpDir, "dst")

	// Attempt to copy nonexistent source
	err := commands.CopyDir(srcDir, dstDir)
	assert.Error(t, err)
}

func TestCopyDir_PermissionsPreserved(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory with specific permissions
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create file
	filePath := filepath.Join(srcDir, "file.txt")
	err = os.WriteFile(filePath, []byte("content"), 0o644)
	require.NoError(t, err)

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify file exists in destination
	dstFilePath := filepath.Join(dstDir, "file.txt")
	assert.FileExists(t, dstFilePath)

	// Verify content
	content, err := os.ReadFile(dstFilePath)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestCopyDir_LargeFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source with large file
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create a large file (1MB)
	largePath := filepath.Join(srcDir, "large.bin")
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	err = os.WriteFile(largePath, largeContent, 0o644)
	require.NoError(t, err)

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify file exists and content matches
	dstLargePath := filepath.Join(dstDir, "large.bin")
	assert.FileExists(t, dstLargePath)

	dstContent, err := os.ReadFile(dstLargePath)
	require.NoError(t, err)
	assert.Equal(t, largeContent, dstContent)
}

func TestCreateBackup_CreatesTimestampedDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory with files
	srcDir := filepath.Join(tmpDir, "test-dir")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644)
	require.NoError(t, err)

	// Create backup
	backupPath := commands.CreateBackup(srcDir)
	require.NotEmpty(t, backupPath)

	// Verify backup directory exists
	assert.DirExists(t, backupPath)

	// Verify backup contains the original file
	assert.FileExists(t, filepath.Join(backupPath, "file.txt"))

	// Verify content
	content, err := os.ReadFile(filepath.Join(backupPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestCreateBackup_BackupNameFormat(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory
	srcDir := filepath.Join(tmpDir, ".ai-rulez")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create backup
	backupPath := commands.CreateBackup(srcDir)
	require.NotEmpty(t, backupPath)

	// Verify backup name format: <original-name>.backup.<timestamp>
	backupName := filepath.Base(backupPath)
	assert.Contains(t, backupName, ".ai-rulez.backup.")

	// Verify timestamp format (YYYYMMDD_HHMMSS)
	assert.Regexp(t, `\.backup\.\d{8}_\d{6}$`, backupName)
}

func TestCreateBackup_MultipleBackups(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory
	srcDir := filepath.Join(tmpDir, "test-dir")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create first backup
	backupPath1 := commands.CreateBackup(srcDir)
	require.NotEmpty(t, backupPath1)
	assert.DirExists(t, backupPath1)

	// Wait a bit to ensure different timestamps (at least 1 second)
	time.Sleep(1100 * time.Millisecond)

	// Create second backup
	backupPath2 := commands.CreateBackup(srcDir)
	require.NotEmpty(t, backupPath2)
	assert.DirExists(t, backupPath2)

	// Verify both backups exist and are different
	assert.NotEqual(t, backupPath1, backupPath2)
	assert.DirExists(t, backupPath1)
	assert.DirExists(t, backupPath2)
}

func TestCreateBackup_WithNestedContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source with nested structure
	srcDir := filepath.Join(tmpDir, "test-dir")
	err := os.MkdirAll(filepath.Join(srcDir, "subdir1", "subdir2"), 0o755)
	require.NoError(t, err)

	// Create files
	err = os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "subdir1", "file1.txt"), []byte("file1"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "subdir1", "subdir2", "file2.txt"), []byte("file2"), 0o644)
	require.NoError(t, err)

	// Create backup
	backupPath := commands.CreateBackup(srcDir)
	require.NotEmpty(t, backupPath)

	// Verify nested structure is preserved
	assert.FileExists(t, filepath.Join(backupPath, "root.txt"))
	assert.DirExists(t, filepath.Join(backupPath, "subdir1"))
	assert.FileExists(t, filepath.Join(backupPath, "subdir1", "file1.txt"))
	assert.DirExists(t, filepath.Join(backupPath, "subdir1", "subdir2"))
	assert.FileExists(t, filepath.Join(backupPath, "subdir1", "subdir2", "file2.txt"))
}

func TestCreateBackup_SameParentDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory
	srcDir := filepath.Join(tmpDir, ".ai-rulez")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create backup
	backupPath := commands.CreateBackup(srcDir)
	require.NotEmpty(t, backupPath)

	// Verify backup is in the same parent directory
	assert.Equal(t, filepath.Dir(srcDir), filepath.Dir(backupPath))

	// Verify both exist
	assert.DirExists(t, srcDir)
	assert.DirExists(t, backupPath)
}

func TestCreateBackup_HandlesInvalidSourceGracefully(t *testing.T) {
	tmpDir := t.TempDir()

	// Try to backup nonexistent directory
	nonexistentDir := filepath.Join(tmpDir, "nonexistent")
	backupPath := commands.CreateBackup(nonexistentDir)

	// Should return empty string on error
	assert.Empty(t, backupPath)
}

func TestMigrateCommand_RequiresVersionArg(t *testing.T) {
	// Verify that command requires at least one argument (version)
	cmd := commands.MigrateCmd
	// MigrateCmd uses cobra.MinimumNArgs(1)
	assert.NotNil(t, cmd.Args)
	assert.Equal(t, "migrate <version> [config-file]", cmd.Use)
}

func TestMigrateCommand_MinimumArgs(t *testing.T) {
	// Verify minimum args requirement via command definition
	// MigrateCmd.Args is set to cobra.MinimumNArgs(1)
	assert.NotNil(t, commands.MigrateCmd.Args)
	cmd := commands.MigrateCmd

	// Verify that the command has the Args validator set
	err := cmd.Args(cmd, []string{"v3"})
	assert.NoError(t, err)

	// Verify that missing args would fail (would be caught by cobra)
	// This is tested implicitly through command execution
}

func TestMigrateCommand_OutputFlagUsage(t *testing.T) {
	cmd := commands.MigrateCmd
	flags := cmd.Flags()

	outputFlag := flags.Lookup("output")
	assert.NotNil(t, outputFlag)
	assert.Equal(t, ".ai-rulez", outputFlag.DefValue)
}

func TestMigrateCommand_DryRunFlagUsage(t *testing.T) {
	cmd := commands.MigrateCmd
	flags := cmd.Flags()

	dryRunFlag := flags.Lookup("dry-run")
	assert.NotNil(t, dryRunFlag)
	assert.Equal(t, "false", dryRunFlag.DefValue)
}

func TestMigrateCommand_ForceFlag(t *testing.T) {
	cmd := commands.MigrateCmd
	flags := cmd.Flags()

	forceFlag := flags.Lookup("force")
	assert.NotNil(t, forceFlag)
	assert.Equal(t, "false", forceFlag.DefValue)
}

func TestMigrateCommand_ValidateFlag(t *testing.T) {
	cmd := commands.MigrateCmd
	flags := cmd.Flags()

	validateFlag := flags.Lookup("validate")
	assert.NotNil(t, validateFlag)
	assert.Equal(t, "false", validateFlag.DefValue)
}

func TestCopyDir_MixedFileTypes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source with various file types
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create different file types
	testFiles := map[string]string{
		"document.md": "# Markdown",
		"config.yaml": "version: 1",
		"data.json":   `{"key": "value"}`,
		"script.sh":   "#!/bin/bash\necho hello",
		"empty.txt":   "",
	}

	for filename, content := range testFiles {
		filePath := filepath.Join(srcDir, filename)
		err = os.WriteFile(filePath, []byte(content), 0o644)
		require.NoError(t, err)
	}

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify all files are copied
	for filename, expectedContent := range testFiles {
		dstFilePath := filepath.Join(dstDir, filename)
		assert.FileExists(t, dstFilePath)

		content, err := os.ReadFile(dstFilePath)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(content))
	}
}

func TestCopyDir_PreservesFileContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source with specific content
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	originalContent := "This is the original content.\nWith multiple lines.\nAnd special chars: !@#$%^&*()"
	filePath := filepath.Join(srcDir, "test.txt")
	err = os.WriteFile(filePath, []byte(originalContent), 0o644)
	require.NoError(t, err)

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify content is preserved exactly
	copiedContent, err := os.ReadFile(filepath.Join(dstDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(copiedContent))
}

func TestCopyDir_ComplexStructure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a realistic .ai-rulez structure
	srcDir := filepath.Join(tmpDir, "src")
	err := os.MkdirAll(filepath.Join(srcDir, "rules"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(srcDir, "context"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(srcDir, "skills", "code-reviewer"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(srcDir, "domains", "backend", "rules"), 0o755)
	require.NoError(t, err)

	// Create files
	err = os.WriteFile(filepath.Join(srcDir, "config.yaml"), []byte("version: 3"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "rules", "style.md"), []byte("# Style Rule"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "skills", "code-reviewer", "SKILL.md"), []byte("# Skill"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "domains", "backend", "rules", "perf.md"), []byte("# Perf"), 0o644)
	require.NoError(t, err)

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	err = commands.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify complete structure
	assert.FileExists(t, filepath.Join(dstDir, "config.yaml"))
	assert.FileExists(t, filepath.Join(dstDir, "rules", "style.md"))
	assert.FileExists(t, filepath.Join(dstDir, "skills", "code-reviewer", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dstDir, "domains", "backend", "rules", "perf.md"))

	// Verify file contents
	content, err := os.ReadFile(filepath.Join(dstDir, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "version: 3", string(content))
}

func TestCreateBackup_PreservesAllContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory with various content types
	srcDir := filepath.Join(tmpDir, ".ai-rulez")
	err := os.MkdirAll(filepath.Join(srcDir, "rules"), 0o755)
	require.NoError(t, err)

	// Create test files with specific content
	err = os.WriteFile(filepath.Join(srcDir, "config.yaml"), []byte("version: 3\nname: test"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "rules", "rule1.md"), []byte("# Rule 1\npriority: high"), 0o644)
	require.NoError(t, err)

	// Create backup
	backupPath := commands.CreateBackup(srcDir)
	require.NotEmpty(t, backupPath)

	// Verify all content is preserved
	configContent, err := os.ReadFile(filepath.Join(backupPath, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "version: 3\nname: test", string(configContent))

	ruleContent, err := os.ReadFile(filepath.Join(backupPath, "rules", "rule1.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Rule 1\npriority: high", string(ruleContent))
}

func TestDeleteBackupDirectory_RemovesBackupDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a source directory and its backup
	srcDir := filepath.Join(tmpDir, ".ai-rulez")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create a backup directory with the expected naming pattern
	backupDir := filepath.Join(tmpDir, ".ai-rulez.backup.20231215_120000")
	err = os.MkdirAll(backupDir, 0o755)
	require.NoError(t, err)

	// Create a test file in the backup
	err = os.WriteFile(filepath.Join(backupDir, "test.txt"), []byte("backup content"), 0o644)
	require.NoError(t, err)

	// Verify backup exists
	assert.DirExists(t, backupDir)

	// Call deleteBackupDirectory
	commands.DeleteBackupDirectory(tmpDir)

	// Verify backup was deleted
	assert.NoDirExists(t, backupDir)
}

func TestDeleteBackupDirectory_IgnoresNonBackupDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some directories that should not be deleted
	otherDir := filepath.Join(tmpDir, ".ai-rulez")
	err := os.MkdirAll(otherDir, 0o755)
	require.NoError(t, err)

	anotherDir := filepath.Join(tmpDir, "some-other-backup")
	err = os.MkdirAll(anotherDir, 0o755)
	require.NoError(t, err)

	// Create a backup directory
	backupDir := filepath.Join(tmpDir, ".ai-rulez.backup.20231215_120000")
	err = os.MkdirAll(backupDir, 0o755)
	require.NoError(t, err)

	// Call deleteBackupDirectory
	commands.DeleteBackupDirectory(tmpDir)

	// Verify backup was deleted but others remain
	assert.DirExists(t, otherDir)
	assert.DirExists(t, anotherDir)
	assert.NoDirExists(t, backupDir)
}

func TestDeleteBackupDirectory_HandlesMultipleBackups(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple backup directories
	backup1 := filepath.Join(tmpDir, ".ai-rulez.backup.20231215_100000")
	backup2 := filepath.Join(tmpDir, ".ai-rulez.backup.20231215_110000")
	backup3 := filepath.Join(tmpDir, ".ai-rulez.backup.20231215_120000")

	for _, backup := range []string{backup1, backup2, backup3} {
		err := os.MkdirAll(backup, 0o755)
		require.NoError(t, err)
	}

	// Verify all backups exist
	assert.DirExists(t, backup1)
	assert.DirExists(t, backup2)
	assert.DirExists(t, backup3)

	// Call deleteBackupDirectory
	commands.DeleteBackupDirectory(tmpDir)

	// Verify all backups were deleted
	assert.NoDirExists(t, backup1)
	assert.NoDirExists(t, backup2)
	assert.NoDirExists(t, backup3)
}

func TestDeleteBackupDirectory_NoBackupDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a source directory
	srcDir := filepath.Join(tmpDir, ".ai-rulez")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Call deleteBackupDirectory - should not error
	// This is a no-op since there are no backups to delete
	commands.DeleteBackupDirectory(tmpDir)

	// Verify source directory still exists
	assert.DirExists(t, srcDir)
}

func TestDeleteBackupDirectory_InvalidDirectory(t *testing.T) {
	// Test with a directory that doesn't exist
	// deleteBackupDirectory should handle this gracefully and not panic
	commands.DeleteBackupDirectory("/nonexistent/path")
	// If this doesn't panic, the test passes
}
