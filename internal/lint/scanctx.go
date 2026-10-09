package lint

import (
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// scanLine is one line of a scanned text with the context the rules need.
type scanLine struct {
	No    int
	Text  string
	Plain string // Text with inline code spans blanked (prose lines)
	// Fenced is true inside a fenced code block; Lang is its info string's first
	// word, lower-case; Block numbers the fence (0 outside fences).
	Fenced bool
	Lang   string
	Block  int
	// Front is true inside the YAML frontmatter.
	Front bool
	// Neg marks a line that documents a bad example or a guardrail ("never run
	// rm -rf /"): the nearest heading, the line itself, or the text that
	// introduces its fence carries a word such as never, avoid or dangerous.
	Neg bool
}

// scanText is one scanned file split into lines.
type scanText struct {
	abs   string
	raw   string
	md    bool
	lines []scanLine
}

var (
	// negRe reads a line that talks *about* a risky command rather than
	// instructing it.
	negRe = newWordGatedRe(`(?i)\b(?:never|don'?t|do\s+not|must\s+not|should\s+not|shouldn'?t|avoid|instead\s+of|rather\s+than|forbidden|prohibit\w*|disallow\w*|denied|dangerous|unsafe|insecure|malicious|attack\w*|exploit\w*|anti-?patterns?|bad|wrong|incorrect|harmful|destructive|risky|refuse\w*|reject\w*|vulnerab\w*|injection|threat|red\s+flags?)\b|❌|⛔|🚫|⚠`, true,
		"never", "don", "shouldn", "not", "avoid", "instead", "rather", "forbid", "prohibit", "disallow", "denied", "dangerous", "unsafe", "insecure",
		"malicious", "attack", "exploit", "anti", "bad", "wrong", "incorrect", "harmful", "destructive", "risky", "refuse", "reject",
		"vulnerab", "injection", "threat", "red", "❌", "⛔", "🚫", "⚠")

	shellLangs = map[string]bool{"": true, "sh": true, shellBash: true, shellZsh: true, "shell": true, "console": true, "terminal": true, "shellsession": true, "shell-session": true, "fish": true}
)

func isMarkdownPath(abs string) bool {
	switch strings.ToLower(filepath.Ext(abs)) {
	case ".md", ".mdc", ".markdown", ".mdx":
		return true
	}
	return false
}

func newScanText(r *runner, abs, raw string) *scanText { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	t := &scanText{abs: abs, raw: raw, md: isMarkdownPath(abs)}
	lines := strings.Split(raw, "\n")
	bodyStart := 0
	if t.md {
		bodyStart = parseDoc(raw).bodyStart
	}
	var open, lang string
	block, headingNeg, openNeg := 0, false, false
	var prev [2]string // the last two non-blank, non-fenced lines
	for i, text := range lines {
		l := scanLine{No: i + 1, Text: text, Plain: text}
		if i < bodyStart {
			l.Front = true
			t.lines = append(t.lines, l)
			continue
		}
		if !t.md {
			l.Neg = negRe.MatchString(text)
			t.lines = append(t.lines, l)
			continue
		}
		if m := fenceRe().FindStringSubmatch(text); m != nil {
			run, info := m[1], strings.TrimSpace(m[2])
			switch {
			case open == "":
				if run[0] != '`' || !strings.Contains(info, "`") {
					open = run
					block++
					lang = ""
					if f := strings.Fields(info); len(f) > 0 {
						lang = strings.ToLower(f[0])
					}
					openNeg = negRe.MatchString(info) || negRe.MatchString(prev[0]) || negRe.MatchString(prev[1])
					l.Fenced, l.Lang, l.Block, l.Neg = true, lang, block, openNeg
					t.lines = append(t.lines, l)
					continue
				}
			case run[0] == open[0] && len(run) >= len(open) && info == "":
				l.Fenced, l.Lang, l.Block = true, lang, block
				open = ""
				t.lines = append(t.lines, l)
				continue
			}
		}
		if open != "" {
			l.Fenced, l.Lang, l.Block = true, lang, block
			l.Neg = openNeg || headingNeg || negRe.MatchString(text)
			t.lines = append(t.lines, l)
			continue
		}
		if m := headingRe().FindStringSubmatch(text); m != nil {
			headingNeg = negRe.MatchString(m[2])
		}
		l.Plain = codeSpanRe().ReplaceAllStringFunc(text, func(s string) string { return strings.Repeat(" ", len(s)) })
		l.Neg = headingNeg || negRe.MatchString(text)
		if strings.TrimSpace(text) != "" {
			prev[0], prev[1] = prev[1], text
		}
		t.lines = append(t.lines, l)
	}
	return t
}

// shellLike reports whether a line is shell: a line of a script, or a line of a
// fenced block tagged (or not tagged) as shell.
func (t *scanText) shellLike(l scanLine) bool {
	if l.Front {
		return false
	}
	if !t.md {
		return true
	}
	return l.Fenced && shellLangs[l.Lang]
}

// prose reports whether a line is markdown prose (outside frontmatter and fences).
func (t *scanText) prose(l scanLine) bool { return t.md && !l.Fenced && !l.Front }

var (
	segSplitRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`&&|\|\||[;|]|\$\(|\x60`) })
	// pipeSegSplitRe splits only at command separators, keeping $( and backticks inside a word.
	pipeSegSplitRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`&&|\|\||[;|]`) })
)

