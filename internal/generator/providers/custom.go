package providers

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

// init wires the provider DSL into the config package's custom-preset hooks.
// It lives here (rather than in config) because providers imports config, not
// the other way around.
func init() {
	config.ProviderSpecGeneratorFactory = func(preset config.Preset, baseDir string) (config.PresetGenerator, error) {
		gen, err := loadCustomProvider(preset, baseDir)
		if err != nil {
			return nil, err
		}
		return gen, nil
	}

	config.ProviderSpecValidator = func(preset config.Preset, baseDir string) error {
		_, err := loadCustomProvider(preset, baseDir)
		return err
	}
}

// loadCustomProvider resolves, reads, and validates a preset's provider spec,
// enforcing that the spec's declared name matches the preset name so target
// filtering and output attribution agree.
func loadCustomProvider(preset config.Preset, baseDir string) (*Generator, error) {
	resolved, err := resolveProviderPath(preset.Provider, baseDir)
	if err != nil {
		return nil, oops.With("preset_name", preset.Name).Wrapf(err, "resolve provider spec")
	}
	gen, err := LoadProviderFile(resolved)
	if err != nil {
		return nil, oops.With("preset_name", preset.Name).Wrapf(err, "load provider spec")
	}
	if preset.Name != gen.Spec.Name {
		return nil, oops.
			With("preset_name", preset.Name).
			With("provider_name", gen.Spec.Name).
			With("path", resolved).
			Hint("Rename the preset or the spec so both share the same name").
			Errorf("provider spec name %q does not match preset name %q", gen.Spec.Name, preset.Name)
	}
	return gen, nil
}

// LoadProviderFile reads and validates a provider spec from disk, detecting the
// format (TOML/YAML/JSON) from the file extension.
func LoadProviderFile(filePath string) (*Generator, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, oops.With("path", filePath).Wrapf(err, "read provider spec")
	}
	spec, err := LoadProviderSpec(raw, filePath, FormatAuto)
	if err != nil {
		return nil, err
	}
	return New(spec), nil
}

// resolveProviderPath resolves a project-relative spec path and rejects paths
// that escape the project root (absolute paths or ".." traversal).
func resolveProviderPath(rel, baseDir string) (string, error) {
	if rel == "" {
		return "", oops.Hint("Set 'provider' to a project-relative spec path").Errorf("empty provider spec path")
	}
	normalized := strings.ReplaceAll(rel, `\`, "/")
	cleaned := path.Clean(normalized)
	if path.IsAbs(normalized) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", oops.
			With("value", rel).
			Hint("Use a project-relative path that does not contain '..'").
			Errorf("provider spec path escapes the project root")
	}
	return filepath.Join(baseDir, filepath.FromSlash(cleaned)), nil
}
