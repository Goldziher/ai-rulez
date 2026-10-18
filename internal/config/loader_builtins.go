package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// warnUnknownBuiltinExclusions logs a warning for every "!name" entry in the
// builtins list that excludes something that does not exist. Such an entry is
// otherwise inert: resolution simply never matches it, so the builtin the author
// meant to drop stays in every generated file with nothing reporting it.
//
// Deliberately a warning, not an error: builtins are renamed and retired upstream,
// and a project excluding a rule that has since moved into a skill must keep
// generating. This runs during config load, so `validate` and `generate` — and
// every other command that loads a config — report it alike.
func warnUnknownBuiltinExclusions(log logger.Logger, names []string) {
	for _, unknown := range builtins.UnknownExclusions(names) {
		args := []any{"exclusion", "!" + unknown.Spec}
		if unknown.Suggestion != "" {
			args = append(args, "did_you_mean", "!"+unknown.Suggestion)
		}
		log.Warn("Unknown builtin exclusion ignored; nothing was suppressed", args...)
	}
}

// itemSelected reports whether a pack item loads: "!domain/name" always removes it,
// and a pack limited to chosen items loads only those.
func itemSelected(domain, name string, only map[string]map[string]bool, excluded map[string]bool) bool {
	if excluded[domain+"/"+name] {
		return false
	}
	allowed := only[domain]
	return allowed == nil || allowed[name]
}

// itemCount is the number of content files a builtin domain holds.
func (d *Domain) itemCount() int {
	return len(d.Rules) + len(d.Context) + len(d.Skills) + len(d.Agents) + len(d.Commands)
}

// validateBuiltins rejects positive entries of the builtins list ("domain" or
// "domain/name") that name nothing real. Unlike an unknown exclusion, which only
// leaves content in place, an unresolved selection silently drops content the
// author asked for, so it is an error that names the valid choices.
func (c *Config) validateBuiltins() error {
	if c.Builtins == nil || !c.Builtins.IsEnabled() || c.Builtins.IsAll() || c.Builtins.IsNone() {
		return nil
	}
	unknown := builtins.UnknownSelections(c.Builtins.GetNames())
	if len(unknown) == 0 {
		return nil
	}
	var problems []string
	for _, u := range unknown {
		msg := fmt.Sprintf("%q (valid: %s)", u.Spec, strings.Join(u.Valid, ", "))
		if u.Suggestion != "" {
			msg = fmt.Sprintf("%q, did you mean %q? (valid: %s)", u.Spec, u.Suggestion, strings.Join(u.Valid, ", "))
		}
		problems = append(problems, msg)
	}
	return oops.
		With("field", "builtins").
		Hint("Use a pack from `ai-rulez builtins list` or an item as `<pack>/<item>` (`ai-rulez builtins show <pack>` lists items)").
		Errorf("builtins names unknown packs or items: %s", strings.Join(problems, "; "))
}

// loadBuiltins resolves and loads builtin domains into the config content tree.
// Builtins have the lowest priority: they are injected into domains that don't already exist.
// If a domain already exists (from local content or includes), the builtin is skipped.
//
// Two sources are loaded. The root `builtins` field is global: it governs the pack
// set visible to every profile, including the auto-includes. A "builtin:<name>"
// reference in a profile's domain list is scoped: the pack is loaded only if it is
// not already global, and profile resolution then confines it to the profiles that
// name it. Because a profile reference is an explicit opt-in, it is honored even
// when the root field is absent or set to false.
func loadBuiltins(config *Config) {
	if config.Content == nil {
		config.Content = &ContentTree{
			Domains: make(map[string]*Domain),
		}
	}

	var rootExclusions map[string]bool
	if config.Builtins.IsEnabled() && !config.Builtins.IsNone() {
		rootExclusions = builtins.ExcludedRules(config.Builtins.GetNames())
		warnUnknownBuiltinExclusions(config.Log(), config.Builtins.GetNames())

		selection := builtins.Selection{}
		if config.Builtins.IsAll() {
			selection.Domains = builtins.ResolveAll()
		} else {
			selection = builtins.ResolveSelection(config.Builtins.GetNames())
		}
		if len(selection.Domains) > 0 {
			config.Log().Debug("Loading builtins", "count", len(selection.Domains), "names", selection.Domains)
			loadBuiltinDomains(config, selection.Domains, selection.Only, rootExclusions, false)
		}
	}

	// A global selection (even item by item) already owns its domain, so it wins
	// over a profile reference to the same pack. Root "!domain/name" exclusions
	// apply to profile-scoped packs too.
	whole, only := groupProfileBuiltinRefs(config.ProfileBuiltinRefs())
	for _, name := range whole {
		loadBuiltinDomains(config, []string{name}, nil, rootExclusions, true)
	}
	for name, items := range only {
		loadBuiltinDomains(config, []string{name}, map[string]map[string]bool{name: items}, rootExclusions, true)
	}
}

// groupProfileBuiltinRefs splits "builtin:" references (prefix already removed) into
// packs loaded whole and packs limited to the items named for them. A whole-pack
// reference wins over item references to the same pack.
func groupProfileBuiltinRefs(refs []string) (whole []string, only map[string]map[string]bool) {
	only = make(map[string]map[string]bool)
	for _, ref := range refs {
		domain, item, qualified := strings.Cut(ref, "/")
		if !qualified {
			whole = append(whole, domain)
			continue
		}
		if only[domain] == nil {
			only[domain] = make(map[string]bool)
		}
		only[domain][item] = true
	}
	for _, domain := range whole {
		delete(only, domain)
	}
	return whole, only
}

