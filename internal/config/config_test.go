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

			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(tt.content), 0o644)
			require.NoError(t, err)

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
				Path: "CLAUDE.md",
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

	_, err = os.Stat(configFile)
	assert.NoError(t, err)

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
					{Path: "CLAUDE.md"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: &config.Config{
				Metadata: config.Metadata{},
				Outputs: []config.Output{
					{Path: "CLAUDE.md"},
				},
			},
			wantErr: true,
			errMsg:  "required field 'metadata.name' is missing",
		},
		{
			name: "no outputs",
			config: &config.Config{
				Metadata: config.Metadata{Name: "test"},
				Outputs:  []config.Output{},
			},
			wantErr: true,
			errMsg:  "required field 'outputs' is missing",
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
			errMsg:  "required field 'outputs[0].path or outputs[0].file' is missing",
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

func TestOutput_GetPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   config.Output
		expected string
	}{
		{
			name: "uses path when set",
			output: config.Output{
				Path: "/path/to/output/",
				File: "file.md",
			},
			expected: "/path/to/output/",
		},
		{
			name: "falls back to file when path empty",
			output: config.Output{
				File: "file.md",
			},
			expected: "file.md",
		},
		{
			name: "path takes precedence",
			output: config.Output{
				Path: "agents/",
				File: "old.md",
			},
			expected: "agents/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.output.GetPath()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOutput_IsDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   config.Output
		expected bool
	}{
		{
			name: "directory with trailing slash in path",
			output: config.Output{
				Path: "/path/to/output/",
			},
			expected: true,
		},
		{
			name: "file without trailing slash in path",
			output: config.Output{
				Path: "/path/to/file.md",
			},
			expected: false,
		},
		{
			name: "directory with trailing slash in file",
			output: config.Output{
				File: "output/",
			},
			expected: true,
		},
		{
			name: "file without trailing slash",
			output: config.Output{
				File: "README.md",
			},
			expected: false,
		},
		{
			name:     "empty path and file",
			output:   config.Output{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.output.IsDirectory()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOutput_GetOutputType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   config.Output
		expected string
	}{
		{
			name: "explicitly set type",
			output: config.Output{
				Type: "agent",
			},
			expected: "agent",
		},
		{
			name:     "default to rule when not set",
			output:   config.Output{},
			expected: "rule",
		},
		{
			name: "custom type",
			output: config.Output{
				Type: "custom",
			},
			expected: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.output.GetOutputType()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOutput_GetNamingScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   config.Output
		expected string
	}{
		{
			name: "explicitly set naming scheme",
			output: config.Output{
				NamingScheme: "{type}-{name}.md",
			},
			expected: "{type}-{name}.md",
		},
		{
			name: "default naming scheme for agents",
			output: config.Output{
				Type: "agent",
			},
			expected: "{name}.md",
		},
		{
			name: "default naming scheme for rules",
			output: config.Output{
				Type: "rule",
			},
			expected: "{type}.md",
		},
		{
			name:     "default when type not set",
			output:   config.Output{},
			expected: "{type}.md",
		},
		{
			name: "custom naming with index",
			output: config.Output{
				NamingScheme: "{index:03d}-{name}.yaml",
			},
			expected: "{index:03d}-{name}.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.output.GetNamingScheme()
			assert.Equal(t, tt.expected, result)
		})
	}
}