// commandSegments returns the command-shaped pieces of a line: every segment of
// a shell line, the inline code spans of a prose line, and a prose line that
// itself starts with one of the start words ("npx -y pkg" as a bare line).
func (t *scanText) commandSegments(l scanLine, start *regexp.Regexp) []string {
	return t.commandSegmentsWith(l, start, segSplitRe())
}

// commandSegmentsWith is commandSegments with the separator pattern chosen by the caller.
func (t *scanText) commandSegmentsWith(l scanLine, start, split *regexp.Regexp) []string {
	var src []string
	switch {
	case t.shellLike(l):
		src = []string{l.Text}
	case t.prose(l):
		for _, m := range backtickRe().FindAllStringSubmatch(l.Text, -1) {
			src = append(src, m[1])
		}
		bare := strings.TrimSpace(listMarkerRe().ReplaceAllString(strings.TrimSpace(l.Text), ""))
		bare = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(bare, "$ "), "> "))
		if start != nil && start.MatchString(bare) {
			src = append(src, bare)
		}
	default:
		return nil
	}
	var out []string
	for _, s := range src {
		s = stripShellComment(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "$ ")))
		for _, seg := range split.Split(s, -1) {
			if seg = strings.TrimSpace(seg); seg != "" {
				out = append(out, seg)
			}
		}
	}
	return out
}

func stripShellComment(s string) string {
	inS, inD := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'' && !inD:
			inS = !inS
		case c == '"' && !inS:
			inD = !inD
		case c == '#' && !inS && !inD && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

// hostClass classifies a URL host. loopback covers localhost and 127/8;
// private covers RFC 1918, link-local, CGNAT, .local and .internal names.
func hostClass(host string) (loopback, private bool) {
	host = strings.ToLower(strings.Trim(host, "[]"))
	switch {
	case host == hostLocalhost || strings.HasSuffix(host, ".localhost") || host == "0.0.0.0" || host == "::1":
		return true, true
	case strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".home.arpa"):
		return false, true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified(), ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || inCIDR(ip, "100.64.0.0/10")
	}
	return false, false
}

func inCIDR(ip net.IP, cidr string) bool {
	_, n, err := net.ParseCIDR(cidr)
	return err == nil && n.Contains(ip)
}

// urlHosts lists the hosts of the URLs on a line.
func urlHosts(line string) []string {
	var out []string
	for _, m := range urlRe().FindAllStringSubmatch(line, -1) {
		out = append(out, strings.ToLower(m[1]))
	}
	return out
}

// allHostsAllowed reports whether the line names at least one URL and every one
// is in lint.security.allowed_hosts (or is loopback).
func (r *runner) allHostsAllowed(line string) bool {
	allowed := r.security().AllowedHosts
	hosts := urlHosts(line)
	if len(allowed) == 0 || len(hosts) == 0 {
		return false
	}
	for _, h := range hosts {
		if lo, _ := hostClass(h); !lo && !hostAllowed(h, allowed) {
			return false
		}
	}
	return true
}