// loadBuiltinDomains loads each named builtin pack into config.Content.Domains,
// skipping names a local or include domain already owns. A pack with an entry in
// only loads just those items; ruleExclusions ("domain/name") always removes an
// item. When scoped is true the domain is tagged BuiltinScoped so profile
// resolution confines it to the profiles that reference it.
func loadBuiltinDomains(config *Config, names []string, only map[string]map[string]bool, ruleExclusions map[string]bool, scoped bool) {
	for _, name := range names {
		entries, err := builtins.LoadDomainContent(name)
		if err != nil {
			config.Warn("Failed to load builtin", "name", name, "error", err)
			continue
		}

		// Skip if domain already exists (local content has higher priority). An
		// earlier builtin load owning the name is not a loss; a local or include
		// domain is, because the pack's other items are never loaded.
		if existing, exists := config.Content.Domains[name]; exists {
			if !existing.Builtin {
				warnBuiltinPackSkipped(config, name, entries, only, ruleExclusions)
			}
			continue
		}
		if len(entries) == 0 {
			continue
		}

		domain := buildBuiltinDomain(config, name, entries, only, ruleExclusions, scoped)
		if only[name] != nil && domain.itemCount() == 0 {
			continue
		}
		config.Content.Domains[name] = domain
		config.Log().Debug("Loaded builtin domain",
			"name", name,
			"scoped", scoped,
			"rules", len(domain.Rules),
			"context", len(domain.Context),
			"skills", len(domain.Skills),
			"agents", len(domain.Agents),
			"commands", len(domain.Commands),
		)
	}
}

// buildBuiltinDomain assembles the domain of one builtin pack from its embedded
// entries, leaving out the items the selection or an exclusion removes.
func buildBuiltinDomain(config *Config, name string, entries []builtins.ContentEntry, only map[string]map[string]bool, ruleExclusions map[string]bool, scoped bool) *Domain {
	domain := &Domain{
		Name:          name,
		Builtin:       true,
		BuiltinScoped: scoped,
	}
	for _, entry := range entries {
		// Skip individual builtin content files excluded via "!domain/name".
		if !itemSelected(name, entry.Name, only, ruleExclusions) {
			config.Log().Debug("Skipping builtin content", "domain", name, "name", entry.Name)
			continue
		}

		// Parse frontmatter from embedded content
		metadata, body, malformed := parseFrontmatter(entry.Content)
		if malformed {
			warnMalformedFrontmatter(config.Log(), entry.Path)
		}
		cf := ContentFile{
			Name:                 entry.Name,
			Path:                 "builtin://" + entry.Path,
			Content:              body,
			Metadata:             metadata,
			MalformedFrontmatter: malformed,
		}

		switch entry.Type {
		case "rules":
			domain.Rules = append(domain.Rules, cf)
		case "context":
			domain.Context = append(domain.Context, cf)
		case skillsDir:
			domain.Skills = append(domain.Skills, cf)
		case "agents":
			domain.Agents = append(domain.Agents, cf)
		case "commands":
			domain.Commands = append(domain.Commands, cf)
		}
	}
	return domain
}

// warnBuiltinPackSkipped reports, once per pack, that a local or included domain
// of the same name keeps the builtin pack from loading, and names the pack items
// that are lost: those no same-named item of the project shadows anyway (#446).
// Merging the pack into the domain is not done: a domain is either a builtin
// (skipped by the lock, SBOM and policy readers, which cannot open builtin://
// paths) or not, and one output id twice in a domain is a validation error.
func warnBuiltinPackSkipped(config *Config, pack string, entries []builtins.ContentEntry, only map[string]map[string]bool, excluded map[string]bool) {
	owned := projectItemNames(config.Content)
	var lost []string
	for _, entry := range entries {
		if !itemSelected(pack, entry.Name, only, excluded) || owned[entry.Type+"/"+entry.Name] {
			continue
		}
		lost = append(lost, entry.Type+"/"+entry.Name)
	}
	if len(lost) == 0 {
		config.Log().Debug("Skipping builtin (local domain exists)", "name", pack)
		return
	}
	sort.Strings(lost)
	config.WarnOnce("builtin-pack-skipped:"+pack,
		"Builtin pack skipped because a domain with the same name exists; its items are not loaded",
		"pack", pack,
		"lost", strings.Join(lost, ", "),
		"hint", "rename the domain "+pack+", or provide the items yourself, or drop "+pack+" from builtins")
}

// projectItemNames lists the "kind/name" of every item the project's own root and
// non-builtin domains define.
func projectItemNames(content *ContentTree) map[string]bool {
	names := map[string]bool{}
	add := func(kind string, files []ContentFile) {
		for i := range files {
			names[kind+"/"+files[i].Name] = true
		}
	}
	add("rules", content.Rules)
	add("context", content.Context)
	add(skillsDir, content.Skills)
	add("agents", content.Agents)
	add("commands", content.Commands)
	for _, domain := range content.Domains {
		if domain == nil || domain.Builtin {
			continue
		}
		add("rules", domain.Rules)
		add("context", domain.Context)
		add(skillsDir, domain.Skills)
		add("agents", domain.Agents)
		add("commands", domain.Commands)
	}
	return names
}
