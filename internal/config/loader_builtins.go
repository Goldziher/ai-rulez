package config

import (
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

	if config.Builtins.IsEnabled() && !config.Builtins.IsNone() {
		warnUnknownBuiltinExclusions(config.Log(), config.Builtins.GetNames())

		var resolved []string
		if config.Builtins.IsAll() {
			resolved = builtins.ResolveAll()
		} else {
			resolved = builtins.ResolveBuiltins(config.Builtins.GetNames())
		}
		if len(resolved) > 0 {
			config.Log().Debug("Loading builtins", "count", len(resolved), "names", resolved)
			loadBuiltinDomains(config, resolved, builtins.ExcludedRules(config.Builtins.GetNames()), false)
		}
	}

	for _, name := range config.ProfileBuiltinRefs() {
		if _, exists := config.Content.Domains[name]; exists {
			continue
		}
		loadBuiltinDomains(config, []string{name}, nil, true)
	}
}

// loadBuiltinDomains loads each named builtin pack into config.Content.Domains,
// skipping names a local or include domain already owns. When scoped is true the
// domain is tagged BuiltinScoped so profile resolution confines it to the
// profiles that reference it.
func loadBuiltinDomains(config *Config, names []string, ruleExclusions map[string]bool, scoped bool) {
	for _, name := range names {
		// Skip if domain already exists (local content has higher priority)
		if _, exists := config.Content.Domains[name]; exists {
			config.Log().Debug("Skipping builtin (local domain exists)", "name", name)
			continue
		}

		entries, err := builtins.LoadDomainContent(name)
		if err != nil {
			config.Warn("Failed to load builtin", "name", name, "error", err)
			continue
		}
		if len(entries) == 0 {
			continue
		}

		domain := &Domain{
			Name:          name,
			Builtin:       true,
			BuiltinScoped: scoped,
		}

		for _, entry := range entries {
			// Skip individual builtin content files excluded via "!domain/name".
			if ruleExclusions[name+"/"+entry.Name] {
				config.Log().Debug("Excluding builtin content", "domain", name, "name", entry.Name)
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
