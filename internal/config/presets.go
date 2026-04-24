package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PresetGenerator defines the interface for V3 preset generators
type PresetGenerator interface {
	Generate(content *ContentTree, baseDir string, config *Config) ([]OutputFile, error)
	GetOutputPaths(baseDir string) []string
	GetName() string
}

// OutputFile represents a generated output file or directory
type OutputFile struct {
	Path    string
	Content string
	IsDir   bool
}

// PresetRegistry maps preset names to their generators
// Populated by init() functions in generator/presets/ package
var PresetRegistry = make(map[string]PresetGenerator)

// RegisterPreset registers a preset generator
func RegisterPreset(name string, generator PresetGenerator) {
	PresetRegistry[name] = generator
}

// GetPresetGenerator retrieves a preset generator by name
func GetPresetGenerator(name string) (PresetGenerator, error) {
	generator, exists := PresetRegistry[name]
	if !exists {
		return nil, ErrInvalidPreset
	}
	return generator, nil
}

// CustomPresetGeneratorFactory is a function type that creates custom preset generators
// This is set by the presets package to avoid circular dependencies
var CustomPresetGeneratorFactory func(Preset) PresetGenerator

// GeneratePresets generates all configured presets for a config
func GeneratePresets(cfg *Config) (map[string][]OutputFile, error) {
	if cfg.Content == nil {
		return nil, ErrNoContent
	}

	results := make(map[string][]OutputFile)

	for _, preset := range cfg.Presets {
		var outputs []OutputFile
		var err error

		if preset.IsBuiltIn() {
			generator, genErr := GetPresetGenerator(preset.BuiltIn)
			if genErr != nil {
				return nil, genErr
			}
			outputs, err = generator.Generate(cfg.Content, cfg.BaseDir, cfg)
		} else {
			// Handle custom preset
			if CustomPresetGeneratorFactory == nil {
				return nil, fmt.Errorf("custom preset generator factory not initialized")
			}
			generator := CustomPresetGeneratorFactory(preset)
			outputs, err = generator.Generate(cfg.Content, cfg.BaseDir, cfg)
		}

		if err != nil {
			return nil, fmt.Errorf("generate preset %s: %w", preset.GetName(), err)
		}

		results[preset.GetName()] = outputs
	}

	return results, nil
}

// sanitizeName removes special characters from names for use in filenames
func sanitizeName(name string) string {
	// Replace spaces and special chars with dashes
	replacer := strings.NewReplacer(
		" ", "-",
		"_", "-",
		"/", "-",
		"\\", "-",
	)
	sanitized := replacer.Replace(name)
	// Remove any remaining non-alphanumeric chars except dashes
	var builder strings.Builder
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "-")
}

// extractSkillID extracts the skill ID from a skill's path
func extractSkillID(skillPath string) string {
	// Path format: .../skills/{skill-id}/SKILL.md
	dir := filepath.Dir(skillPath)
	return filepath.Base(dir)
}

// combineContent combines multiple ContentFile slices
func combineContent(slices ...[]ContentFile) []ContentFile {
	var total int
	for _, slice := range slices {
		total += len(slice)
	}

	result := make([]ContentFile, 0, total)
	for _, slice := range slices {
		result = append(result, slice...)
	}
	return result
}
