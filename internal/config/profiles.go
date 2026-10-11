package config

import (
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
)

// ProfileSeparator separates the elements of a composed profile value, as in
// "base,backend".
const ProfileSeparator = ","

// ProfileDomainWildcard is the profile domain entry that selects every domain the
// config knows. It is only meaningful next to the positive entries a profile
// already has, so it can be narrowed with ProfileDomainNegation exclusions.
const ProfileDomainWildcard = "*"

// ProfileDomainNegation marks a profile domain entry as an exclusion: "!name" removes
// name from the domains the profile selects.
const ProfileDomainNegation = "!"

// isNegatedDomain reports whether a profile domain entry excludes a domain. A
// lone prefix with no name is not an exclusion and is ignored during resolution.
func isNegatedDomain(entry string) bool {
	return strings.HasPrefix(entry, ProfileDomainNegation) && entry != ProfileDomainNegation
}

// negatedDomainName returns the entry with its exclusion prefix removed; a
// positive entry is returned unchanged.
func negatedDomainName(entry string) string {
	return strings.TrimPrefix(entry, ProfileDomainNegation)
}

// hasPositiveDomainEntry reports whether a profile's domain list selects
// anything: at least one entry that is not an exclusion. A list made only of
// "!name" entries is invalid, because there is nothing to subtract from.
func hasPositiveDomainEntry(entries []string) bool {
	for _, entry := range entries {
		if entry != "" && !isNegatedDomain(entry) {
			return true
		}
	}
	return false
}

// ResolveProfileDomains resolves a profile's domain entries against the domains
// a config knows. Positive entries are kept in the order written, "*" expands to
// every known domain (sorted, so the result is deterministic) and "!name"
// entries are removed from the result. Exact names behave as before: an unknown
// one is kept as written, so validation can warn about it and selection simply
// ignores it.
//
// A list with exclusions but no positive entry is an error: there is nothing to
// subtract from, so the author almost certainly meant to write "*" first.
func ResolveProfileDomains(entries []string, knownDomains []string) ([]string, error) {
	var positives, negations []string
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		if isNegatedDomain(entry) {
			negations = append(negations, negatedDomainName(entry))
			continue
		}
		positives = append(positives, entry)
	}
	if !hasPositiveDomainEntry(entries) && len(negations) > 0 {
		return nil, oops.
			With("domains", entries).
			Hint(`A profile made only of exclusions has nothing to subtract from; start it with "*" to select every domain, then exclude the ones you do not want`).
			Errorf("profile excludes domains but selects none")
	}
	if len(positives) == 0 {
		return nil, nil
	}

	result := make([]string, 0, len(positives)+len(knownDomains))
	seen := make(map[string]bool, len(positives)+len(knownDomains))
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		result = append(result, name)
	}

	sortedKnown := append([]string(nil), knownDomains...)
	sort.Strings(sortedKnown)
	for _, entry := range positives {
		if entry == ProfileDomainWildcard {
			for _, name := range sortedKnown {
				add(name)
			}
			continue
		}
		add(entry)
	}

	if len(negations) == 0 {
		return result, nil
	}
	excluded := make(map[string]bool, len(negations))
	for _, name := range negations {
		excluded[builtins.TrimRefPrefix(name)] = true
	}
	out := result[:0]
	for _, name := range result {
		if excluded[builtins.TrimRefPrefix(name)] {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// knownDomainNames returns the domain names a content tree knows, sorted so that
// a wildcard expansion is deterministic. A nil tree knows no domains.
func knownDomainNames(content *ContentTree) []string {
	if content == nil {
		return nil
	}
	names := make([]string, 0, len(content.Domains))
	for name := range content.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SplitProfileNames splits a profile value into its element names.
//
// A composed value selects the union of several profiles' domains, which is what
// lets one profile hold the content everybody gets while the others add role
// specifics — without a combinatorial profile per role pair.
//
// Whitespace around an element is trimmed and empty elements are ignored, so
// "base, backend" and "base,backend," both resolve to the same two names. A value
// made up entirely of separators yields no names at all; callers treat that as a
// profile that does not exist rather than silently falling back, so a mistyped
// value is reported instead of quietly generating something else.
//
// Elements are profile names, never domain names, and a profile value may not
// reference another profile: composition is one level deep by construction, which
// is why no cycle detection is needed anywhere.
func SplitProfileNames(value string) []string {
	if !strings.Contains(value, ProfileSeparator) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}

	parts := strings.Split(value, ProfileSeparator)
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

// CanonicalProfile normalizes a profile value for display and lookup: whitespace
// and empty elements are dropped, element order is preserved. A value with no
// usable elements is returned unchanged so an error message can quote what the
// user actually typed.
func CanonicalProfile(value string) string {
	names := SplitProfileNames(value)
	if len(names) == 0 {
		return value
	}
	return strings.Join(names, ProfileSeparator)
}

// ProfileBuiltinRefs returns every builtin named by a "builtin:<name>" element in
// any profile's domain list, de-duplicated and in deterministic order. These packs
// are loaded scoped to the profiles that name them rather than globally.
func (c *Config) ProfileBuiltinRefs() []string {
	profileNames := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)

	seen := make(map[string]bool)
	var refs []string
	for _, profile := range profileNames {
		for _, domain := range c.Profiles[profile] {
			if !builtins.HasRefPrefix(domain) {
				continue
			}
			name := builtins.TrimRefPrefix(domain)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			refs = append(refs, name)
		}
	}
	return refs
}

// ProfileMatches reports whether an item scoped to the named profiles is active
// for a (possibly composed) profile value. An empty scoped list means every
// profile. A value with no usable elements is treated as the built-in default,
// which includes everything.
func ProfileMatches(profile string, scoped []string) bool {
	if len(scoped) == 0 {
		return true
	}
	elements := SplitProfileNames(profile)
	if len(elements) == 0 {
		return true
	}
	for _, element := range elements {
		for _, restricted := range scoped {
			if element == restricted {
				return true
			}
		}
	}
	return false
}

// DomainActive reports whether a resource tagged with domain is active in a
// content tree. An empty domain is global (always active). A non-empty domain is
// active only when the (already-selected) tree carries it; a nil tree therefore
// activates nothing but untagged resources. It is the shared predicate behind
// every domain-tagged resource kind, so an MCP server, an installed skill and a
// skill source all resolve their domain the same way.
func DomainActive(domain string, content *ContentTree) bool {
	if domain == "" {
		return true
	}
	return content != nil && content.Domains[domain] != nil
}

// FilterContentFilesByProfile keeps the files active for a profile: those with
// no profile restriction, plus those whose Profiles include one of the profile
// value's elements.
func FilterContentFilesByProfile(files []ContentFile, profile string) []ContentFile {
	hasScoped := false
	for i := range files {
		if len(files[i].Profiles) > 0 {
			hasScoped = true
			break
		}
	}
	if !hasScoped {
		return files
	}
	out := make([]ContentFile, 0, len(files))
	for i := range files {
		if ProfileMatches(profile, files[i].Profiles) {
			out = append(out, files[i])
		}
	}
	return out
}

// UnknownProfileNames returns the elements of a profile value that are not
// defined in the config, in the order they were written. Used to name the bad
// element in an error rather than echoing the whole composed value, which leaves
// the reader to work out which part was wrong.
func (c *Config) UnknownProfileNames(profile string) []string {
	names := SplitProfileNames(profile)
	if len(names) == 0 {
		return []string{profile}
	}
	var unknown []string
	for _, name := range names {
		if _, ok := c.Profiles[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	return unknown
}
