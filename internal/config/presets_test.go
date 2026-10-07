package config

import (
	"errors"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	// Arrange
	r := NewRegistry()
	r.Register("test-preset", &mockPresetGenerator{name: "test-preset"})

	// Act
	gen, err := r.Generator("test-preset")

	// Assert
	if err != nil {
		t.Fatalf("Generator() error = %v", err)
	}
	if gen.GetName() != "test-preset" {
		t.Errorf("Generator() got name %v, want %v", gen.GetName(), "test-preset")
	}
	if names := r.Names(); len(names) != 1 || names[0] != "test-preset" {
		t.Errorf("Names() = %v", names)
	}
}

func TestRegistry_Generator_NotFound(t *testing.T) {
	for name, r := range map[string]*Registry{"empty": NewRegistry(), "nil": nil} {
		t.Run(name, func(t *testing.T) {
			_, err := r.Generator("nonexistent-preset")
			if !errors.Is(err, ErrInvalidPreset) {
				t.Errorf("Generator() error = %v, want %v", err, ErrInvalidPreset)
			}
		})
	}
}

func TestGeneratePresets_NoContent(t *testing.T) {
	cfg := &Config{
		Name:    "test",
		Version: "5.0",
		Presets: []Preset{
			{BuiltIn: "claude"},
		},
		Content: nil,
	}

	_, err := GeneratePresets(cfg)
	if !errors.Is(err, ErrNoContent) {
		t.Errorf("GeneratePresets() error = %v, want %v", err, ErrNoContent)
	}
}

func TestGeneratePresets_BuiltIn(t *testing.T) {
	// Register a mock generator
	reg := NewRegistry()
	reg.Register("mock", &mockPresetGenerator{name: "mock"})

	cfg := &Config{
		Registry: reg,
		Name:     "test",
		Version:  "4.0",
		BaseDir:  "/test",
		Presets: []Preset{
			{BuiltIn: "mock"},
		},
		Content: &ContentTree{
			Rules: []ContentFile{
				{Name: "rule1", Content: "content"},
			},
		},
	}

	results, err := GeneratePresets(cfg)
	if err != nil {
		t.Fatalf("GeneratePresets() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("GeneratePresets() got %d results, want 1", len(results))
	}

	outputs, ok := results["mock"]
	if !ok {
		t.Error("Expected 'mock' preset in results")
	}

	if len(outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(outputs))
	}
}

func TestGeneratePresets_CustomPreset(t *testing.T) {
	// Set up custom preset factory
	reg := NewRegistry()
	reg.Custom = func(preset Preset) PresetGenerator { return &mockPresetGenerator{name: preset.Name} }

	cfg := &Config{
		Registry: reg,
		Name:     "test",
		Version:  "4.0",
		BaseDir:  "/test",
		Presets: []Preset{
			{
				Name: "custom-preset",
				Type: PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
		},
		Content: &ContentTree{
			Rules: []ContentFile{
				{Name: "rule1", Content: "content"},
			},
		},
	}

	results, err := GeneratePresets(cfg)
	if err != nil {
		t.Fatalf("GeneratePresets() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("GeneratePresets() got %d results, want 1", len(results))
	}

	outputs, ok := results["custom-preset"]
	if !ok {
		t.Error("Expected 'custom-preset' in results")
	}

	if len(outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(outputs))
	}
}

func TestGeneratePresets_CustomPresetFactoryNotSet(t *testing.T) {
	// A registry with no custom factory
	cfg := &Config{
		Registry: NewRegistry(),
		Name:     "test",
		Version:  "4.0",
		BaseDir:  "/test",
		Presets: []Preset{
			{
				Name: "custom-preset",
				Type: PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
		},
		Content: &ContentTree{},
	}

	_, err := GeneratePresets(cfg)
	if err == nil {
		t.Error("GeneratePresets() expected error when factory not set")
	}

	if err != nil && err.Error() != "custom preset generator factory not initialized" {
		t.Errorf("GeneratePresets() error = %v, want factory not initialized error", err)
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"spaces", "test name", "test-name"},
		{"underscores", "test_name", "test-name"},
		{"slashes", "test/name", "test-name"},
		{"special chars", "test@#$name", "testname"},
		{"mixed", "Test Name_123", "Test-Name-123"},
		{"trailing dashes", "-test-", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractSkillID(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "standard path",
			path:     "/test/skills/my-skill/SKILL.md",
			expected: "my-skill",
		},
		{
			name:     "nested path",
			path:     "/some/deep/path/skills/another-skill/SKILL.md",
			expected: "another-skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSkillID(tt.path)
			if result != tt.expected {
				t.Errorf("extractSkillID(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestCombineContent(t *testing.T) {
	slice1 := []ContentFile{
		{Name: "file1"},
		{Name: "file2"},
	}
	slice2 := []ContentFile{
		{Name: "file3"},
	}
	slice3 := []ContentFile{
		{Name: "file4"},
		{Name: "file5"},
	}

	result := combineContent(slice1, slice2, slice3)

	if len(result) != 5 {
		t.Errorf("combineContent() length = %d, want 5", len(result))
	}

	expected := []string{"file1", "file2", "file3", "file4", "file5"}
	for i, file := range result {
		if file.Name != expected[i] {
			t.Errorf("combineContent()[%d].Name = %q, want %q", i, file.Name, expected[i])
		}
	}
}

// Mock generator for testing
type mockPresetGenerator struct {
	name string
}

func (m *mockPresetGenerator) GetName() string {
	return m.name
}

func (m *mockPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{baseDir + "/output.md"}
}

func (m *mockPresetGenerator) Generate(content *ContentTree, baseDir string, config *Config) ([]OutputFile, error) {
	return []OutputFile{
		{
			Path:    baseDir + "/output.md",
			Content: "test output",
		},
	}, nil
}
