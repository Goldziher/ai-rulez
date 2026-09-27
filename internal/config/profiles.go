package config

import "strings"

// ProfileSeparator separates the elements of a composed profile value, as in
// "base,backend".
const ProfileSeparator = ","

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
