package config

import (
	"errors"
	"testing"
)

func TestRegisterPreset(t *testing.T) {
	// Create a mock generator
	mockGen := &mockPresetGenerator{name: "test-preset"}

	// Register it
	RegisterPreset("test-preset", mockGen)

	// Verify it was registered
	gen, err := GetPresetGenerator("test-preset")
	if err != nil {
		t.Fatalf("GetPresetGenerator() error = %v", err)
	}

	if gen.GetName() != "test-preset" {
		t.Errorf("GetPresetGenerator() got name %v, want %v", gen.GetName(), "test-preset")
	}

	// Clean up
	delete(PresetRegistry, "test-preset")
}

func TestGetPresetGenerator_NotFound(t *testing.T) {
	_, err := GetPresetGenerator("nonexistent-preset")
	if err == nil {
		t.Error("GetPresetGenerator() expected error for nonexistent preset")
	}

	if !errors.Is(err, ErrInvalidPreset) {
		t.Errorf("GetPresetGenerator() error = %v, want %v", err, ErrInvalidPreset)
	}
}

func TestGeneratePresets_NoContent(t *testing.T) {
	cfg := &Config{
		Name:    "test",
		Version: "3.0",
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
	mockGen := &mockPresetGenerator{name: "mock"}
	RegisterPreset("mock", mockGen)
	defer delete(PresetRegistry, "mock")

	cfg := &Config{
		Name:    "test",
		Version: "3.0",
		BaseDir: "/test",
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
	originalFactory := CustomPresetGeneratorFactory
	CustomPresetGeneratorFactory = func(preset Preset) PresetGenerator {
		return &mockPresetGenerator{name: preset.Name}
	}
	defer func() { CustomPresetGeneratorFactory = originalFactory }()

	cfg := &Config{
		Name:    "test",
		Version: "3.0",
		BaseDir: "/test",
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
	// Save and clear factory
	originalFactory := CustomPresetGeneratorFactory
	CustomPresetGeneratorFactory = nil
	defer func() { CustomPresetGeneratorFactory = originalFactory }()

	cfg := &Config{
		Name:    "test",
		Version: "3.0",
		BaseDir: "/test",
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
