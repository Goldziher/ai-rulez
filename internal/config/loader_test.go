package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestLoadConfigWithIncludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "config with single include",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "include.yaml"
outputs:
  - file: "CLAUDE.md"
rules:
  - name: "main rule"
    content: "main content"`,
				"include.yaml": `metadata:
  name: "included"
outputs:
  - file: "output.md"
rules:
  - name: "included rule"
    content: "included content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, "main", cfg.Metadata.Name)
				assert.Len(t, cfg.Rules, 2)
				assert.Equal(t, "main rule", cfg.Rules[0].Name)
				assert.Equal(t, "included rule", cfg.Rules[1].Name)
				assert.Empty(t, cfg.Includes)
			},
		},
		{
			name: "config with multiple includes",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "first.yaml"
  - "second.yaml"
outputs:
  - file: "CLAUDE.md"`,
				"first.yaml": `metadata:
  name: "first"
outputs:
  - file: "output.md"
rules:
  - name: "first rule"
    content: "first content"`,
				"second.yaml": `metadata:
  name: "second"
outputs:
  - file: "output.md"
rules:
  - name: "second rule"
    content: "second content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Len(t, cfg.Rules, 2)
				assert.Equal(t, "first rule", cfg.Rules[0].Name)
				assert.Equal(t, "second rule", cfg.Rules[1].Name)
			},
		},
		{
			name: "nested includes",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "level1.yaml"
outputs:
  - file: "CLAUDE.md"`,
				"level1.yaml": `metadata:
  name: "level1"
outputs:
  - file: "output.md"
includes:
  - "level2.yaml"
rules:
  - name: "level1 rule"
    content: "level1 content"`,
				"level2.yaml": `metadata:
  name: "level2"
outputs:
  - file: "output.md"
rules:
  - name: "level2 rule"
    content: "level2 content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Len(t, cfg.Rules, 2)
				assert.Equal(t, "level1 rule", cfg.Rules[0].Name)
				assert.Equal(t, "level2 rule", cfg.Rules[1].Name)
			},
		},
		{
			name: "missing include file",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "missing.yaml"
outputs:
  - file: "CLAUDE.md"`,
			},
			wantErr: true,
			check:   nil,
		},
		{
			name: "circular include detection",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "circular.yaml"
outputs:
  - file: "CLAUDE.md"`,
				"circular.yaml": `metadata:
  name: "circular"
outputs:
  - file: "output.md"
includes:
  - "main.yaml"`,
			},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary directory and files
			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			// Load config
			mainPath := filepath.Join(tmpDir, "main.yaml")
			cfg, err := config.LoadConfigWithIncludesWithoutProfiles(mainPath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, cfg)
				if tt.check != nil {
					tt.check(t, cfg)
				}
			}
		})
	}
}

func TestMergeRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ruleSets [][]config.Rule
		expected []config.Rule
	}{
		{
			name: "merge two rule sets",
			ruleSets: [][]config.Rule{
				{
					{Name: "rule1", Content: "content1"},
					{Name: "rule2", Content: "content2"},
				},
				{
					{Name: "rule3", Content: "content3"},
				},
			},
			expected: []config.Rule{
				{Name: "rule1", Content: "content1"},
				{Name: "rule2", Content: "content2"},
				{Name: "rule3", Content: "content3"},
			},
		},
		{
			name: "later rules override earlier ones",
			ruleSets: [][]config.Rule{
				{
					{Name: "rule1", Content: "original"},
				},
				{
					{Name: "rule1", Content: "override"},
				},
			},
			expected: []config.Rule{
				{Name: "rule1", Content: "override"},
			},
		},
		{
			name:     "empty rule sets",
			ruleSets: [][]config.Rule{},
			expected: []config.Rule{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := config.MergeRules(tt.ruleSets...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateIncludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *config.Config
		files   map[string]string
		wantErr bool
	}{
		{
			name: "valid includes",
			config: &config.Config{
				Includes: []string{"valid.yaml"},
			},
			files: map[string]string{
				"valid.yaml": `metadata:
  name: "valid"
outputs:
  - file: "CLAUDE.md"`,
			},
			wantErr: false,
		},
		{
			name: "missing include file",
			config: &config.Config{
				Includes: []string{"missing.yaml"},
			},
			files:   map[string]string{},
			wantErr: true,
		},
		{
			name: "invalid YAML in include",
			config: &config.Config{
				Includes: []string{"invalid.yaml"},
			},
			files: map[string]string{
				"invalid.yaml": "invalid: yaml: [",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			err := config.ValidateIncludes(tt.config, tmpDir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateOutputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outputs []config.Output
		wantErr bool
	}{
		{
			name:    "valid outputs",
			outputs: []config.Output{{File: "test.md"}},
			wantErr: false,
		},
		{
			name:    "empty outputs",
			outputs: []config.Output{},
			wantErr: true,
		},
		{
			name:    "missing file",
			outputs: []config.Output{{}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateOutputs(tt.outputs)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMergeRulesWithIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ruleSets [][]config.Rule
		expected []config.Rule
	}{
		{
			name: "rules with IDs override by ID",
			ruleSets: [][]config.Rule{
				{
					{ID: "rule1", Name: "Rule 1", Content: "original"},
					{Name: "Rule 2", Content: "no ID rule"},
				},
				{
					{ID: "rule1", Name: "Rule 1 Override", Content: "overridden"},
				},
			},
			expected: []config.Rule{
				{ID: "rule1", Name: "Rule 1 Override", Content: "overridden"},
				{Name: "Rule 2", Content: "no ID rule"},
			},
		},
		{
			name: "rules without IDs still merge by name",
			ruleSets: [][]config.Rule{
				{
					{Name: "Rule 1", Content: "original"},
				},
				{
					{Name: "Rule 1", Content: "overridden"},
				},
			},
			expected: []config.Rule{
				{Name: "Rule 1", Content: "overridden"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := config.MergeRules(tt.ruleSets...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadConfigWithLocalFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "local file overrides rules by ID",
			files: map[string]string{
				"test.yaml": `metadata:
  name: "main"
profile: "none"
outputs:
  - file: "CLAUDE.md"
rules:
  - id: "rule1"
    name: "Rule 1"
    content: "original content"`,
				"test.local.yaml": `metadata:
  name: "local overrides"
outputs:
  - file: "local.md"
rules:
  - id: "rule1"
    name: "Rule 1 Overridden"
    content: "LOCAL: overridden content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, "main", cfg.Metadata.Name)
				assert.Len(t, cfg.Rules, 1)
				assert.Equal(t, "Rule 1 Overridden", cfg.Rules[0].Name)
				assert.Equal(t, "LOCAL: overridden content", cfg.Rules[0].Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			mainFile := filepath.Join(tmpDir, "test.yaml")
			cfg, err := config.LoadConfigWithIncludesWithoutProfiles(mainFile)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			tt.check(t, cfg)
		})
	}
}
