package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PresetGeneratorV3 defines the interface for V3 preset generators
type PresetGeneratorV3 interface {
	Generate(content *ContentTreeV3, baseDir string, config *ConfigV3) ([]OutputFileV3, error)
	GetOutputPaths(baseDir string) []string
	GetName() string
}

// OutputFileV3 represents a generated output file or directory
type OutputFileV3 struct {
	Path    string
	Content string
	IsDir   bool
}

// PresetRegistryV3 maps preset names to their generators
// Populated by init() functions in generator/presets/ package
var PresetRegistryV3 = make(map[string]PresetGeneratorV3)

// RegisterPresetV3 registers a preset generator
func RegisterPresetV3(name string, generator PresetGeneratorV3) {
	PresetRegistryV3[name] = generator
}

// GetPresetGeneratorV3 retrieves a preset generator by name
func GetPresetGeneratorV3(name string) (PresetGeneratorV3, error) {
	generator, exists := PresetRegistryV3[name]
	if !exists {
		return nil, ErrInvalidPresetV3
	}
	return generator, nil
}

// CustomPresetGeneratorFactory is a function type that creates custom preset generators
// This is set by the presets package to avoid circular dependencies
var CustomPresetGeneratorFactory func(PresetV3) PresetGeneratorV3

// GeneratePresetsV3 generates all configured presets for a V3 config
func GeneratePresetsV3(cfg *ConfigV3) (map[string][]OutputFileV3, error) {
	if cfg.Content == nil {
		return nil, ErrNoContent
	}

	results := make(map[string][]OutputFileV3)

	for _, preset := range cfg.Presets {
		var outputs []OutputFileV3
		var err error

		if preset.IsBuiltIn() {
			generator, genErr := GetPresetGeneratorV3(preset.BuiltIn)
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
