package preflight

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/hookplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
)

// The "where" of a hook or an allow rule is what the translators produce, not a
// static list of harnesses: a harness the rule cannot be expressed for, a user-only
// harness in a project run and a harness whose hooks are code (a plugin module)
// are all decided by rendering the one item for it and asking whether anything
// came out. The renders run with warnings silenced; generate issues them itself.

// hookLands lists the presets the single action hooks[group].Hooks[action] is
// written for.
func hookLands(cfg *config.Config, presets []string, group, action int) []string {
	saved := cfg.Hooks
	defer func() { cfg.Hooks = saved }()
	probe := saved[group]
	probe.Hooks = saved[group].Hooks[action : action+1]
	cfg.Hooks = []config.HookGroup{probe}
	defer silenced(cfg)()

	var where []string
	for _, p := range presets {
		if hookWritten(cfg, p) {
			where = append(where, p)
		}
	}
	return where
}

func hookWritten(cfg *config.Config, harness string) bool {
	if !cfg.Hooks[0].HookTargetsHarness(harness) {
		return false
	}
	if flavor, ok := hookplugins.FlavorFor(harness); ok {
		_, written, err := hookplugins.Render(cfg, harness, flavor)
		return err == nil && written
	}
	switch {
	case harness == config.HarnessCline:
		return len(settings.ClineHookScripts(cfg)) > 0
	case !settings.HasHookDialect(harness):
		return false
	case settings.HookDialectOwnsFile(harness):
		_, written, err := settings.OwnedHooksKeys(cfg, harness)
		return err == nil && written
	}
	keys, err := settings.HookKeys(cfg, harness, "")
	return err == nil && len(keys) > 0
}

// allowLands lists the presets the allow rule is written for.
func allowLands(cfg *config.Config, presets []string, rule string) []string {
	saved := cfg.Permissions
	defer func() { cfg.Permissions = saved }()
	cfg.Permissions = &config.Permissions{Allow: []string{rule}}
	defer silenced(cfg)()

	var where []string
	for _, p := range presets {
		if allowWritten(cfg, p) {
			where = append(where, p)
		}
	}
	return where
}

func allowWritten(cfg *config.Config, harness string) bool {
	switch {
	case harness == config.HarnessClaude:
		return true
	case harness == config.HarnessCodex:
		_, written := settings.CodexRules(cfg)
		return written
	case !settings.IsPermissionDialect(harness):
		return false
	case settings.PermissionsUserOnly(harness) && !cfg.UserScope:
		return false
	}
	keys, err := settings.PermissionKeys(cfg, harness, "")
	return err == nil && len(keys) > 0
}

// silenced makes the probe renders of a landing check say nothing and returns the
// function that puts cfg's collector back. A fresh collector takes the warnings, so
// the one of the real run neither shows them nor counts them as issued.
func silenced(cfg *config.Config) (restore func()) {
	prev := cfg.Diag
	cfg.Diag = diag.New(func(string, ...any) {})
	return func() { cfg.Diag = prev }
}
