package semver

import (
	"strings"

	"github.com/samber/oops"
)

// Constraint is a parsed npm-style version range: comparator sets joined by
// "||", each set a space-separated AND of comparators.
//
// Grammar (whitespace between an operator and its version is allowed):
//
//	range      = set { "||" set }
//	set        = hyphen | comparator { " " comparator }
//	hyphen     = partial " - " partial
//	comparator = [ "^" | "~" | "~>" | ">=" | "<=" | ">" | "<" | "=" ] partial
//	partial    = [ "v" ] ( "*" | "x" | "X" | MAJOR [ "." ( "x" | MINOR [ "." ( "x" | PATCH [ "-" pre ] ) ] ) ] )
//
// A bare partial ("1.2", "1.x") matches the range it names. "*" matches any
// release. A prerelease version satisfies a range only when a comparator of the
// same set names a prerelease of the same major.minor.patch ("^1.2.3-beta.2"
// matches 1.2.3-beta.4 and not 1.3.0-beta.1), unless the caller asks to include
// prereleases.
type Constraint struct {
	raw  string
	sets [][]comparator
}

type comparator struct {
	op string // one of "=", "<", "<=", ">", ">="
	v  Version
}

// ParseConstraint parses a constraint. The empty string and an empty "||"
// alternative are errors: a constraint must say what it allows.
func ParseConstraint(s string) (*Constraint, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, oops.Errorf("empty version constraint")
	}
	c := &Constraint{raw: raw}
	for _, alt := range strings.Split(raw, "||") {
		set, err := parseSet(strings.TrimSpace(alt))
		if err != nil {
			return nil, oops.With("constraint", s).Wrapf(err, "invalid version constraint %q", s)
		}
		c.sets = append(c.sets, set)
	}
	return c, nil
}

// String is the constraint as written (trimmed).
func (c *Constraint) String() string { return c.raw }

// Check reports whether v satisfies the constraint. Build metadata is ignored.
// With includePrerelease a prerelease satisfies any range its bounds contain;
// without it, only the ranges that name a prerelease of v's major.minor.patch.
func (c *Constraint) Check(v Version, includePrerelease bool) bool {
	for _, set := range c.sets {
		if setMatches(set, v, includePrerelease) {
			return true
		}
	}
	return false
}

func setMatches(set []comparator, v Version, includePrerelease bool) bool {
	for _, cmp := range set {
		if !cmp.matches(v) {
			return false
		}
	}
	if !v.IsPrerelease() || includePrerelease {
		return true
	}
	for _, cmp := range set {
		if cmp.v.IsPrerelease() && cmp.v.Major == v.Major && cmp.v.Minor == v.Minor && cmp.v.Patch == v.Patch {
			return true
		}
	}
	return false
}

func (c comparator) matches(v Version) bool {
	r := v.Compare(c.v)
	switch c.op {
	case "<":
		return r < 0
	case "<=":
		return r <= 0
	case ">":
		return r > 0
	case ">=":
		return r >= 0
	}
	return r == 0
}

func parseSet(s string) ([]comparator, error) {
	if s == "" {
		return nil, oops.Errorf("empty comparator set")
	}
	tokens := strings.Fields(s)
	if len(tokens) == 3 && tokens[1] == "-" {
		return hyphen(tokens[0], tokens[2])
	}
	// Join an operator that was separated from its version (">= 1.2.3").
	var joined []string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if isOperator(t) {
			if i+1 >= len(tokens) {
				return nil, oops.Errorf("operator %q has no version", t)
			}
			i++
			t += tokens[i]
		}
		joined = append(joined, t)
	}
	var set []comparator
	for _, t := range joined {
		cs, err := desugar(t)
		if err != nil {
			return nil, err
		}
		set = append(set, cs...)
	}
	return set, nil
}

func isOperator(t string) bool {
	switch t {
	case "^", "~", "~>", ">=", "<=", ">", "<", "=":
		return true
	}
	return false
}

// partial is a version with optional parts: -1 marks a missing or wildcard part.
type partial struct {
	major, minor, patch int64
	pre                 []string
	build               string
}

func (p partial) any() bool { return p.major < 0 }

func parsePartial(s string) (partial, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "="), "v")
	if s == "" {
		return partial{}, oops.Errorf("missing version")
	}
	rest := s
	var p partial
	if i := strings.IndexByte(rest, '+'); i >= 0 {
		rest, p.build = rest[:i], rest[i+1:]
		if !validIdentifiers(p.build, false) {
			return partial{}, oops.Errorf("invalid build metadata in %q", s)
		}
	}
	hasPre := false
	if i := strings.IndexByte(rest, '-'); i >= 0 {
		var pre string
		rest, pre, hasPre = rest[:i], rest[i+1:], true
		if !validIdentifiers(pre, true) {
			return partial{}, oops.Errorf("invalid prerelease in %q", s)
		}
		p.pre = strings.Split(pre, ".")
	}
	parts := strings.Split(rest, ".")
	if len(parts) > 3 {
		return partial{}, oops.Errorf("%q has more than three parts", s)
	}
	nums := [3]int64{-1, -1, -1}
	wild := false
	for i, part := range parts {
		switch part {
		case "x", "X", "*":
			wild = true
			continue
		}
		if wild {
			return partial{}, oops.Errorf("%q: a number cannot follow a wildcard", s)
		}
		n, ok := parseNumeric(part)
		if !ok || n > 1<<62 {
			return partial{}, oops.Errorf("%q: %q is not a valid number", s, part)
		}
		nums[i] = int64(n) //nolint:gosec // bounded above
	}
	p.major, p.minor, p.patch = nums[0], nums[1], nums[2]
	if hasPre && p.patch < 0 {
		return partial{}, oops.Errorf("%q: a prerelease needs MAJOR.MINOR.PATCH", s)
	}
	if p.major < 0 && (p.minor >= 0 || p.patch >= 0) {
		return partial{}, oops.Errorf("%q: invalid wildcard", s)
	}
	return p, nil
}

