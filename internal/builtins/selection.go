package builtins

import (
	"sort"
	"strings"
)

// Selection is the resolved form of a builtins list: which packs load whole and
// which load only a chosen set of their items.
type Selection struct {
	// Domains is every pack that contributes content, sorted. It includes packs
	// that load only through item selections (see Only).
	Domains []string
	// Only maps a pack to the item names ("domain/name" selections) it is limited
	// to. A pack absent from Only loads whole.
	Only map[string]map[string]bool
}

// ResolveSelection applies the builtins list to the auto-includes.
//
// Rules, in order:
//   - a bare "domain" loads the whole pack; auto-includes load unless excluded;
//   - "!domain" removes the pack, auto-include or explicit;
//   - a positive "domain/name" loads only that item of a pack that is not
//     otherwise loaded whole, so it still applies after "!domain";
//   - "domain" wins over "domain/name" for the same pack;
//   - "!domain/name" always removes the item (see ExcludedRules), whatever
//     selected it.
//
// Positive entries that name nothing real are not loaded; UnknownSelections
// reports them.
func ResolveSelection(list []string) Selection {
	excluded := make(map[string]bool)
	whole := make(map[string]bool)
	items := make(map[string]map[string]bool)

	for _, entry := range list {
		if spec, ok := strings.CutPrefix(entry, "!"); ok {
			// "!domain/name" is a per-item exclusion handled by ExcludedRules.
			if !strings.Contains(spec, "/") {
				excluded[spec] = true
			}
			continue
		}
		domain, name, qualified := strings.Cut(entry, "/")
		if !qualified {
			whole[entry] = true
			continue
		}
		if _, ok := registry[domain]; !ok {
			continue
		}
		if items[domain] == nil {
			items[domain] = make(map[string]bool)
		}
		items[domain][name] = true
	}

	for name := range autoIncludeDomains {
		whole[name] = true
	}

	sel := Selection{Only: make(map[string]map[string]bool)}
	for name := range whole {
		if !excluded[name] {
			sel.Domains = append(sel.Domains, name)
		}
	}
	for domain, names := range items {
		if whole[domain] && !excluded[domain] {
			continue
		}
		sel.Only[domain] = names
		sel.Domains = append(sel.Domains, domain)
	}
	sort.Strings(sel.Domains)
	return sel
}

// UnknownSelection is a positive builtins entry that names nothing real.
type UnknownSelection struct {
	// Spec is the entry as written.
	Spec string
	// Suggestion is the closest real identifier, or "" when none is close.
	Suggestion string
	// Valid lists the identifiers the entry could have named: the pack's items
	// when the pack exists, every pack otherwise.
	Valid []string
}

// UnknownSelections reports the positive entries of a builtins list ("domain" or
// "domain/name") that do not resolve against the registry and its embedded
// content. Unlike an unknown exclusion, which only leaves content in place, an
// unresolved selection silently drops content the author asked for, so callers
// treat it as an error.
func UnknownSelections(list []string) []UnknownSelection {
	var unknown []UnknownSelection
	seen := make(map[string]bool, len(list))
	for _, entry := range list {
		if entry == "" || strings.HasPrefix(entry, "!") || seen[entry] || exclusionResolves(entry) {
			continue
		}
		seen[entry] = true
		domain, _, qualified := strings.Cut(entry, "/")
		valid := ResolveAll()
		if _, ok := registry[domain]; ok && qualified {
			valid = qualifiedContentNames(domain)
		}
		unknown = append(unknown, UnknownSelection{
			Spec:       entry,
			Suggestion: nearestExclusion(entry),
			Valid:      valid,
		})
	}
	return unknown
}

// ResolvesRef reports whether a "domain" or "domain/name" reference, with or
// without a "builtin:" prefix, names a real pack or pack item.
func ResolvesRef(ref string) bool {
	return exclusionResolves(TrimRefPrefix(ref))
}

// RefDomain returns the pack a "builtin:domain" or "builtin:domain/name"
// reference addresses.
func RefDomain(ref string) string {
	domain, _, _ := strings.Cut(TrimRefPrefix(ref), "/")
	return domain
}
