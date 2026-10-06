package sbom

import (
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// scope narrows the document to a profile and/or a role, the way generate does.
// A profile keeps the root content and the domains it names; a role keeps the
// items it selects (and the context files, which no role selects away). Remote
// sources and MCP servers are not part of a role (roles select content), so a
// role lists them all; a profile drops the servers and installed skills that are
// scoped to other profiles.
type scope struct {
	profile string
	// domains are the local domains a profile keeps; nil keeps every domain.
	domains map[string]bool
	role    string
	// roleItems are the kept items of a role, by key; nil keeps all.
	roleItems map[string]bool
	hasRole   bool
}

func newScope(cfg *config.Config, o Options) (*scope, error) {
	s := &scope{profile: o.Profile, role: o.Role}
	if o.Profile != "" {
		if err := s.selectProfile(cfg); err != nil {
			return nil, err
		}
	}
	if o.Role != "" {
		res, err := cfg.ResolveRole(o.Role)
		if err != nil {
			return nil, oops.Hint("`ai-rulez roles list` shows the defined roles").Wrap(err)
		}
		s.hasRole = true
		s.roleItems = map[string]bool{}
		for i := range res.Items {
			it := &res.Items[i]
			s.roleItems[itemKey(it.Kind, it.Domain, it.ID)] = true
		}
	}
	return s, nil
}

func (s *scope) selectProfile(cfg *config.Config) error {
	names := config.SplitProfileNames(s.profile)
	if len(names) == 1 && names[0] == "default" && !cfg.HasProfile("default") {
		// The built-in default: every domain when no profile is defined, else the
		// root content only (builtins and included domains are not local items).
		if len(cfg.Profiles) == 0 {
			return nil
		}
		s.domains = map[string]bool{}
		return nil
	}
	if !cfg.HasProfile(s.profile) {
		return oops.Hint("`ai-rulez profile list` shows the defined profiles").Errorf("profile %q is not defined", s.profile)
	}
	s.domains = map[string]bool{}
	for _, ref := range cfg.GetProfileDomains(s.profile) {
		s.domains[builtins.TrimRefPrefix(ref)] = true
	}
	return nil
}

func itemKey(kind, domain, id string) string { return kind + "\x00" + domain + "\x00" + baseID(id) }

// baseID drops the "#N" suffix the lock gives the second item of a key.
func baseID(id string) string {
	i := strings.LastIndexByte(id, '#')
	if i < 0 || i == len(id)-1 {
		return id
	}
	for _, r := range id[i+1:] {
		if r < '0' || r > '9' {
			return id
		}
	}
	return id[:i]
}

// keepItem reports whether the item is part of the slice.
func (s *scope) keepItem(it *lockfile.Item) bool {
	if s.domains != nil && it.Domain != "" && !s.domains[it.Domain] {
		return false
	}
	if !s.hasRole {
		return true
	}
	switch it.Kind {
	case config.RoleKindRule, config.RoleKindSkill, config.RoleKindAgent, config.RoleKindCommand, config.RoleKindCheck:
		return s.roleItems[itemKey(it.Kind, it.Domain, it.ID)]
	case "role":
		return it.ID == s.role
	}
	return true
}

// keepScoped reports whether something scoped to the given profiles (an MCP
// server, an installed skill) is part of the slice.
func (s *scope) keepScoped(profiles []string) bool {
	return s.profile == "" || config.ProfileMatches(s.profile, profiles)
}

// filter keeps the items that are part of the slice.
func (s *scope) filter(items []lockfile.Item) []lockfile.Item {
	var kept []lockfile.Item
	for i := range items {
		if s.keepItem(&items[i]) {
			kept = append(kept, items[i])
		}
	}
	return kept
}
