package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected *config.Config
		wantErr  bool
	}{
		{
			name: "valid config",
			content: `metadata:
  name: "test project"
  version: "1.0.0"
  description: "test description"
outputs:
  - file: "CLAUDE.md"
rules:
  - name: "test rule"
    priority: 10
    content: "test content"`,
			expected: &config.Config{
				Metadata: config.Metadata{
					Name:        "test project",
					Version:     "1.0.0",
					Description: "test description",
				},
				Outputs: []config.Output{
					{
						File: "CLAUDE.md",
					},
				},
				Rules: []config.Rule{
					{
						Name:     "test rule",
						Priority: 10,
						Content:  "test content",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "minimal config",
			content: `metadata:
  name: "minimal"
outputs:
  - file: "CLAUDE.md"`,
			expected: &config.Config{
				Metadata: config.Metadata{
					Name: "minimal",
				},
				Outputs: []config.Output{
					{
						File: "CLAUDE.md",
					},
				},
			},
			wantErr: false,
		},
		{
			name:     "invalid yaml",
			content:  "invalid: yaml: [",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary file
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(tt.content), 0o644)
			require.NoError(t, err)

			// Load config
			result, err := config.LoadConfig(configFile)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSaveConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "test",
			Version:     "1.0.0",
			Description: "test config",
		},
		Outputs: []config.Output{
			{
				File: "CLAUDE.md",
			},
		},
		Rules: []config.Rule{
			{
				Name:     "test rule",
				Priority: 10,
				Content:  "test content",
			},
		},
	}

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "subdir", "config.yaml")

	err := config.SaveConfig(cfg, configFile)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(configFile)
	assert.NoError(t, err)

	// Load and verify content
	loaded, err := config.LoadConfig(configFile)
	require.NoError(t, err)
	assert.Equal(t, cfg, loaded)
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &config.Config{
				Metadata: config.Metadata{Name: "test"},
				Outputs: []config.Output{
					{File: "CLAUDE.md"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: &config.Config{
				Metadata: config.Metadata{},
				Outputs: []config.Output{
					{File: "CLAUDE.md"},
				},
			},
			wantErr: true,
			errMsg:  "metadata.name is required",
		},
		{
			name: "no outputs",
			config: &config.Config{
				Metadata: config.Metadata{Name: "test"},
				Outputs:  []config.Output{},
			},
			wantErr: true,
			errMsg:  "at least one output must be defined",
		},
		{
			name: "output missing file",
			config: &config.Config{
				Metadata: config.Metadata{Name: "test"},
				Outputs: []config.Output{
					{},
				},
			},
			wantErr: true,
			errMsg:  "output[0].file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
