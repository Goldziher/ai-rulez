package policy

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode"

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
	if strings.HasPrefix(s, "[") {
		return parseIPv6Pattern(raw, s)
	}
	if strings.Contains(s, "://") || strings.ContainsAny(s, " \t\\@:?#") {
		return pattern{}, fmt.Errorf("%q is not a host pattern (write host or host/path, without a scheme)", raw)
	}
	host, rest, hasPath := strings.Cut(s, "/")
	if err := checkHost(raw, host); err != nil {
		return pattern{}, err
	}
	return finishPattern(raw, host, rest, hasPath)
}

// parseIPv6Pattern parses "[addr]" or "[addr]/path".
func parseIPv6Pattern(raw, s string) (pattern, error) {
	end := strings.Index(s, "]")
	if end < 0 {
		return pattern{}, fmt.Errorf("%q: an IPv6 host needs a closing ]", raw)
	}
	rest := s[end+1:]
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return pattern{}, fmt.Errorf("%q: nothing but a path may follow the IPv6 host", raw)
	}
	addr, err := netip.ParseAddr(s[1:end])
	if err != nil || addr.Zone() != "" || !addr.Is6() {
		return pattern{}, fmt.Errorf("%q is not a valid IPv6 address", raw)
	}
	path, hasPath := strings.CutPrefix(rest, "/")
	return finishPattern(raw, addr.String(), path, hasPath)
}

// finishPattern attaches the path segments to a validated host.
func finishPattern(raw, host, rest string, hasPath bool) (pattern, error) {
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
	host := p.host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if len(p.segs) == 0 {
		return host
	}
	return host + "/" + strings.Join(p.segs, "/")
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
// which names no host. It fails closed: a location that cannot be read the way
// git reads it (a backslash, whitespace, a malformed authority) is an error,
// never a guess, because a guessed host defeats the host policy.
func splitLocation(source string) (host string, segs []string, ok bool, err error) {
	src := strings.TrimSpace(source)
	if src == "" || !lockfile.IsGitSource(src) {
		return "", nil, false, nil
	}
	if strings.ContainsAny(src, "\\") || strings.IndexFunc(src, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", nil, false, fmt.Errorf("the location contains a backslash, whitespace or a control character")
	}
	lower := strings.TrimPrefix(strings.ToLower(src), "git+")
	var path string
	if scheme, _, found := strings.Cut(lower, "://"); found {
		if scheme == "file" {
			return "", nil, false, nil
		}
		u, perr := url.Parse(lower)
		if perr != nil || u.Opaque != "" {
			return "", nil, false, fmt.Errorf("the location is not a valid URL")
		}
		host, path = u.Hostname(), u.Path
	} else { // scp-like [user@]host:path
		host, path, err = splitSCP(lower)
		if err != nil {
			return "", nil, false, err
		}
	}
	if host, err = canonicalHost(host); err != nil {
		return "", nil, false, err
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			segs = append(segs, seg)
		}
	}
	return host, segs, true, nil
}

// splitSCP splits "[user@]host:path" (host may be a bracketed IPv6 literal).
// ssh reads the host after the last "@".
func splitSCP(s string) (host, path string, err error) {
	limit := strings.Index(s, ":")
	if i := strings.Index(s, "]:"); i >= 0 && strings.Contains(s[:i], "[") {
		limit = i
	}
	if limit < 0 {
		return "", "", fmt.Errorf("the scp-like address is malformed")
	}
	if at := strings.LastIndex(s[:limit], "@"); at >= 0 {
		s = s[at+1:]
	}
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 || end+1 >= len(s) || s[end+1] != ':' {
			return "", "", fmt.Errorf("the scp-like address is malformed")
		}
		return s[:end+1], s[end+2:], nil
	}
	h, p, found := strings.Cut(s, ":")
	if !found || strings.ContainsAny(h, "/@?#%") {
		return "", "", fmt.Errorf("the scp-like address is malformed")
	}
	return h, p, nil
}

// canonicalHost validates a host name or IP literal and returns its lowercase
// form without brackets, port or trailing dot.
func canonicalHost(h string) (string, error) {
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	if strings.Contains(h, ":") {
		addr, err := netip.ParseAddr(h)
		if err != nil || addr.Zone() != "" {
			return "", fmt.Errorf("%q is not a valid IPv6 address", h)
		}
		return addr.String(), nil
	}
	h = strings.TrimRight(h, ".")
	if h == "" {
		return "", fmt.Errorf("the location has no host")
	}
	for _, r := range h {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '-' && r != '_' {
			return "", fmt.Errorf("the host %q has a character that is not allowed", h)
		}
	}
	return h, nil
}
