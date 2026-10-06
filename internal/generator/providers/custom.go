package providers

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// Register adds every embedded provider spec to r under its declared name, and the
// factory provider-backed custom presets are built with. It replaces the init()
// that used to do this, so the registry a caller gets is the one it built. An
// embedded spec that does not parse is a build-time bug, so it panics.
func Register(r *config.Registry) {
	names, err := BuiltinNames()
	if err != nil {
		panic("providers: enumerate builtin specs: " + err.Error())
	}
	for _, name := range names {
		gen, err := LoadBuiltin(name)
		if err != nil {
			panic("providers: load builtin " + name + ": " + err.Error())
		}
		r.Register(name, gen)
	}
	r.Provider = func(preset config.Preset, baseDir string) (config.PresetGenerator, error) {
		gen, err := loadCustomProvider(preset, baseDir)
		if err != nil {
			return nil, err
		}
		return gen, nil
	}
}

// SplitRulesDir implements config.RulesDirOwner: the folder of a split rules
// output, which is shared with hand-written rule files and so gets the protections
// the built-in rules folders have.
func (g *Generator) SplitRulesDir() string {
	if out := g.Spec.Outputs[OutputTypeRules]; out != nil && out.Split {
		return out.Dir
	}
	return ""
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
