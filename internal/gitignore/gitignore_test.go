package gitignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestUpdateGitignoreFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gitignore_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	cfg := &config.Config{
		Outputs: []config.Output{
			{Path: "CLAUDE.md"},
			{Path: ".cursor/rules/", Type: "rule", NamingScheme: "rules.mdc"},
			{Path: ".windsurfrules"},
		},
	}

	err = UpdateGitignoreFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("UpdateGitignoreFiles failed: %v", err)
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("Failed to read .gitignore: %v", err)
	}

	contentStr := string(content)
	expectedFiles := []string{"CLAUDE.md", ".cursor/", ".windsurfrules"}
	for _, file := range expectedFiles {
		if !strings.Contains(contentStr, file) {
			t.Errorf("Expected .gitignore to contain %s, but it doesn't", file)
		}
	}

	existingContent := "node_modules/\n.cursor/\n"
	err = os.WriteFile(gitignorePath, []byte(existingContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write existing .gitignore: %v", err)
	}

	err = UpdateGitignoreFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("UpdateGitignoreFiles failed on second run: %v", err)
	}

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
	count := strings.Count(contentStr, ".cursor/")
	if count != 1 {
		t.Errorf("Expected .cursor/ to appear once, but found %d occurrences", count)
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		filename string
		pattern  string
		expected bool
	}{
		{"CLAUDE.md", "CLAUDE.md", true},
		{"test.txt", "test.txt", true},
		{"test.txt", "other.txt", false},

		{"test.md", "*.md", true},
		{"README.md", "*.md", true},
		{"test.txt", "*.md", false},
		{"prefix_test", "prefix*", true},
		{"test_suffix", "*suffix", true},

		{"test.md", "docs/", false},

		{"CLAUDE.md", "/CLAUDE.md", true},
		{"subdir/CLAUDE.md", "/CLAUDE.md", false},

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
		{"error.log", true},
		{"CLAUDE.md", true},
		{"README.md", false},
		{"dist/bundle.js", true},
		{"build", true},
		{"src/build", false},
	}

	for _, test := range tests {
		result := isIgnored(test.filename, patterns)
		if result != test.expected {
			t.Errorf("isIgnored(%q) = %v, expected %v", test.filename, result, test.expected)
		}
	}
}

func TestReadGitignoreEntries(t *testing.T) {
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
	err = os.WriteFile(gitignorePath, []byte(content), 0o644)
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
	tmpDir, err := os.MkdirTemp("", "gitignore_no_outputs_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	cfg := &config.Config{
		Outputs: []config.Output{},
	}

	err = UpdateGitignoreFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("UpdateGitignoreFiles failed: %v", err)
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		t.Error("Expected .gitignore not to be created when there are no outputs")
	}
}
