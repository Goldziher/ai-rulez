package providers

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

// MappedRulesFolder is the rules folder of a configured provider whose
// frontmatter is declared by an activation map.
type MappedRulesFolder struct {
	Dir     string // base-relative, slash-separated, no trailing slash
	Mapping *rulefiles.ActivationMap
}

// MappedRulesFolders lists the rules folders of the configured presets (builtin
// and custom provider specs) that write frontmatter from an activation map, so a
// reader of generated rule files can tell how each one is activated without a
// dialect of its own. A spec that fails to load is skipped.
func MappedRulesFolders(cfg *config.Config) []MappedRulesFolder {
	if cfg == nil {
		return nil
	}
	var folders []MappedRulesFolder
	for _, preset := range cfg.Presets {
		var gen *Generator
		var err error
		switch {
		case preset.IsBuiltIn():
			gen, err = LoadBuiltin(preset.BuiltIn)
		case preset.Provider != "":
			gen, err = loadCustomProvider(preset, cfg.BaseDir)
		default:
			continue
		}
		if err != nil || gen == nil {
			continue
		}
		spec := gen.Spec.Outputs[OutputTypeRules]
		if spec == nil || spec.Activation == nil || spec.Dir == "" {
			continue
		}
		folders = append(folders, MappedRulesFolder{
			Dir:     filepath.ToSlash(spec.Dir),
			Mapping: activationMap(spec.Activation),
		})
	}
	return folders
}
