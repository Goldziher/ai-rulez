package config

import (
	"errors"
	"testing"
)

func TestRegisterPresetV3(t *testing.T) {
	// Create a mock generator
	mockGen := &mockPresetGenerator{name: "test-preset"}

	// Register it
	RegisterPresetV3("test-preset", mockGen)

	// Verify it was registered
	gen, err := GetPresetGeneratorV3("test-preset")
	if err != nil {
		t.Fatalf("GetPresetGeneratorV3() error = %v", err)
	}

	if gen.GetName() != "test-preset" {
		t.Errorf("GetPresetGeneratorV3() got name %v, want %v", gen.GetName(), "test-preset")
	}

	// Clean up
	delete(PresetRegistryV3, "test-preset")
}

func TestGetPresetGeneratorV3_NotFound(t *testing.T) {
	_, err := GetPresetGeneratorV3("nonexistent-preset")
	if err == nil {
		t.Error("GetPresetGeneratorV3() expected error for nonexistent preset")
	}

	if !errors.Is(err, ErrInvalidPresetV3) {
		t.Errorf("GetPresetGeneratorV3() error = %v, want %v", err, ErrInvalidPresetV3)
	}
}

func TestGeneratePresetsV3_NoContent(t *testing.T) {
	cfg := &ConfigV3{
		Name:    "test",
		Version: "3.0",
		Presets: []PresetV3{
			{BuiltIn: "claude"},
		},
		Content: nil,
	}

	_, err := GeneratePresetsV3(cfg)
	if !errors.Is(err, ErrNoContent) {
		t.Errorf("GeneratePresetsV3() error = %v, want %v", err, ErrNoContent)
	}
}

func TestGeneratePresetsV3_BuiltIn(t *testing.T) {
	// Register a mock generator
	mockGen := &mockPresetGenerator{name: "mock"}
	RegisterPresetV3("mock", mockGen)
	defer delete(PresetRegistryV3, "mock")

	cfg := &ConfigV3{
		Name:    "test",
		Version: "3.0",
		BaseDir: "/test",
		Presets: []PresetV3{
			{BuiltIn: "mock"},
		},
		Content: &ContentTreeV3{
			Rules: []ContentFile{
				{Name: "rule1", Content: "content"},
			},
		},
	}

	results, err := GeneratePresetsV3(cfg)
	if err != nil {
		t.Fatalf("GeneratePresetsV3() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("GeneratePresetsV3() got %d results, want 1", len(results))
	}

	outputs, ok := results["mock"]
	if !ok {
		t.Error("Expected 'mock' preset in results")
	}

	if len(outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(outputs))
	}
}

func TestGeneratePresetsV3_CustomPreset(t *testing.T) {
	// Set up custom preset factory
	originalFactory := CustomPresetGeneratorFactory
	CustomPresetGeneratorFactory = func(preset PresetV3) PresetGeneratorV3 {
		return &mockPresetGenerator{name: preset.Name}
	}
	defer func() { CustomPresetGeneratorFactory = originalFactory }()

	cfg := &ConfigV3{
		Name:    "test",
		Version: "3.0",
		BaseDir: "/test",
		Presets: []PresetV3{
			{
				Name: "custom-preset",
				Type: PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
		},
		Content: &ContentTreeV3{
			Rules: []ContentFile{
				{Name: "rule1", Content: "content"},
			},
		},
	}

	results, err := GeneratePresetsV3(cfg)
	if err != nil {
		t.Fatalf("GeneratePresetsV3() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("GeneratePresetsV3() got %d results, want 1", len(results))
	}

	outputs, ok := results["custom-preset"]
	if !ok {
		t.Error("Expected 'custom-preset' in results")
	}

	if len(outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(outputs))
	}
}

func TestGeneratePresetsV3_CustomPresetFactoryNotSet(t *testing.T) {
	// Save and clear factory
	originalFactory := CustomPresetGeneratorFactory
	CustomPresetGeneratorFactory = nil
	defer func() { CustomPresetGeneratorFactory = originalFactory }()

	cfg := &ConfigV3{
		Name:    "test",
		Version: "3.0",
		BaseDir: "/test",
		Presets: []PresetV3{
			{
				Name: "custom-preset",
				Type: PresetTypeMarkdown,
				Path: "CUSTOM.md",
			},
		},
		Content: &ContentTreeV3{},
	}

	_, err := GeneratePresetsV3(cfg)
	if err == nil {
		t.Error("GeneratePresetsV3() expected error when factory not set")
	}

	if err != nil && err.Error() != "custom preset generator factory not initialized" {
		t.Errorf("GeneratePresetsV3() error = %v, want factory not initialized error", err)
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

func (m *mockPresetGenerator) Generate(content *ContentTreeV3, baseDir string, config *ConfigV3) ([]OutputFileV3, error) {
	return []OutputFileV3{
		{
			Path:    baseDir + "/output.md",
			Content: "test output",
		},
	}, nil
}
