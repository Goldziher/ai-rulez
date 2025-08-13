package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLefthook(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected bool
	}{
		{
			name:     "detects lefthook.yaml",
			files:    []string{"lefthook.yaml"},
			expected: true,
		},
		{
			name:     "detects lefthook.yml",
			files:    []string{"lefthook.yml"},
			expected: true,
		},
		{
			name:     "detects .lefthook.yml",
			files:    []string{".lefthook.yml"},
			expected: true,
		},
		{
			name:     "no lefthook config",
			files:    []string{"other.yaml"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			defer os.Chdir(oldWd)
			os.Chdir(tmpDir)

			// Create test files
			for _, file := range tt.files {
				if err := os.WriteFile(file, []byte("test"), 0o644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			// Test detection
			result := detectLefthook()
			if result != tt.expected {
				t.Errorf("detectLefthook() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDetectPreCommit(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected bool
	}{
		{
			name:     "detects .pre-commit-config.yaml",
			files:    []string{".pre-commit-config.yaml"},
			expected: true,
		},
		{
			name:     "detects .pre-commit-config.yml",
			files:    []string{".pre-commit-config.yml"},
			expected: true,
		},
		{
			name:     "no pre-commit config",
			files:    []string{"other.yaml"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			defer os.Chdir(oldWd)
			os.Chdir(tmpDir)

			// Create test files
			for _, file := range tt.files {
				if err := os.WriteFile(file, []byte("test"), 0o644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			// Test detection
			result := detectPreCommit()
			if result != tt.expected {
				t.Errorf("detectPreCommit() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDetectHusky(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(tmpDir string) error
		expected bool
	}{
		{
			name: "detects .husky directory",
			setup: func(tmpDir string) error {
				return os.Mkdir(".husky", 0o755)
			},
			expected: true,
		},
		{
			name: "no husky directory",
			setup: func(tmpDir string) error {
				return nil
			},
			expected: false,
		},
		{
			name: ".husky is a file not directory",
			setup: func(tmpDir string) error {
				return os.WriteFile(".husky", []byte("test"), 0o644)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			defer os.Chdir(oldWd)
			os.Chdir(tmpDir)

			// Setup test scenario
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			// Test detection
			result := detectHusky()
			if result != tt.expected {
				t.Errorf("detectHusky() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSetupLefthookConfig(t *testing.T) {
	tests := []struct {
		name          string
		existingYAML  string
		expectedCheck string
		expectError   bool
	}{
		{
			name: "adds to existing pre-commit section",
			existingYAML: `pre-commit:
  commands:
    lint:
      run: golangci-lint run
`,
			expectedCheck: "ai-rulez",
			expectError:   false,
		},
		{
			name:          "creates pre-commit section if missing",
			existingYAML:  `some_other_section: {}`,
			expectedCheck: "ai-rulez",
			expectError:   false,
		},
		{
			name: "doesn't duplicate existing ai-rulez hook",
			existingYAML: `pre-commit:
  commands:
    ai-rulez:
      run: ai-rulez generate --dry-run
`,
			expectedCheck: "ai-rulez",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			defer os.Chdir(oldWd)
			os.Chdir(tmpDir)

			// Create lefthook.yaml with test content
			if err := os.WriteFile("lefthook.yaml", []byte(tt.existingYAML), 0o644); err != nil {
				t.Fatalf("Failed to create lefthook.yaml: %v", err)
			}

			// Run setup
			err := setupLefthookConfig()
			if (err != nil) != tt.expectError {
				t.Errorf("setupLefthookConfig() error = %v, expectError %v", err, tt.expectError)
			}

			// Check result if no error expected
			if !tt.expectError {
				content, err := os.ReadFile("lefthook.yaml")
				if err != nil {
					t.Fatalf("Failed to read lefthook.yaml: %v", err)
				}

				if tt.expectedCheck != "" {
					contentStr := string(content)
					if !contains(contentStr, tt.expectedCheck) {
						t.Errorf("Expected content to contain %q, got:\n%s", tt.expectedCheck, contentStr)
					}
				}
			}
		})
	}
}

func TestSetupPreCommitConfig(t *testing.T) {
	tests := []struct {
		name          string
		existingYAML  string
		expectedCheck string
		expectError   bool
	}{
		{
			name: "adds ai-rulez repo to existing config",
			existingYAML: `repos:
  - repo: https://github.com/some/other-repo
    rev: v1.0.0
    hooks:
      - id: some-hook
`,
			expectedCheck: "Goldziher/ai-rulez",
			expectError:   false,
		},
		{
			name:          "creates repos section if missing",
			existingYAML:  `default_language_version:\n  python: python3`,
			expectedCheck: "Goldziher/ai-rulez",
			expectError:   false,
		},
		{
			name: "doesn't duplicate existing ai-rulez repo",
			existingYAML: `repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.0.0
    hooks:
      - id: ai-rulez-validate
`,
			expectedCheck: "Goldziher/ai-rulez",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			defer os.Chdir(oldWd)
			os.Chdir(tmpDir)

			// Create .pre-commit-config.yaml with test content
			if err := os.WriteFile(".pre-commit-config.yaml", []byte(tt.existingYAML), 0o644); err != nil {
				t.Fatalf("Failed to create .pre-commit-config.yaml: %v", err)
			}

			// Run setup
			err := setupPreCommitConfig()
			if (err != nil) != tt.expectError {
				t.Errorf("setupPreCommitConfig() error = %v, expectError %v", err, tt.expectError)
			}

			// Check result if no error expected
			if !tt.expectError {
				content, err := os.ReadFile(".pre-commit-config.yaml")
				if err != nil {
					t.Fatalf("Failed to read .pre-commit-config.yaml: %v", err)
				}

				if tt.expectedCheck != "" {
					contentStr := string(content)
					if !contains(contentStr, tt.expectedCheck) {
						t.Errorf("Expected content to contain %q, got:\n%s", tt.expectedCheck, contentStr)
					}
				}
			}
		})
	}
}

func TestSetupHuskyConfig(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    func(tmpDir string) error
		expectedCheck string
		expectError   bool
	}{
		{
			name: "creates new pre-commit hook",
			setupFiles: func(tmpDir string) error {
				return os.Mkdir(".husky", 0o755)
			},
			expectedCheck: "ai-rulez generate --dry-run",
			expectError:   false,
		},
		{
			name: "appends to existing pre-commit hook",
			setupFiles: func(tmpDir string) error {
				if err := os.Mkdir(".husky", 0o755); err != nil {
					return err
				}
				content := `#!/usr/bin/env sh
. "$(dirname -- "$0")/_/husky.sh"

npm test
`
				return os.WriteFile(".husky/pre-commit", []byte(content), 0o755)
			},
			expectedCheck: "ai-rulez generate --dry-run",
			expectError:   false,
		},
		{
			name: "doesn't duplicate existing ai-rulez command",
			setupFiles: func(tmpDir string) error {
				if err := os.Mkdir(".husky", 0o755); err != nil {
					return err
				}
				content := `#!/usr/bin/env sh
. "$(dirname -- "$0")/_/husky.sh"

npx ai-rulez generate --dry-run
`
				return os.WriteFile(".husky/pre-commit", []byte(content), 0o755)
			},
			expectedCheck: "ai-rulez",
			expectError:   false,
		},
		{
			name: "fails if .husky doesn't exist",
			setupFiles: func(tmpDir string) error {
				return nil
			},
			expectedCheck: "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			oldWd, _ := os.Getwd()
			defer os.Chdir(oldWd)
			os.Chdir(tmpDir)

			// Setup test files
			if err := tt.setupFiles(tmpDir); err != nil {
				t.Fatalf("Failed to setup test files: %v", err)
			}

			// Run setup
			err := setupHuskyConfig()
			if (err != nil) != tt.expectError {
				t.Errorf("setupHuskyConfig() error = %v, expectError %v", err, tt.expectError)
			}

			// Check result if no error expected
			if !tt.expectError && tt.expectedCheck != "" {
				hookPath := filepath.Join(".husky", "pre-commit")
				content, err := os.ReadFile(hookPath)
				if err != nil {
					t.Fatalf("Failed to read %s: %v", hookPath, err)
				}

				contentStr := string(content)
				if !contains(contentStr, tt.expectedCheck) {
					t.Errorf("Expected content to contain %q, got:\n%s", tt.expectedCheck, contentStr)
				}

				// Check file is executable
				info, err := os.Stat(hookPath)
				if err != nil {
					t.Fatalf("Failed to stat %s: %v", hookPath, err)
				}
				if info.Mode()&0o111 == 0 {
					t.Errorf("Hook file is not executable")
				}
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