func ver(major, minor, patch int64, pre ...string) Version {
	return Version{Major: uint64(major), Minor: uint64(minor), Patch: uint64(patch), Pre: pre} //nolint:gosec // non-negative
}

// below is the exclusive upper bound "<M.m.p-0": it excludes every prerelease of
// M.m.p, which a range ending before M.m.p must not admit.
func below(major, minor, patch int64) comparator {
	return comparator{"<", ver(major, minor, patch, "0")}
}

func atLeast(major, minor, patch int64, pre []string) comparator {
	return comparator{">=", Version{Major: uint64(major), Minor: uint64(minor), Patch: uint64(patch), Pre: pre}} //nolint:gosec // non-negative
}

var anyRelease = []comparator{{">=", Version{}}}

func desugar(token string) ([]comparator, error) {
	op, rest := splitOperator(token)
	p, err := parsePartial(rest)
	if err != nil {
		return nil, err
	}
	switch op {
	case "^":
		return caret(p), nil
	case "~", "~>":
		return tilde(p), nil
	case ">":
		if p.any() {
			return nil, oops.Errorf("%q: > with a wildcard matches nothing", token)
		}
		switch {
		case p.minor < 0:
			return []comparator{atLeast(p.major+1, 0, 0, nil)}, nil
		case p.patch < 0:
			return []comparator{atLeast(p.major, p.minor+1, 0, nil)}, nil
		}
		return []comparator{{">", full(p)}}, nil
	case ">=":
		if p.any() {
			return anyRelease, nil
		}
		return []comparator{atLeast(p.major, max(p.minor, 0), max(p.patch, 0), p.pre)}, nil
	case "<":
		if p.any() {
			return nil, oops.Errorf("%q: < with a wildcard matches nothing", token)
		}
		if p.patch < 0 {
			return []comparator{below(p.major, max(p.minor, 0), 0)}, nil
		}
		return []comparator{{"<", full(p)}}, nil
	case "<=":
		if p.any() {
			return anyRelease, nil
		}
		switch {
		case p.minor < 0:
			return []comparator{below(p.major+1, 0, 0)}, nil
		case p.patch < 0:
			return []comparator{below(p.major, p.minor+1, 0)}, nil
		}
		return []comparator{{"<=", full(p)}}, nil
	}
	// exact or wildcard
	switch {
	case p.any():
		return anyRelease, nil
	case p.minor < 0:
		return []comparator{atLeast(p.major, 0, 0, nil), below(p.major+1, 0, 0)}, nil
	case p.patch < 0:
		return []comparator{atLeast(p.major, p.minor, 0, nil), below(p.major, p.minor+1, 0)}, nil
	}
	return []comparator{{"=", full(p)}}, nil
}

func full(p partial) Version {
	v := ver(p.major, p.minor, p.patch, p.pre...)
	v.Build = p.build
	return v
}

func caret(p partial) []comparator {
	switch {
	case p.any():
		return anyRelease
	case p.minor < 0:
		return []comparator{atLeast(p.major, 0, 0, nil), below(p.major+1, 0, 0)}
	case p.patch < 0:
		lo := atLeast(p.major, p.minor, 0, nil)
		if p.major > 0 {
			return []comparator{lo, below(p.major+1, 0, 0)}
		}
		return []comparator{lo, below(0, p.minor+1, 0)}
	}
	lo := atLeast(p.major, p.minor, p.patch, p.pre)
	switch {
	case p.major > 0:
		return []comparator{lo, below(p.major+1, 0, 0)}
	case p.minor > 0:
		return []comparator{lo, below(0, p.minor+1, 0)}
	}
	return []comparator{lo, below(0, 0, p.patch+1)}
}

func tilde(p partial) []comparator {
	switch {
	case p.any():
		return anyRelease
	case p.minor < 0:
		return []comparator{atLeast(p.major, 0, 0, nil), below(p.major+1, 0, 0)}
	}
	return []comparator{atLeast(p.major, p.minor, max(p.patch, 0), p.pre), below(p.major, p.minor+1, 0)}
}

func hyphen(from, to string) ([]comparator, error) {
	lo, err := parsePartial(from)
	if err != nil {
		return nil, err
	}
	hi, err := parsePartial(to)
	if err != nil {
		return nil, err
	}
	var set []comparator
	if !lo.any() {
		set = append(set, atLeast(lo.major, max(lo.minor, 0), max(lo.patch, 0), lo.pre))
	}
	switch {
	case hi.any():
	case hi.minor < 0:
		set = append(set, below(hi.major+1, 0, 0))
	case hi.patch < 0:
		set = append(set, below(hi.major, hi.minor+1, 0))
	default:
		set = append(set, comparator{"<=", full(hi)})
	}
	if len(set) == 0 {
		return anyRelease, nil
	}
	return set, nil
}

func splitOperator(t string) (op, rest string) {
	for _, o := range []string{"~>", ">=", "<=", "^", "~", ">", "<", "="} {
		if strings.HasPrefix(t, o) {
			return o, t[len(o):]
		}
	}
	return "", t
}
