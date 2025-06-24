package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/airules/internal/config"
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
			name: "finds .airules.yaml in current directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, ".airules.yaml")
				err := os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)
				return tmpDir
			},
			wantFile: ".airules.yaml",
		},
		{
			name: "finds airules.yaml in current directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "airules.yaml")
				err := os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)
				return tmpDir
			},
			wantFile: "airules.yaml",
		},
		{
			name: "finds config in parent directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "subdir", "nested")
				err := os.MkdirAll(subDir, 0755)
				require.NoError(t, err)

				configPath := filepath.Join(tmpDir, ".airules.yaml")
				err = os.WriteFile(configPath, []byte("test"), 0644)
				require.NoError(t, err)

				return subDir
			},
			wantFile: ".airules.yaml",
		},
		{
			name: "prefers .airules.yaml over airules.yaml",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create both files
				err := os.WriteFile(filepath.Join(tmpDir, ".airules.yaml"), []byte("test"), 0644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(tmpDir, "airules.yaml"), []byte("test"), 0644)
				require.NoError(t, err)

				return tmpDir
			},
			wantFile: ".airules.yaml",
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
				assert.Contains(t, err.Error(), "no airules configuration file found")
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
					".airules.yaml",
					"project1/airules.yaml",
					"project2/.airules.yaml",
					"nested/deep/airules.yaml",
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
			wantCount: 4,
		},
		{
			name: "skips hidden directories",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create config files
				configs := []string{
					"airules.yaml",
					".hidden/airules.yaml", // Should be skipped
					"visible/.airules.yaml",
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
				assert.Contains(t, err.Error(), "no airules configuration files found")
			} else {
				assert.NoError(t, err)
				assert.Len(t, configs, tt.wantCount)
			}
		})
	}
}
