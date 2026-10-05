package settings

import (
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// UnsupportedDiagnostics names the configured presets for which the declared
// [[hooks]] and [permissions] cannot be generated, so the omission is reported
// instead of silent. Hooks render for the harnesses in config.HookHarnesses and
// permissions for claude; every other preset has either no hook or allow-list
// mechanism, or one whose format is not verified against vendor documentation.
// The `mcp` preset is not a harness and is ignored. The result is sorted.
func UnsupportedDiagnostics(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	var noHooks, noPermissions []string
	for _, preset := range cfg.Presets {
		name := preset.GetName()
		if name == string(config.PresetMCP) {
			continue
		}
		if !slices.Contains(config.HookHarnesses, name) {
			noHooks = append(noHooks, name)
		}
		if name != config.HarnessClaude {
			noPermissions = append(noPermissions, name)
		}
	}
	var out []string
	if cfg.HasSettingsHooks() && len(noHooks) > 0 {
		sort.Strings(noHooks)
		out = append(out, "[[hooks]] are not generated for "+strings.Join(noHooks, ", ")+
			": ai-rulez renders hooks for "+strings.Join(config.HookHarnesses, ", ")+" only")
	}
	if !cfg.Permissions.IsEmpty() && len(noPermissions) > 0 {
		sort.Strings(noPermissions)
		out = append(out, "[permissions] are generated for claude only, not for "+strings.Join(noPermissions, ", ")+
			": those harnesses have no allow/ask/deny list in a documented settings file")
	}
	return out
}
