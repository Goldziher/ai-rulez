package settings

import (
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// UnsupportedDiagnostics names the configured presets for which the declared
// [[hooks]] and [permissions] cannot be generated, so the omission is reported
// instead of silent. Hooks render for the harnesses in config.HookHarnesses and
// permissions for PermissionHarnesses; every other preset has either no hook or
// allow-list mechanism, or one whose format is not verified against vendor
// documentation. User-level-only permission harnesses are named in a project run.
// The `mcp`, `okf` and `llms-txt` presets are not harnesses and are ignored. The result is sorted.
func UnsupportedDiagnostics(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	var noHooks, noPermissions, userOnly []string
	for _, preset := range cfg.Presets {
		name := preset.GetName()
		if name == string(config.PresetMCP) || name == config.PresetOKF || name == config.PresetLLMsTxt {
			continue
		}
		if !slices.Contains(config.HookHarnesses, name) {
			noHooks = append(noHooks, name)
		}
		if slices.Contains(userOnlyPermissionHarnesses, name) && !cfg.UserScope {
			userOnly = append(userOnly, name)
		}
		if !SupportsPermissions(name) {
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
		out = append(out, "[permissions] are not generated for "+strings.Join(noPermissions, ", ")+
			": ai-rulez translates them for "+strings.Join(PermissionHarnesses(), ", ")+
			" only; the others have no documented permission file (see docs/permissions.md), so their deny rules are NOT enforced")
	}
	if !cfg.Permissions.IsEmpty() && len(userOnly) > 0 {
		sort.Strings(userOnly)
		out = append(out, "[permissions] are generated for "+strings.Join(userOnly, ", ")+
			" with --user only: they read their permission settings from a user-level file, so this project run enforces none of the rules, deny rules included")
	}
	return out
}
