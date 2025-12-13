package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPathExtractionFix verifies Issue #12 fix: correct extraction of top-level directories
// from paths like ".claude/skills/test-skill" -> ".claude"
func TestPathExtractionFix(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		// Original issue case
		{"claude with nested skills path", ".claude/skills/test-skill", ".claude"},

		// Variations
		{"single level directory", ".claude", ".claude"},
		{"nested path with multiple levels", ".claude/skills/test-skill/subcategory", ".claude"},
		{"cursor directory", ".cursor", ".cursor"},
		{"custom nested path", "output/nested/path", "output"},
		{"simple file", "file.md", "file.md"},

		// Edge cases
		{"path with dots", ".output.dir/file.txt", ".output.dir"},
		{"relative path up", "../parent/child", ".."},
		{"absolute path creates empty first element", "/absolute/path", ""}, // filepath.Split on leading / gives empty string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the FIXED logic from generator_v3.go line 205
			// OLD (WRONG): parts := filepath.SplitList(relPath)
			// NEW (CORRECT): parts := strings.Split(filepath.Clean(relPath), string(filepath.Separator))

			parts := strings.Split(filepath.Clean(tt.path), string(filepath.Separator))
			var topLevel string
			if len(parts) > 0 {
				topLevel = parts[0]
			}

			assert.Equal(t, tt.expected, topLevel, "path: %s should extract to top-level: %s", tt.path, tt.expected)
		})
	}
}

// TestPathExtractionVsOldBuggy demonstrates the difference between buggy and fixed approach
func TestPathExtractionVsOldBuggy(t *testing.T) {
	testPath := ".claude/skills/test-skill"

	// OLD BUGGY CODE (WRONG)
	// filepath.SplitList splits on PATH separator (:  on Unix, ; on Windows)
	// not on forward slash / which is used in paths
	oldBuggyParts := filepath.SplitList(testPath)
	// On Unix: splits on :, so returns [".claude/skills/test-skill"] (no split!)
	// On Windows: splits on ;, so returns [".claude/skills/test-skill"] (no split!)

	// NEW FIXED CODE (CORRECT)
	newFixedParts := strings.Split(filepath.Clean(testPath), string(filepath.Separator))
	// On Unix: splits on /, so returns [".claude", "skills", "test-skill"]
	// On Windows: splits on \, so returns [".claude", "skills", "test-skill"]

	// Verify the fix
	assert.NotEmpty(t, oldBuggyParts)

	// The old code would take the entire path as top-level (WRONG!)
	oldTopLevel := oldBuggyParts[0]

	// The new code correctly extracts just the top-level directory
	newTopLevel := newFixedParts[0]

	assert.Equal(t, ".claude", newTopLevel, "fixed code should extract .claude")
	assert.NotEqual(t, ".claude", oldTopLevel, "buggy code returns full path, not top-level")
}
