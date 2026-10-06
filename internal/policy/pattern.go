package policy

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// A host pattern names where content may come from: a host, optionally
// followed by a path prefix.
//
//	github.com                  the host, any path
//	*.example.org               example.org and every subdomain
//	github.com/example-org      any path that starts with the segment example-org
//	github.com/example-*        segments may carry one leading or trailing *
//	github.com/*/rules          * is exactly one segment
//
// A pattern matches by path prefix, so a trailing /** is accepted and means
// the same as leaving it out. Everything is case-insensitive.
type pattern struct {
	host string
	segs []string
}

// parsePattern parses and normalizes a pattern; the error says what is wrong.
func parsePattern(raw string) (pattern, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return pattern{}, fmt.Errorf("empty pattern")
	}
	if strings.Contains(s, "://") || strings.ContainsAny(s, " \t\\@:?#") {
		return pattern{}, fmt.Errorf("%q is not a host pattern (write host or host/path, without a scheme)", raw)
	}
	host, rest, hasPath := strings.Cut(s, "/")
	if err := checkHost(raw, host); err != nil {
		return pattern{}, err
	}
	p := pattern{host: host}
	if !hasPath {
		return p, nil
	}
	segs, err := parseSegments(raw, strings.Split(strings.TrimSuffix(rest, "/"), "/"))
	if err != nil {
		return pattern{}, err
	}
	p.segs = segs
	return p, nil
}

// checkHost validates the host part: a concrete host or *.suffix.
func checkHost(raw, host string) error {
	if host == "" || host == "*" || host == "**" || strings.Contains(host, "**") {
		return fmt.Errorf("%q needs a concrete host or *.suffix", raw)
	}
	if i := strings.Index(host, "*"); i >= 0 && (i != 0 || !strings.HasPrefix(host, "*.") || strings.Count(host, "*") != 1 || len(host) == 2) {
		return fmt.Errorf("%q: a host wildcard is only allowed as the leading *. label", raw)
	}
	return nil
}

// parseSegments validates the path segments and drops a trailing **.
func parseSegments(raw string, segs []string) ([]string, error) {
	for i, seg := range segs {
		switch {
		case seg == "":
			return nil, fmt.Errorf("%q has an empty path segment", raw)
		case seg == "**":
			if i != len(segs)-1 {
				return nil, fmt.Errorf("%q: ** is only allowed as the last segment", raw)
			}
			return segs[:i], nil
		case !validSegmentStars(seg):
			return nil, fmt.Errorf("%q: a segment may carry one * at its start or end", raw)
		}
	}
	return segs, nil
}

// validSegmentStars allows no *, a lone *, or one * at the start or the end.
func validSegmentStars(seg string) bool {
	n := strings.Count(seg, "*")
	return n == 0 || (n == 1 && (seg == "*" || strings.HasPrefix(seg, "*") || strings.HasSuffix(seg, "*")))
}

// String is the canonical spelling of the pattern.
func (p pattern) String() string {
	if len(p.segs) == 0 {
		return p.host
	}
	return p.host + "/" + strings.Join(p.segs, "/")
}

// normalizePattern returns the canonical spelling of a pattern.
func normalizePattern(raw string) (string, error) {
	p, err := parsePattern(raw)
	if err != nil {
		return "", err
	}
	return p.String(), nil
}

func hostMatches(ph, h string) bool {
	if suffix, ok := strings.CutPrefix(ph, "*."); ok {
		return h == suffix || strings.HasSuffix(h, "."+suffix)
	}
	return ph == h
}

func segMatches(ps, v string) bool {
	switch {
	case v == "":
		return false
	case ps == "*":
		return true
	case strings.HasSuffix(ps, "*"):
		return strings.HasPrefix(v, strings.TrimSuffix(ps, "*"))
	case strings.HasPrefix(ps, "*"):
		return strings.HasSuffix(v, strings.TrimPrefix(ps, "*"))
	}
	return ps == v
}

// matches reports whether the concrete location (host and path segments, both
// lowercase) falls under the pattern.
func (p pattern) matches(host string, segs []string) bool {
	if !hostMatches(p.host, host) || len(segs) < len(p.segs) {
		return false
	}
	for i, ps := range p.segs {
		if !segMatches(ps, segs[i]) {
			return false
		}
	}
	return true
}

// segCovers reports whether every segment ps matches is also matched by pp. r
// may itself carry a wildcard.
func segCovers(pp, rs string) bool {
	switch {
	case pp == "*":
		return true
	case !strings.Contains(pp, "*"):
		return pp == rs
	case strings.HasSuffix(pp, "*"):
		pre := strings.TrimSuffix(pp, "*")
		if strings.HasPrefix(rs, "*") {
			return false // "*x" also matches strings that do not start with pre
		}
		return strings.HasPrefix(strings.TrimSuffix(rs, "*"), pre)
	default: // "*suf"
		suf := strings.TrimPrefix(pp, "*")
		if strings.HasSuffix(rs, "*") {
			return false
		}
		return strings.HasSuffix(strings.TrimPrefix(rs, "*"), suf)
	}
}

// hostCovers reports whether every host rh matches is also matched by ph.
func hostCovers(ph, rh string) bool {
	if suffix, ok := strings.CutPrefix(ph, "*."); ok {
		r := strings.TrimPrefix(rh, "*.")
		return r == suffix || strings.HasSuffix(r, "."+suffix)
	}
	return ph == rh // a literal host covers only itself
}

// Covers reports whether every location matched by repo is also matched by
// policy. It is conservative: when the answer cannot be proved it is false, so
// a false reject is possible and a false accept is not. An unparsable pattern
// covers nothing and is covered by nothing.
func Covers(policy, repo string) bool {
	pp, err := parsePattern(policy)
	if err != nil {
		return false
	}
	rp, err := parsePattern(repo)
	if err != nil {
		return false
	}
	if !hostCovers(pp.host, rp.host) || len(rp.segs) < len(pp.segs) {
		return false
	}
	for i, ps := range pp.segs {
		if !segCovers(ps, rp.segs[i]) {
			return false
		}
	}
	return true
}

// splitLocation parses where a source points: a git URL or scp-like address
// gives its lowercase host and path segments. ok is false for a local path,
// which names no host.
func splitLocation(source string) (host string, segs []string, ok bool) {
	src := strings.TrimSpace(source)
	if src == "" || !lockfile.IsGitSource(src) {
		return "", nil, false
	}
	lower := strings.ToLower(src)
	lower = strings.TrimPrefix(lower, "git+")
	var path string
	if _, rest, found := strings.Cut(lower, "://"); found {
		if strings.HasPrefix(lower, "file://") {
			return "", nil, false
		}
		authority, p, _ := strings.Cut(rest, "/")
		if i := strings.LastIndex(authority, "@"); i >= 0 {
			authority = authority[i+1:]
		}
		host, path = authority, p
	} else { // scp-like user@host:path
		_, afterAt, _ := strings.Cut(lower, "@")
		h, p, _ := strings.Cut(afterAt, ":")
		host, path = h, p
	}
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host, "]") {
		host = host[:i] // drop the port
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" {
		return "", nil, false
	}
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			segs = append(segs, seg)
		}
	}
	return host, segs, true
}
