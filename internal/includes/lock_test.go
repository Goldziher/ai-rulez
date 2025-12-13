package includes

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadLockFile_NotExists(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()

	// Act
	lock, err := LoadLockFile(tmpDir)

	// Assert
	if err != nil {
		t.Errorf("LoadLockFile() should not error when lock file doesn't exist, got %v", err)
	}
	if lock == nil {
		t.Fatalf("LoadLockFile() should return empty lock when file doesn't exist")
	}
	if len(lock.Includes) != 0 {
		t.Error("LoadLockFile() should return lock with empty includes map")
	}
}

func TestLoadLockFile_ExistsValid(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, ".ai-rulez")
	lockPath := filepath.Join(lockDir, lockFileName)

	// Create directory and lock file
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("Failed to create lock directory: %v", err)
	}

	lockContent := []byte(`includes:
  test-include:
    source: "https://github.com/org/repo"
    type: "git"
    resolved_ref: "abc123def"
    resolved_at: 2025-12-27T08:00:00Z
`)

	if err := os.WriteFile(lockPath, lockContent, 0o644); err != nil {
		t.Fatalf("Failed to write lock file: %v", err)
	}

	// Act
	lock, err := LoadLockFile(tmpDir)

	// Assert
	if err != nil {
		t.Errorf("LoadLockFile() failed: %v", err)
	}
	if lock == nil {
		t.Fatalf("LoadLockFile() returned nil")
	}
	if len(lock.Includes) != 1 {
		t.Errorf("LoadLockFile() expected 1 include, got %d", len(lock.Includes))
	}

	entry, exists := lock.Includes["test-include"]
	if !exists {
		t.Error("LoadLockFile() missing expected include 'test-include'")
	}
	if entry.Source != "https://github.com/org/repo" {
		t.Errorf("Expected source 'https://github.com/org/repo', got '%s'", entry.Source)
	}
	if entry.Type != "git" {
		t.Errorf("Expected type 'git', got '%s'", entry.Type)
	}
	if entry.ResolvedRef != "abc123def" {
		t.Errorf("Expected resolved_ref 'abc123def', got '%s'", entry.ResolvedRef)
	}
}

func TestSaveLockFile_CreatesDirectory(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	lock := &Lock{
		Includes: map[string]LockEntry{
			"test-include": {
				Source:      "https://github.com/org/repo",
				Type:        "git",
				ResolvedRef: "abc123",
				ResolvedAt:  time.Now(),
			},
		},
	}

	// Act
	err := SaveLockFile(tmpDir, lock)

	// Assert
	if err != nil {
		t.Errorf("SaveLockFile() failed: %v", err)
	}

	// Verify lock file exists
	lockPath := filepath.Join(tmpDir, ".ai-rulez", lockFileName)
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("SaveLockFile() did not create lock file")
	}
}

func TestSaveLockFile_WritesCorrectContent(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	lock := &Lock{
		Includes: map[string]LockEntry{
			"test-include": {
				Source:      "https://github.com/org/repo",
				Type:        "git",
				ResolvedRef: "abc123",
				ResolvedAt:  time.Now().Round(time.Second),
			},
		},
	}

	// Act
	err := SaveLockFile(tmpDir, lock)
	if err != nil {
		t.Fatalf("SaveLockFile() failed: %v", err)
	}

	// Verify content can be read back
	loadedLock, err := LoadLockFile(tmpDir)
	if err != nil {
		t.Errorf("Failed to reload lock file: %v", err)
	}

	entry, exists := loadedLock.Includes["test-include"]
	if !exists {
		t.Error("Reloaded lock missing expected include")
	}
	if entry.Source != "https://github.com/org/repo" {
		t.Errorf("Reloaded source mismatch: expected 'https://github.com/org/repo', got '%s'", entry.Source)
	}
	if entry.Type != "git" {
		t.Errorf("Reloaded type mismatch: expected 'git', got '%s'", entry.Type)
	}
}

func TestUpdateLock(t *testing.T) {
	// Arrange
	lock := &Lock{
		Includes: make(map[string]LockEntry),
	}
	entry := LockEntry{
		Source:      "https://github.com/org/repo",
		Type:        "git",
		ResolvedRef: "abc123",
		ResolvedAt:  time.Now(),
	}

	// Act
	UpdateLock(lock, "test-include", entry)

	// Assert
	if len(lock.Includes) != 1 {
		t.Errorf("UpdateLock() expected 1 include, got %d", len(lock.Includes))
	}
	savedEntry, exists := lock.Includes["test-include"]
	if !exists {
		t.Error("UpdateLock() did not save include")
	}
	if savedEntry.Source != entry.Source {
		t.Errorf("UpdateLock() source mismatch")
	}
}

func TestUpdateLock_NilIncludes(t *testing.T) {
	// Arrange
	lock := &Lock{}
	entry := LockEntry{
		Source:      "https://github.com/org/repo",
		Type:        "git",
		ResolvedRef: "abc123",
		ResolvedAt:  time.Now(),
	}

	// Act
	UpdateLock(lock, "test-include", entry)

	// Assert
	if lock.Includes == nil {
		t.Error("UpdateLock() should initialize nil Includes map")
	}
	if len(lock.Includes) != 1 {
		t.Errorf("UpdateLock() expected 1 include, got %d", len(lock.Includes))
	}
}

func TestCreateLockEntry(t *testing.T) {
	// Arrange
	sourceType := "git"
	source := "https://github.com/org/repo"
	resolvedRef := "abc123def"

	// Act
	entry := CreateLockEntry(sourceType, source, resolvedRef)

	// Assert
	if entry.Source != source {
		t.Errorf("CreateLockEntry() source mismatch: expected '%s', got '%s'", source, entry.Source)
	}
	if entry.Type != sourceType {
		t.Errorf("CreateLockEntry() type mismatch: expected '%s', got '%s'", sourceType, entry.Type)
	}
	if entry.ResolvedRef != resolvedRef {
		t.Errorf("CreateLockEntry() resolved_ref mismatch: expected '%s', got '%s'", resolvedRef, entry.ResolvedRef)
	}
	if entry.ResolvedAt.IsZero() {
		t.Error("CreateLockEntry() should set resolved_at timestamp")
	}
}

func TestCreateLockEntry_WithLocalPath(t *testing.T) {
	// Arrange
	sourceType := "local"
	source := "/path/to/local/content"
	resolvedRef := ""

	// Act
	entry := CreateLockEntry(sourceType, source, resolvedRef)

	// Assert
	if entry.Source != source {
		t.Errorf("CreateLockEntry() source mismatch")
	}
	if entry.Type != sourceType {
		t.Errorf("CreateLockEntry() type mismatch")
	}
	if entry.ResolvedRef != "" {
		t.Errorf("CreateLockEntry() should allow empty resolved_ref for local paths")
	}
}
