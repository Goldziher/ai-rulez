package generator

import (
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// PresetsMissingStub lists the configured presets whose harness can call MCP and
// that should carry the dynamic-skills stub (because a skill is served) but whose
// rendered output has none: the stub was excluded, replaced by an unrelated
// skill, or the preset renders no skills at all. Nothing is written.
func (g *Generator) PresetsMissingStub(profile string) ([]string, error) {
	generateMu.Lock()
	defer generateMu.Unlock()
	g.beginRun()
	render, err := g.renderPresets(profile)
	if err != nil {
		return nil, oops.Wrapf(err, "render presets")
	}
	served := false
	for _, p := range g.config.SkillDeliveries(render.content, nil) {
		if p.Delivery != config.DeliveryStatic {
			served = true
		}
	}
	if !served {
		return nil, nil
	}
	var missing []string
	for preset, outputs := range render.byPreset {
		if !config.HarnessSupportsMCP(preset) {
			continue
		}
		if !hasStub(outputs) {
			missing = append(missing, preset)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func hasStub(outputs []config.OutputFile) bool {
	for _, out := range outputs {
		if !out.IsDir && filepath.Base(out.Path) == skillEntryFile && filepath.Base(filepath.Dir(out.Path)) == config.DynamicSkillsName {
			return true
		}
	}
	return false
}
