package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PresetGenerator defines the interface for preset generators
type PresetGenerator interface {
	Generate(content *ContentTree, baseDir string, config *Config) ([]OutputFile, error)
	GetOutputPaths(baseDir string) []string
	GetName() string
}

// OutputFile represents a generated output file or directory.
//
// When RawContent is non-nil, the file is written verbatim — no header
// injection, no hash bookkeeping, no trailing-newline normalization. Use this
// for skill supporting files (references, scripts, assets) where any
// ai-rulez-generated marker would corrupt the payload (e.g. Python scripts,
// binary assets).
//
// When RawContent is nil, Content is rendered through the standard
// generation pipeline (header banner + content/source hashes).
//
// Mode is the file permission bits to apply on write (0o644 if zero). Used
// to preserve the executable bit on bundled skill scripts so the agent can
// invoke them directly. Only honored on the raw-content write path.
//
// LocalOnly marks an output rendered from machine-local override content
// (.ai-rulez/local/ → CLAUDE.local.md, AGENTS.local.md, ...). Such outputs are
// gitignored unconditionally — even when config gitignore is disabled — because
// committing them would leak machine-local configuration.
type OutputFile struct {
	Path       string
	Content    string
	RawContent []byte
	Mode       os.FileMode
	IsDir      bool
	LocalOnly  bool
}

// LocalRootProvider is implemented by preset generators that emit a single
// markdown root instructions file and therefore support a machine-local ".local"
// variant of it (CLAUDE.md → CLAUDE.local.md). LocalRootFile returns the local
// variant's path relative to the output base dir, or "" when the preset has no
// single-file markdown root (e.g. cursor, windsurf) and so has no local variant.
type LocalRootProvider interface {
	LocalRootFile() string
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
