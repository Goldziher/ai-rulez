// Package semver parses semantic versions (https://semver.org, 2.0.0) and
// npm-style version constraints ("^1.2", "~2.1.0", ">=1.4.0 <2.0.0", "1.x || 3").
// It exists because ai-rulez resolves a source's version constraint against the
// repository's git tags and the lock must hold exactly what that resolution
// picked: the grammar and the prerelease rules are small, fixed and tested here
// rather than inherited from a dependency.
package semver

import (
	"strconv"
	"strings"

	"github.com/samber/oops"
)

// Version is a parsed semantic version. Build metadata is kept for display and
// ignored by Compare, as the specification requires.
type Version struct {
	Major, Minor, Patch uint64
	// Pre holds the dot-separated prerelease identifiers ("beta", "2").
	Pre   []string
	Build string
}

// Parse parses a canonical semantic version without a "v" prefix: MAJOR.MINOR.PATCH
// with optional -prerelease and +build parts. Leading zeros in numeric parts are
// refused, as are empty identifiers.
func Parse(s string) (Version, error) {
	v, err := parse(s)
	if err != nil {
		return Version{}, err
	}
	return v, nil
}

func parse(s string) (Version, error) {
	bad := func(reason string) (Version, error) {
		return Version{}, oops.With("version", s).Errorf("%q is not a semantic version: %s", s, reason)
	}
	rest := s
	var build string
	if i := strings.IndexByte(rest, '+'); i >= 0 {
		rest, build = rest[:i], rest[i+1:]
		if !validIdentifiers(build, false) {
			return bad("invalid build metadata")
		}
	}
	var pre string
	hasPre := false
	if i := strings.IndexByte(rest, '-'); i >= 0 {
		rest, pre, hasPre = rest[:i], rest[i+1:], true
		if !validIdentifiers(pre, true) {
			return bad("invalid prerelease")
		}
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return bad("want MAJOR.MINOR.PATCH")
	}
	var nums [3]uint64
	for i, p := range parts {
		n, ok := parseNumeric(p)
		if !ok {
			return bad("invalid " + [...]string{"major", "minor", "patch"}[i] + " number")
		}
		nums[i] = n
	}
	v := Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Build: build}
	if hasPre {
		v.Pre = strings.Split(pre, ".")
	}
	return v, nil
}

// parseNumeric parses a number without leading zeros.
func parseNumeric(s string) (uint64, bool) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

// validIdentifiers checks dot-separated identifiers of [0-9A-Za-z-]. In a
// prerelease a numeric identifier must not have leading zeros.
func validIdentifiers(s string, prerelease bool) bool {
	if s == "" {
		return false
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		numeric := true
		for _, c := range id {
			switch {
			case c >= '0' && c <= '9':
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '-':
				numeric = false
			default:
				return false
			}
		}
		if prerelease && numeric && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}

// String is the canonical form, with the build metadata when there is one.
func (v Version) String() string {
	var b strings.Builder
	b.WriteString(strconv.FormatUint(v.Major, 10))
	b.WriteByte('.')
	b.WriteString(strconv.FormatUint(v.Minor, 10))
	b.WriteByte('.')
	b.WriteString(strconv.FormatUint(v.Patch, 10))
	if len(v.Pre) > 0 {
		b.WriteByte('-')
		b.WriteString(strings.Join(v.Pre, "."))
	}
	if v.Build != "" {
		b.WriteByte('+')
		b.WriteString(v.Build)
	}
	return b.String()
}

// IsPrerelease reports whether v has a prerelease part.
func (v Version) IsPrerelease() bool { return len(v.Pre) > 0 }

// Compare returns -1, 0 or 1 by semantic version precedence. Build metadata is ignored.
func (v Version) Compare(o Version) int {
	if c := cmpUint(v.Major, o.Major); c != 0 {
		return c
	}
	if c := cmpUint(v.Minor, o.Minor); c != 0 {
		return c
	}
	if c := cmpUint(v.Patch, o.Patch); c != 0 {
		return c
	}
	return comparePre(v.Pre, o.Pre)
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1 // a release is greater than any of its prereleases
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareIdentifier(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmpUint(uint64(len(a)), uint64(len(b)))
}

// compareIdentifier orders numeric identifiers by value and below alphanumeric
// ones; alphanumeric identifiers compare in ASCII order.
func compareIdentifier(a, b string) int {
	an, aNum := numericID(a)
	bn, bNum := numericID(b)
	switch {
	case aNum && bNum:
		return cmpUint(an, bn)
	case aNum:
		return -1
	case bNum:
		return 1
	}
	return strings.Compare(a, b)
}

func numericID(s string) (uint64, bool) {
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// A numeric identifier too large for uint64 sorts above every smaller
		// one only by length; treat it as the maximum, which keeps order total
		// enough for tag selection without allocating big integers.
		return ^uint64(0), true
	}
	return n, true
}

// ParseTag parses a git tag name as a version. With an empty prefix the tag is
// the version or the version after a single "v" ("1.2.3", "v1.2.3"). With a
// prefix the tag must start with it and the rest must be a plain version
// ("deploy/v" + "2.1.3"). Anything else is not a version tag (ok is false).
func ParseTag(name, prefix string) (Version, bool) {
	var rest string
	if prefix == "" {
		rest = strings.TrimPrefix(name, "v")
	} else {
		if !strings.HasPrefix(name, prefix) {
			return Version{}, false
		}
		rest = name[len(prefix):]
	}
	v, err := parse(rest)
	if err != nil {
		return Version{}, false
	}
	return v, true
}
