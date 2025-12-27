package crud_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileManagerPathExists tests path existence checking
func TestFileManagerPathExists(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	// Create a test file and directory
	testFile := filepath.Join(tmpDir, "test.txt")
	testDir := filepath.Join(tmpDir, "testdir")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))
	require.NoError(t, os.Mkdir(testDir, 0o755))

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"existing file", testFile, true},
		{"existing directory", testDir, true},
		{"non-existent path", filepath.Join(tmpDir, "nonexistent"), false},
		{"non-existent nested path", filepath.Join(tmpDir, "a", "b", "c"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fm.PathExists(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFileManagerIsDirectory tests directory checking
func TestFileManagerIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	// Create a test file and directory
	testFile := filepath.Join(tmpDir, "test.txt")
	testDir := filepath.Join(tmpDir, "testdir")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))
	require.NoError(t, os.Mkdir(testDir, 0o755))

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"is directory", testDir, true},
		{"is file", testFile, false},
		{"non-existent path", filepath.Join(tmpDir, "nonexistent"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fm.IsDirectory(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFileManagerCreateDirectory tests directory creation
func TestFileManagerCreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	tests := []struct {
		name      string
		path      string
		shouldErr bool
	}{
		{"create single directory", filepath.Join(tmpDir, "new"), false},
		{"create nested directories", filepath.Join(tmpDir, "a", "b", "c"), false},
		{"create in existing directory", filepath.Join(tmpDir, "new"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fm.CreateDirectory(tt.path)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.DirExists(t, tt.path)
			}
		})
	}
}

// TestFileManagerDeleteDirectory tests directory deletion
func TestFileManagerDeleteDirectory(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns path to delete
		shouldErr bool
	}{
		{
			name: "delete empty directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "to-delete")
				require.NoError(t, os.Mkdir(subDir, 0o755))
				return subDir
			},
			shouldErr: false,
		},
		{
			name: "delete directory with files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "to-delete")
				require.NoError(t, os.Mkdir(subDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("test"), 0o644))
				return subDir
			},
			shouldErr: false,
		},
		{
			name: "delete directory with nested structure",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "to-delete")
				require.NoError(t, os.MkdirAll(filepath.Join(subDir, "a", "b"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(subDir, "a", "b", "file.txt"), []byte("test"), 0o644))
				return subDir
			},
			shouldErr: false,
		},
		{
			name: "delete non-existent directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := crud.NewFileManager(t.TempDir())
			pathToDelete := tt.setup(t)

			err := fm.DeleteDirectory(pathToDelete)

			if tt.shouldErr {
				assert.Error(t, err)
				// Directory should still exist if error
				// (unless it's a non-existent path)
			} else {
				assert.NoError(t, err)
				assert.NoDirExists(t, pathToDelete)
			}
		})
	}
}

// TestFileManagerWriteFile tests atomic file writing
func TestFileManagerWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	tests := []struct {
		name      string
		filePath  string
		content   string
		shouldErr bool
	}{
		{
			name:      "write file to existing directory",
			filePath:  filepath.Join(tmpDir, "test.txt"),
			content:   "Hello, World!",
			shouldErr: false,
		},
		{
			name:      "write file with nested directory creation",
			filePath:  filepath.Join(tmpDir, "nested", "dir", "test.txt"),
			content:   "Nested content",
			shouldErr: false,
		},
		{
			name:      "write empty file",
			filePath:  filepath.Join(tmpDir, "empty.txt"),
			content:   "",
			shouldErr: false,
		},
		{
			name:      "write large file",
			filePath:  filepath.Join(tmpDir, "large.txt"),
			content:   string(make([]byte, 10000)),
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fm.WriteFile(tt.filePath, tt.content)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.FileExists(t, tt.filePath)

				// Verify content
				data, err := os.ReadFile(tt.filePath)
				require.NoError(t, err)
				assert.Equal(t, tt.content, string(data))
			}
		})
	}
}

// TestFileManagerWriteFileAlreadyExists tests preventing overwrites
func TestFileManagerWriteFileAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	filePath := filepath.Join(tmpDir, "existing.txt")
	originalContent := "original"

	// Write initial file
	require.NoError(t, os.WriteFile(filePath, []byte(originalContent), 0o644))

	// Try to write again
	err := fm.WriteFile(filePath, "new content")

	assert.Error(t, err)

	// Verify original content unchanged
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(data))
}

// TestFileManagerDeleteFile tests file deletion
func TestFileManagerDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns file path
		shouldErr bool
	}{
		{
			name: "delete existing file",
			setup: func(t *testing.T) string {
				filePath := filepath.Join(tmpDir, "to-delete.txt")
				require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))
				return filePath
			},
			shouldErr: false,
		},
		{
			name: "delete non-existent file",
			setup: func(t *testing.T) string {
				return filepath.Join(tmpDir, "nonexistent.txt")
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setup(t)

			err := fm.DeleteFile(filePath)

			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NoFileExists(t, filePath)
			}
		})
	}
}

// TestFileManagerCreateDomainStructure tests domain directory structure creation
func TestFileManagerCreateDomainStructure(t *testing.T) {
	tmpDir := t.TempDir()
	aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")
	require.NoError(t, os.Mkdir(aiRulezDir, 0o755))

	fm := crud.NewFileManager(aiRulezDir)

	err := fm.CreateDomainStructure("test-domain")
	require.NoError(t, err)

	// Verify structure
	domainPath := filepath.Join(aiRulezDir, "domains", "test-domain")
	assert.DirExists(t, domainPath)
	assert.DirExists(t, filepath.Join(domainPath, "rules"))
	assert.DirExists(t, filepath.Join(domainPath, "context"))
	assert.DirExists(t, filepath.Join(domainPath, "skills"))
}

// TestFileManagerDomainExists tests domain existence checking
func TestFileManagerDomainExists(t *testing.T) {
	tmpDir := t.TempDir()
	aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "domains"), 0o755))

	fm := crud.NewFileManager(aiRulezDir)

	// Create a domain
	require.NoError(t, fm.CreateDomainStructure("existing"))

	tests := []struct {
		name     string
		domain   string
		expected bool
	}{
		{"existing domain", "existing", true},
		{"non-existent domain", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fm.DomainExists(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFileManagerListSubdirectories tests listing subdirectories
func TestFileManagerListSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	// Create test structure
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "dir1"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "dir2"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "dir3"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0o644))

	subdirs, err := fm.ListSubdirectories(tmpDir)

	assert.NoError(t, err)
	assert.Len(t, subdirs, 3)
	assert.Contains(t, subdirs, "dir1")
	assert.Contains(t, subdirs, "dir2")
	assert.Contains(t, subdirs, "dir3")
}

// TestFileManagerListMarkdownFiles tests listing markdown files
func TestFileManagerListMarkdownFiles(t *testing.T) {
	tmpDir := t.TempDir()
	fm := crud.NewFileManager(tmpDir)

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.md"), []byte("test"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.md"), []byte("test"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755))

	files, err := fm.ListMarkdownFiles(tmpDir)

	assert.NoError(t, err)
	assert.Len(t, files, 2)
	assert.Contains(t, files, "file1.md")
	assert.Contains(t, files, "file2.md")
}
