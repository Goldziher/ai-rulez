package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestFindConfigFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns the start directory
		wantFile  string                    // Expected filename (not full path)
		wantError bool
	}{
		{
			name: "finds .ai-rulez.yaml in current directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, ".ai-rulez.yaml")
				err := os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)
				return tmpDir
			},
			wantFile: ".ai-rulez.yaml",
		},
		{
			name: "finds ai-rulez.yaml in current directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
				err := os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)
				return tmpDir
			},
			wantFile: "ai-rulez.yaml",
		},
		{
			name: "finds config in parent directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "subdir", "nested")
				err := os.MkdirAll(subDir, 0755)
				require.NoError(t, err)

				configPath := filepath.Join(tmpDir, ".ai-rulez.yaml")
				err = os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)

				return subDir
			},
			wantFile: ".ai-rulez.yaml",
		},
		{
			name: "prefers .ai-rulez.yaml over ai-rulez.yaml",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create both files
				err := os.WriteFile(filepath.Join(tmpDir, ".ai-rulez.yaml"), []byte("test"), 0644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(tmpDir, "ai-rulez.yaml"), []byte("test"), 0644)
				require.NoError(t, err)

				return tmpDir
			},
			wantFile: ".ai-rulez.yaml",
		},
		{
			name: "finds ai_rulez.yaml",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
				err := os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)
				return tmpDir
			},
			wantFile: "ai_rulez.yaml",
		},
		{
			name: "finds .ai_rulez.yaml",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, ".ai_rulez.yaml")
				err := os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)
				return tmpDir
			},
			wantFile: ".ai_rulez.yaml",
		},
		{
			name: "finds .yml variant",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "ai-rulez.yml")
				err := os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)
				return tmpDir
			},
			wantFile: "ai-rulez.yml",
		},
		{
			name: "priority order: .ai-rulez.yaml first",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create all variants
				files := []string{
					".ai-rulez.yaml", "ai-rulez.yaml",
					".ai_rulez.yaml", "ai_rulez.yaml",
					".ai-rulez.yml", "ai-rulez.yml",
				}
				for _, f := range files {
					err := os.WriteFile(filepath.Join(tmpDir, f), []byte("test"), 0644)
					require.NoError(t, err)
				}

				return tmpDir
			},
			wantFile: ".ai-rulez.yaml",
		},
		{
			name: "no config file found",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			startDir := tt.setup(t)
			configPath, err := config.FindConfigFile(startDir)

			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "no configuration file found")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantFile, filepath.Base(configPath))
			}
		})
	}
}

func TestFindAllConfigFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns the root directory
		wantCount int
		wantError bool
	}{
		{
			name: "finds multiple config files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create config files in different directories
				configs := []string{
					".ai-rulez.yaml",
					"project1/ai-rulez.yaml",
					"project2/.ai-rulez.yaml",
					"nested/deep/ai-rulez.yaml",
					"project3/ai_rulez.yaml",
					"project4/.ai_rulez.yml",
					"project5/ai-rulez.yml",
				}

				for _, config := range configs {
					path := filepath.Join(tmpDir, config)
					err := os.MkdirAll(filepath.Dir(path), 0755)
					require.NoError(t, err)
					err = os.WriteFile(path, []byte("test"), 0644)
					require.NoError(t, err)
				}

				return tmpDir
			},
			wantCount: 7,
		},
		{
			name: "skips hidden directories",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create config files
				configs := []string{
					"ai-rulez.yaml",
					".hidden/ai-rulez.yaml", // Should be skipped
					"visible/.ai-rulez.yaml",
				}

				for _, config := range configs {
					path := filepath.Join(tmpDir, config)
					err := os.MkdirAll(filepath.Dir(path), 0755)
					require.NoError(t, err)
					err = os.WriteFile(path, []byte("test"), 0644)
					require.NoError(t, err)
				}

				return tmpDir
			},
			wantCount: 2, // Only non-hidden directories
		},
		{
			name: "no config files found",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rootDir := tt.setup(t)
			configs, err := config.FindAllConfigFiles(rootDir)

			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "no configuration files found")
			} else {
				assert.NoError(t, err)
				assert.Len(t, configs, tt.wantCount)
			}
		})
	}
}
