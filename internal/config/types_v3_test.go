package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestPresetV3_IsBuiltIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preset   config.PresetV3
		expected bool
	}{
		{
			name:     "built-in preset",
			preset:   config.PresetV3{BuiltIn: "claude"},
			expected: true,
		},
		{
			name: "custom preset",
			preset: config.PresetV3{
				Name: "custom",
				Type: config.PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
			expected: false,
		},
		{
			name:     "empty preset",
			preset:   config.PresetV3{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.preset.IsBuiltIn()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPresetV3_GetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preset   config.PresetV3
		expected string
	}{
		{
			name:     "built-in preset name",
			preset:   config.PresetV3{BuiltIn: "claude"},
			expected: "claude",
		},
		{
			name: "custom preset name",
			preset: config.PresetV3{
				Name: "my-custom-preset",
				Type: config.PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
			expected: "my-custom-preset",
		},
		{
			name:     "empty preset",
			preset:   config.PresetV3{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.preset.GetName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPresetV3_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preset   config.PresetV3
		expected bool
	}{
		{
			name:     "valid built-in preset",
			preset:   config.PresetV3{BuiltIn: "claude"},
			expected: true,
		},
		{
			name:     "invalid built-in preset",
			preset:   config.PresetV3{BuiltIn: "invalid"},
			expected: false,
		},
		{
			name: "valid custom preset",
			preset: config.PresetV3{
				Name: "custom",
				Type: config.PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
			expected: true,
		},
		{
			name: "invalid custom preset - missing name",
			preset: config.PresetV3{
				Type: config.PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
			expected: false,
		},
		{
			name: "invalid custom preset - missing type",
			preset: config.PresetV3{
				Name: "custom",
				Path: "CUSTOM.md",
			},
			expected: false,
		},
		{
			name: "invalid custom preset - missing path",
			preset: config.PresetV3{
				Name: "custom",
				Type: config.PresetTypeMarkdown,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.preset.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetadataV3_GetPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata *config.MetadataV3
		expected config.Priority
	}{
		{
			name:     "nil metadata defaults to medium",
			metadata: nil,
			expected: config.PriorityMedium,
		},
		{
			name:     "empty priority defaults to medium",
			metadata: &config.MetadataV3{},
			expected: config.PriorityMedium,
		},
		{
			name:     "critical priority",
			metadata: &config.MetadataV3{Priority: "critical"},
			expected: config.PriorityCritical,
		},
		{
			name:     "high priority",
			metadata: &config.MetadataV3{Priority: "high"},
			expected: config.PriorityHigh,
		},
		{
			name:     "low priority",
			metadata: &config.MetadataV3{Priority: "low"},
			expected: config.PriorityLow,
		},
		{
			name:     "invalid priority defaults to medium",
			metadata: &config.MetadataV3{Priority: "invalid"},
			expected: config.PriorityMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetPriority()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetadataV3_HasTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata *config.MetadataV3
		expected bool
	}{
		{
			name:     "nil metadata has no targets",
			metadata: nil,
			expected: false,
		},
		{
			name:     "empty targets",
			metadata: &config.MetadataV3{},
			expected: false,
		},
		{
			name:     "has targets",
			metadata: &config.MetadataV3{Targets: []string{"*.md"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.HasTargets()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigV3_ShouldUpdateGitignore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   config.ConfigV3
		expected bool
	}{
		{
			name:     "nil gitignore defaults to true",
			config:   config.ConfigV3{},
			expected: true,
		},
		{
			name: "explicitly enabled",
			config: config.ConfigV3{
				Gitignore: &[]bool{true}[0],
			},
			expected: true,
		},
		{
			name: "explicitly disabled",
			config: config.ConfigV3{
				Gitignore: &[]bool{false}[0],
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ShouldUpdateGitignore()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigV3_GetDefaultProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   config.ConfigV3
		expected string
	}{
		{
			name:     "no default profile",
			config:   config.ConfigV3{},
			expected: "",
		},
		{
			name: "has default profile",
			config: config.ConfigV3{
				Default: "full",
			},
			expected: "full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetDefaultProfile()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigV3_GetProfileDomains(t *testing.T) {
	t.Parallel()

	cfg := config.ConfigV3{
		Default: "full",
		Profiles: map[string][]string{
			"full":     {"backend", "frontend", "qa"},
			"backend":  {"backend", "qa"},
			"frontend": {"frontend", "qa"},
		},
	}

	tests := []struct {
		name     string
		profile  string
		expected []string
	}{
		{
			name:     "empty profile uses default",
			profile:  "",
			expected: []string{"backend", "frontend", "qa"},
		},
		{
			name:     "specific profile",
			profile:  "backend",
			expected: []string{"backend", "qa"},
		},
		{
			name:     "non-existent profile",
			profile:  "invalid",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.GetProfileDomains(tt.profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigV3_HasProfile(t *testing.T) {
	t.Parallel()

	cfg := config.ConfigV3{
		Profiles: map[string][]string{
			"full":    {"backend", "frontend"},
			"backend": {"backend"},
		},
	}

	tests := []struct {
		name     string
		profile  string
		expected bool
	}{
		{
			name:     "existing profile",
			profile:  "full",
			expected: true,
		},
		{
			name:     "non-existent profile",
			profile:  "invalid",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.HasProfile(tt.profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigV3_IsV3(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   config.ConfigV3
		expected bool
	}{
		{
			name: "valid v3 config",
			config: config.ConfigV3{
				Version: "3.0",
			},
			expected: true,
		},
		{
			name: "invalid version",
			config: config.ConfigV3{
				Version: "2.0",
			},
			expected: false,
		},
		{
			name:     "empty version",
			config:   config.ConfigV3{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsV3()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigV3_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    config.ConfigV3
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid config",
			config: config.ConfigV3{
				Version: "3.0",
				Name:    "test-project",
				Presets: []config.PresetV3{
					{BuiltIn: "claude"},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid version",
			config: config.ConfigV3{
				Version: "2.0",
				Name:    "test-project",
			},
			expectErr: true,
			errMsg:    "invalid version",
		},
		{
			name: "missing name",
			config: config.ConfigV3{
				Version: "3.0",
				Presets: []config.PresetV3{
					{BuiltIn: "claude"},
				},
			},
			expectErr: true,
			errMsg:    "required field 'name' is missing",
		},
		{
			name: "invalid default profile",
			config: config.ConfigV3{
				Version: "3.0",
				Name:    "test-project",
				Default: "nonexistent",
				Presets: []config.PresetV3{
					{BuiltIn: "claude"},
				},
				Profiles: map[string][]string{
					"backend": {"backend"},
				},
			},
			expectErr: true,
			errMsg:    "does not exist in profiles",
		},
		{
			name: "invalid preset",
			config: config.ConfigV3{
				Version: "3.0",
				Name:    "test-project",
				Presets: []config.PresetV3{
					{BuiltIn: "invalid-preset"},
				},
			},
			expectErr: true,
			errMsg:    "unknown built-in preset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateV3()
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestContentTreeV3_GetRulesForDomains(t *testing.T) {
	t.Parallel()

	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "root-rule-1", Path: "/rules/rule1.md"},
			{Name: "root-rule-2", Path: "/rules/rule2.md"},
		},
		Domains: map[string]*config.DomainV3{
			"backend": {
				Name: "backend",
				Rules: []config.ContentFile{
					{Name: "backend-rule-1", Path: "/domains/backend/rules/rule1.md"},
				},
			},
			"frontend": {
				Name: "frontend",
				Rules: []config.ContentFile{
					{Name: "frontend-rule-1", Path: "/domains/frontend/rules/rule1.md"},
				},
			},
		},
	}

	tests := []struct {
		name          string
		domains       []string
		expectedCount int
	}{
		{
			name:          "no domains - only root",
			domains:       nil,
			expectedCount: 2,
		},
		{
			name:          "backend domain",
			domains:       []string{"backend"},
			expectedCount: 3, // 2 root + 1 backend
		},
		{
			name:          "all domains",
			domains:       []string{"backend", "frontend"},
			expectedCount: 4, // 2 root + 1 backend + 1 frontend
		},
		{
			name:          "non-existent domain",
			domains:       []string{"invalid"},
			expectedCount: 2, // only root
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tree.GetRulesForDomains(tt.domains)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestContentTreeV3_GetAllContentFiles(t *testing.T) {
	t.Parallel()

	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1"},
		},
		Context: []config.ContentFile{
			{Name: "context1"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1"},
		},
		Domains: map[string]*config.DomainV3{
			"backend": {
				Name: "backend",
				Rules: []config.ContentFile{
					{Name: "backend-rule1"},
				},
				Context: []config.ContentFile{
					{Name: "backend-context1"},
				},
			},
		},
	}

	files := tree.GetAllContentFiles()
	assert.Len(t, files, 5) // 1 rule + 1 context + 1 skill + 1 backend rule + 1 backend context
}

func TestContentFile_GetFileExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		file     config.ContentFile
		expected string
	}{
		{
			name:     "markdown file",
			file:     config.ContentFile{Path: "/rules/test.md"},
			expected: ".md",
		},
		{
			name:     "json file",
			file:     config.ContentFile{Path: "/config.json"},
			expected: ".json",
		},
		{
			name:     "no extension",
			file:     config.ContentFile{Path: "/README"},
			expected: "",
		},
		{
			name:     "multiple dots",
			file:     config.ContentFile{Path: "/test.backup.md"},
			expected: ".md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.file.GetFileExtension()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContentFile_IsMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		file     config.ContentFile
		expected bool
	}{
		{
			name:     "md extension",
			file:     config.ContentFile{Path: "/test.md"},
			expected: true,
		},
		{
			name:     "markdown extension",
			file:     config.ContentFile{Path: "/test.markdown"},
			expected: true,
		},
		{
			name:     "json extension",
			file:     config.ContentFile{Path: "/test.json"},
			expected: false,
		},
		{
			name:     "no extension",
			file:     config.ContentFile{Path: "/README"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.file.IsMarkdown()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigV3_GetContentForProfile(t *testing.T) {
	t.Parallel()

	t.Run("no content loaded", func(t *testing.T) {
		cfg := &config.ConfigV3{
			Version: "3.0",
			Name:    "test",
		}

		_, err := cfg.GetContentForProfile("backend")
		require.Error(t, err)
		assert.Equal(t, config.ErrNoContent, err)
	})

	t.Run("with content", func(t *testing.T) {
		cfg := &config.ConfigV3{
			Version: "3.0",
			Name:    "test",
			Default: "backend",
			Profiles: map[string][]string{
				"backend": {"backend"},
			},
			Content: &config.ContentTreeV3{
				Rules: []config.ContentFile{
					{Name: "root-rule"},
				},
				Domains: map[string]*config.DomainV3{
					"backend": {
						Name: "backend",
						Rules: []config.ContentFile{
							{Name: "backend-rule"},
						},
					},
				},
			},
		}

		content, err := cfg.GetContentForProfile("backend")
		require.NoError(t, err)
		require.NotNil(t, content)
		assert.Len(t, content.Rules, 2) // root + backend
	})
}
