package providers

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/hookplugins"
)

// validateHookPluginSidecar checks a `hook_plugin` sidecar: it needs a flavor, and
// the flavor is the only option it takes. The module is owned wholly by ai-rulez,
// so it has no document format, key or dialect. Other kinds take no flavor.
func validateHookPluginSidecar(i int, sc *SidecarSpec) error {
	if sc.Kind != SidecarHookPlugin {
		if sc.Flavor != "" {
			return fmt.Errorf("sidecars[%d].flavor is only valid on kind %q", i, SidecarHookPlugin)
		}
		return nil
	}
	if sc.Flavor == "" {
		return fmt.Errorf("sidecars[%d].flavor is required for kind %q", i, SidecarHookPlugin)
	}
	if !hookplugins.IsFlavor(sc.Flavor) {
		return fmt.Errorf("sidecars[%d].flavor: unknown flavor %q (want opencode, opencode-v1, pi or amp)", i, sc.Flavor)
	}
	return nil
}

// renderHookPlugin renders the plugin module of a hook_plugin sidecar. An empty body
// means no [[hooks]] group applies to the harness, so no file is written.
// A module the consumer wrote under the same name is theirs: it is left alone, with
// a warning, and the render is empty.
func (g *Generator) renderHookPlugin(sc *SidecarSpec, cfg *config.Config, outputPath string) (sidecarRender, error) {
	body, ok, err := hookplugins.Render(cfg, g.Spec.Name, hookplugins.Flavor(sc.Flavor))
	if err != nil || !ok || !hookplugins.MayWriteModule(outputPath) {
		return sidecarRender{}, err
	}
	return sidecarRender{Body: body}, nil
}
