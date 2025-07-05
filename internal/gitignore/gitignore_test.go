package gitignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestUpdateGitignoreFiles(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "gitignore_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a test config file
	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	cfg := &config.Config{
		Outputs: []config.Output{
			{File: "CLAUDE.md"},
			{File: ".cursorrules"},
			{File: ".windsurfrules"},
		},
	}

	// Test case 1: No existing .gitignore file
	err = UpdateGitignoreFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("UpdateGitignoreFiles failed: %v", err)
	}

	// Check that .gitignore was created with correct content
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("Failed to read .gitignore: %v", err)
	}

	contentStr := string(content)
	expectedFiles := []string{"CLAUDE.md", ".cursorrules", ".windsurfrules"}
	for _, file := range expectedFiles {
		if !strings.Contains(contentStr, file) {
			t.Errorf("Expected .gitignore to contain %s, but it doesn't", file)
		}
	}

	// Test case 2: Existing .gitignore with some files already ignored
	existingContent := "node_modules/\n.cursorrules\n"
	err = os.WriteFile(gitignorePath, []byte(existingContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write existing .gitignore: %v", err)
	}

	err = UpdateGitignoreFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("UpdateGitignoreFiles failed on second run: %v", err)
	}

	// Check that only new files were added
	content, err = os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("Failed to read .gitignore after second update: %v", err)
	}

	contentStr = string(content)
	if !strings.Contains(contentStr, "CLAUDE.md") {
		t.Error("Expected .gitignore to contain CLAUDE.md")
	}
	if !strings.Contains(contentStr, ".windsurfrules") {
		t.Error("Expected .gitignore to contain .windsurfrules")
	}
	// .cursorrules should appear only once (from the original content)
	count := strings.Count(contentStr, ".cursorrules")
	if count != 1 {
		t.Errorf("Expected .cursorrules to appear once, but found %d occurrences", count)
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		filename string
		pattern  string
		expected bool
	}{
		// Exact matches
		{"CLAUDE.md", "CLAUDE.md", true},
		{"test.txt", "test.txt", true},
		{"test.txt", "other.txt", false},

		// Wildcard patterns
		{"test.md", "*.md", true},
		{"README.md", "*.md", true},
		{"test.txt", "*.md", false},
		{"prefix_test", "prefix*", true},
		{"test_suffix", "*suffix", true},

		// Directory patterns (should not match files)
		{"test.md", "docs/", false},

		// Absolute path patterns
		{"CLAUDE.md", "/CLAUDE.md", true},
		{"subdir/CLAUDE.md", "/CLAUDE.md", false},

		// Substring matching
		{"generated_file.md", "generated", true},
		{"my_file.txt", "generated", false},
	}

	for _, test := range tests {
		result := matchesPattern(test.filename, test.pattern)
		if result != test.expected {
			t.Errorf("matchesPattern(%q, %q) = %v, expected %v", 
				test.filename, test.pattern, result, test.expected)
		}
	}
}

func TestIsIgnored(t *testing.T) {
	patterns := []string{
		"*.log",
		"node_modules/",
		"CLAUDE.md",
		"dist/*",
		"/build",
	}

	tests := []struct {
		filename string
		expected bool
	}{
		{"error.log", true},    // matches *.log
		{"CLAUDE.md", true},    // exact match
		{"README.md", false},   // no match
		{"dist/bundle.js", true}, // matches dist/*
		{"build", true},        // matches /build
		{"src/build", false},   // doesn't match /build (absolute)
	}

	for _, test := range tests {
		result := isIgnored(test.filename, patterns)
		if result != test.expected {
			t.Errorf("isIgnored(%q) = %v, expected %v", test.filename, result, test.expected)
		}
	}
}

func TestReadGitignoreEntries(t *testing.T) {
	// Create a temporary file
	tmpDir, err := os.MkdirTemp("", "gitignore_read_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content := `# This is a comment
node_modules/
*.log

# Another comment
dist/
.env
`
	err = os.WriteFile(gitignorePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write .gitignore: %v", err)
	}

	entries, err := readGitignoreEntries(gitignorePath)
	if err != nil {
		t.Fatalf("readGitignoreEntries failed: %v", err)
	}

	expected := []string{"node_modules/", "*.log", "dist/", ".env"}
	if len(entries) != len(expected) {
		t.Fatalf("Expected %d entries, got %d", len(expected), len(entries))
	}

	for i, entry := range entries {
		if entry != expected[i] {
			t.Errorf("Expected entry %d to be %q, got %q", i, expected[i], entry)
		}
	}
}

func TestUpdateGitignoreFilesWithNoOutputs(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "gitignore_no_outputs_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a test config file with no outputs
	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	cfg := &config.Config{
		Outputs: []config.Output{}, // No outputs
	}

	// Should not create .gitignore if no outputs
	err = UpdateGitignoreFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("UpdateGitignoreFiles failed: %v", err)
	}

	// Check that .gitignore was not created
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		t.Error("Expected .gitignore not to be created when there are no outputs")
	}
}