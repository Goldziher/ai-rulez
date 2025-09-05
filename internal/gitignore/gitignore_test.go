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
			{Path: ".claude/agents/", Type: "agent", NamingScheme: "{name}.md"},
			{Path: ".continue/prompts/ai_rulez_prompts.yaml"},
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
	expectedFiles := []string{
		"CLAUDE.md",
		".cursor/rules/",
		".windsurfrules",
		".claude/agents/",
		".continue/prompts/ai_rulez_prompts.yaml",
	}
	for _, file := range expectedFiles {
		if !strings.Contains(contentStr, file) {
			t.Errorf("Expected .gitignore to contain %s, but it doesn't", file)
		}
	}

	existingContent := "node_modules/\n.cursor/rules/\n"
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
	count := strings.Count(contentStr, ".cursor/rules/")
	if count != 1 {
		t.Errorf("Expected .cursor/rules/ to appear once, but found %d occurrences", count)
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

func TestConfigGitignoreFlag(t *testing.T) {
	tests := []struct {
		name           string
		gitignoreFlag  *bool
		expectedResult bool
	}{
		{
			name:           "default value (nil)",
			gitignoreFlag:  nil,
			expectedResult: true,
		},
		{
			name:           "explicitly true",
			gitignoreFlag:  boolPtr(true),
			expectedResult: true,
		},
		{
			name:           "explicitly false",
			gitignoreFlag:  boolPtr(false),
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Gitignore: tt.gitignoreFlag,
			}

			result := cfg.ShouldUpdateGitignore()
			if result != tt.expectedResult {
				t.Errorf("ShouldUpdateGitignore() = %v, expected %v", result, tt.expectedResult)
			}
		})
	}
}

func TestGitignoreWithPresets(t *testing.T) {
	tests := []struct {
		name             string
		presetOutputs    []config.Output
		expectedPatterns []string
	}{
		{
			name: "claude preset",
			presetOutputs: []config.Output{
				{Path: "CLAUDE.md"},
				{Path: ".claude/agents/"},
			},
			expectedPatterns: []string{"CLAUDE.md", ".claude/agents/"},
		},
		{
			name: "cursor preset",
			presetOutputs: []config.Output{
				{Path: ".cursor/rules/"},
			},
			expectedPatterns: []string{".cursor/rules/"},
		},
		{
			name: "continue preset",
			presetOutputs: []config.Output{
				{Path: ".continue/rules/"},
				{Path: ".continue/prompts/ai_rulez_prompts.yaml"},
			},
			expectedPatterns: []string{".continue/rules/", ".continue/prompts/ai_rulez_prompts.yaml"},
		},
		{
			name: "popular preset",
			presetOutputs: []config.Output{
				{Path: "CLAUDE.md"},
				{Path: ".cursor/rules/"},
				{Path: ".windsurf/"},
				{Path: ".github/copilot-instructions.md"},
			},
			expectedPatterns: []string{
				"CLAUDE.md",
				".cursor/rules/",
				".windsurf/",
				".github/copilot-instructions.md",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "gitignore_preset_test")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
			cfg := &config.Config{
				Outputs: tt.presetOutputs,
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
			for _, pattern := range tt.expectedPatterns {
				if !strings.Contains(contentStr, pattern) {
					t.Errorf("Expected .gitignore to contain %q, but it doesn't", pattern)
				}
			}
		})
	}
}

func TestGitignoreWithMixedOutputs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gitignore_mixed_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	cfg := &config.Config{
		Outputs: []config.Output{
			{Path: "CLAUDE.md"},
			{Path: ".claude/agents/"},
			{Path: "CUSTOM.md"},
			{Path: ".my-custom-dir/"},
			{Path: "docs/AI_RULES.md"},
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
	expectedPatterns := []string{
		"CLAUDE.md",
		".claude/agents/",
		"CUSTOM.md",
		".my-custom-dir/",
		"docs/AI_RULES.md",
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(contentStr, pattern) {
			t.Errorf("Expected .gitignore to contain %q, but it doesn't", pattern)
		}
	}

	lines := strings.Split(contentStr, "\n")
	seen := make(map[string]int)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			seen[line]++
			if seen[line] > 1 {
				t.Errorf("Pattern %q appears %d times in .gitignore (should appear only once)", line, seen[line])
			}
		}
	}
}

func TestGitignoreDeduplication(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gitignore_dedup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	cfg := &config.Config{
		Outputs: []config.Output{
			{Path: "CLAUDE.md"},
			{Path: "CLAUDE.md"},
			{Path: ".claude/agents/"},
			{Path: ".claude/agents/"},
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

	claudeMdCount := strings.Count(contentStr, "CLAUDE.md")
	agentsCount := strings.Count(contentStr, ".claude/agents/")

	if claudeMdCount != 1 {
		t.Errorf("Expected CLAUDE.md to appear once, but it appears %d times", claudeMdCount)
	}
	if agentsCount != 1 {
		t.Errorf("Expected .claude/agents/ to appear once, but it appears %d times", agentsCount)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
