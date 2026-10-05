package mcp

import (
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// configHolder gives the role resolver the configuration of the latest build, so
// a live reload that edits [[roles]] takes effect without restarting the server.
type configHolder struct {
	mu  sync.RWMutex
	cfg *config.Config
}

func (h *configHolder) set(cfg *config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg = cfg
}

func (h *configHolder) get() *config.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// RolesFromConfig resolves a role name against the project's [[roles]]: the
// skills the role keeps (its domains, include and exclude selectors, extends
// merged in) and the delivery it gives each skill. Skills that do not come from
// the project's content tree, such as those of a skill source, are matched by
// name against the role's skill selectors. An unknown role, or one whose
// inheritance is broken, is not resolved.
func RolesFromConfig(get func() *config.Config) RoleResolver {
	return func(role string) (RoleScope, bool) {
		cfg := get()
		if cfg == nil {
			return RoleScope{}, false
		}
		resolved, err := cfg.ResolveRole(role)
		if err != nil {
			return RoleScope{}, false
		}
		flat := resolved.Flat()
		known := map[string]bool{}
		for _, it := range cfg.AllItems() {
			if it.Kind == config.RoleKindSkill {
				known[it.Domain+"/"+it.ID] = true
			}
		}
		kept := map[string]bool{}
		for _, it := range resolved.ItemsOf(config.RoleKindSkill) {
			kept[it.Domain+"/"+it.ID] = true
		}
		return RoleScope{
			Keep: func(domain, name string) bool {
				if key := domain + "/" + name; known[key] {
					return kept[key]
				}
				return flat.Keeps(config.RoleKindSkill, "", name)
			},
			Delivery: resolved.DeliveryOverride,
		}, true
	}
}
