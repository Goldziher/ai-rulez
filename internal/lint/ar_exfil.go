package lint

import (
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Codes for the exfiltration and transport rules.
const (
	CodeExfilCommand     = "AR014"
	CodeImageExfil       = "AR016"
	CodeInsecureHTTP     = "AR024"
	CodeRawIPURL         = "AR025"
	CodeDataURILink      = "AR026"
	CodeEscapeObfuscated = "AR023"
)

func init() {
	MarkExampleAware(CodeEscapeObfuscated, CodeInsecureHTTP, CodeRawIPURL)
	registerRules(
		RuleInfo{CodeExfilCommand, "exfil-command", SeverityError, "a network command sends a secret environment variable, the environment or a credential file off the machine (curl/wget/nc with $TOKEN, DNS exfiltration)"},
		RuleInfo{CodeImageExfil, "markdown-image-exfil", SeverityWarning, "a markdown image URL carries a query string; rendering it in an agent UI sends the query to the host"},
		RuleInfo{CodeEscapeObfuscated, "escape-sequence-obfuscation", SeverityInfo, "four or more consecutive \\xNN or \\uNNNN escapes hide a string from a reviewer"},
		RuleInfo{CodeInsecureHTTP, "insecure-transport", SeverityWarning, "a plain http:// URL (not loopback or private) is fetched by a command, used by an MCP server, or is the source of an include"},
		RuleInfo{CodeRawIPURL, "raw-ip-url", SeverityInfo, "a URL points at a public IPv4 address instead of a host name"},
		RuleInfo{CodeDataURILink, "data-uri-link", SeverityWarning, "a markdown link or image target starts with data: or javascript:"},
	)
	registerTextScan(scanExfilCommands, AnalyzerSecurity)
	registerTextScan(scanImageExfil, AnalyzerSecurity)
	registerTextScan(scanDataURIs, AnalyzerSecurity)
	registerTextScan(scanRawIPs, AnalyzerSecurity)
	registerTextScan(scanInsecureHTTP, AnalyzerSecurity)
	registerTextScan(scanEscapes, AnalyzerSecurity)
	registerRunCheck(checkInsecureConfig, AnalyzerSecurity)
}

// logicalLines returns the lines of t with shell continuation lines joined into
// the line that starts them.
func (t *scanText) logicalLines() []scanLine {
	var out []scanLine
	for i := 0; i < len(t.lines); i++ {
		l := t.lines[i]
		for t.shellLike(l) && strings.HasSuffix(strings.TrimRight(l.Text, " \t"), `\`) && i+1 < len(t.lines) &&
			t.lines[i+1].Fenced == l.Fenced && t.lines[i+1].Block == l.Block {
			i++
			l.Text = strings.TrimSuffix(strings.TrimRight(l.Text, " \t"), `\`) + " " + strings.TrimSpace(t.lines[i].Text)
		}
		out = append(out, l)
	}
	return out
}

var (
	netCmdRe    = regexp.MustCompile(`(?i)(?:^|[\s;&|(` + "`" + `])(?:curl|wget|xh|nc|ncat|netcat|scp|rsync|iwr|irm|invoke-webrequest|invoke-restmethod)\s`)
	httpieRe    = regexp.MustCompile(`(?:^|[\s;&|(])https?\s+(?:-\S+\s+)*(?:GET|POST|PUT|PATCH|DELETE)\s`)
	secretVarRe = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?`)
	secretName  = regexp.MustCompile(`(?i)(?:^|_)(?:secret|token|api_?key|passw(?:or)?d|private_?key|access_?key|secret_?key|credentials?|auth)$|^(?:DATABASE_URL|DB_URL|DB_PASSWORD)$|^AWS_SECRET`)
	headerArgRe = regexp.MustCompile(`(?i)(?:-H|--header)\s*("[^"]*"|'[^']*')|(?:-u|--user)\s+\S+|--oauth2-bearer\s+\S+`)
	envDumpRe   = regexp.MustCompile(`(?i)(?:^|[\s;&|(])(?:curl|wget|nc|ncat|xh)\b[^\n]*(?:\$\(\s*(?:env|printenv|set)\s*\)|` + "`" + `\s*(?:env|printenv)\s*` + "`" + `|@-?\s*<\(\s*(?:env|printenv)|\$\(\s*cat\s+\S*(?:\.ssh/|\.aws/|\.netrc|\.git-credentials|\.npmrc|\.kube/|/\.env\b)\S*\s*\))`)
	envPipeRe   = regexp.MustCompile(`(?i)(?:(?:^|[\s;&(])(?:env|printenv)|cat\s+\S*(?:\.ssh/|\.aws/|\.netrc|\.git-credentials|\.npmrc|\.kube/|/\.env\b)\S*)\s*(?:\|[^|\n]*)*\|\s*(?:curl|wget|nc|ncat|xh)\b`)
	dnsExfilRe  = regexp.MustCompile(`^(?:dig|nslookup|host)\s+(?:[+@-]\S*\s+)*\S*\$\(`)
	dnsTickRe   = regexp.MustCompile(`^(?:dig|nslookup|host)\s+(?:[+@-]\S*\s+)*\S*` + "`")
	dnsStartRe  = regexp.MustCompile(`^(?:dig|nslookup|host)\s`)
)

// secretVars lists the secret-looking environment variables a line references.
func secretVars(line string) []string {
	var out []string
	for _, m := range secretVarRe.FindAllStringSubmatch(line, -1) {
		if secretName.MatchString(m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

func scanExfilCommands(r *runner, t *scanText) {
	for _, l := range t.logicalLines() {
		if l.Front {
			continue
		}
		text := l.Text
		network := netCmdRe.MatchString(text) || httpieRe.MatchString(text)
		switch {
		case network && len(secretVars(text)) > 0:
			if r.allHostsAllowed(text) {
				continue
			}
			rest := headerArgRe.ReplaceAllString(text, " ")
			if len(secretVars(rest)) == 0 {
				r.addSev(SeverityWarning, CodeExfilCommand, t.abs, l.No, "sends $%s in an authentication header to a host that is not in lint.security.allowed_hosts; confirm the destination", secretVars(text)[0])
			} else {
				r.add(CodeExfilCommand, t.abs, l.No, "sends the secret $%s to a remote host in a URL or body", secretVars(rest)[0])
			}
		case envDumpRe.MatchString(text), envPipeRe.MatchString(text):
			if r.allHostsAllowed(text) {
				continue
			}
			r.add(CodeExfilCommand, t.abs, l.No, "sends the environment or a credential file to a remote host")
		default:
			for _, seg := range t.commandSegmentsWith(l, dnsStartRe, pipeSegSplitRe) {
				if dnsExfilRe.MatchString(seg) || (t.shellLike(l) && dnsTickRe.MatchString(seg)) {
					r.add(CodeExfilCommand, t.abs, l.No, "builds a DNS lookup from command output, a classic exfiltration channel")
					break
				}
			}
		}
	}
}

var (
	imageURLRe = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?(https?://[^)\s>]+)`)
	badgeHosts = []string{"img.shields.io", "badgen.net", "codecov.io", "*.codecov.io", "badge.fury.io", "*.travis-ci.com", "*.travis-ci.org", "api.codacy.com", "sonarcloud.io", "coveralls.io", "circleci.com", "*.githubusercontent.com", "camo.githubusercontent.com", "github.com", "gitlab.com", "readthedocs.org", "*.readthedocs.io"}
)

func scanImageExfil(r *runner, t *scanText) {
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		for _, m := range imageURLRe.FindAllStringSubmatch(l.Text, -1) {
			u, err := url.Parse(m[1])
			if err != nil || u.RawQuery == "" {
				continue
			}
			host := strings.ToLower(u.Hostname())
			if hostAllowed(host, badgeHosts) && (host != "github.com" && host != "gitlab.com" || strings.Contains(u.Path, "badge")) {
				continue
			}
			if lo, _ := hostClass(host); lo || hostAllowed(host, r.security().AllowedHosts) {
				continue
			}
			r.add(CodeImageExfil, t.abs, l.No, "image URL on %s carries a query string; an agent UI that renders it sends the query to that host", host)
		}
	}
}

var (
	dataLinkRe = regexp.MustCompile(`(?i)\]\(\s*<?(data:[^)\s>]*|javascript:[^)]*|vbscript:[^)]*)`)
	dataRefRe  = regexp.MustCompile(`(?i)^\s{0,3}\[[^\]\n]+\]:\s*<?(data:\S*|javascript:\S*)`)
	smallImgRe = regexp.MustCompile(`(?i)^data:image/(?:png|gif|jpe?g|webp);base64,`)
)

// maxInlineImage is the largest base64 image payload treated as a harmless icon.
const maxInlineImage = 4096

func scanDataURIs(r *runner, t *scanText) {
	for _, l := range t.lines {
		if !t.prose(l) {
			continue
		}
		matches := dataLinkRe.FindAllStringSubmatch(l.Text, -1)
		if m := dataRefRe.FindStringSubmatch(l.Text); m != nil {
			matches = append(matches, m)
		}
		for _, m := range matches {
			target := m[1]
			if smallImgRe.MatchString(target) && len(target) <= maxInlineImage {
				continue
			}
			scheme, _, _ := strings.Cut(target, ":")
			r.add(CodeDataURILink, t.abs, l.No, "link target uses the %s: scheme, which can carry an executable or hidden payload", strings.ToLower(scheme))
		}
	}
}

var (
	ipURLRe     = regexp.MustCompile(`(?i)\bhttps?://(\d{1,3}(?:\.\d{1,3}){3})(?::\d+)?\b`)
	testNetNets = []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"}
)

func publicIPv4(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return false
	}
	if lo, priv := hostClass(s); lo || priv || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.Equal(net.IPv4bcast) {
		return false
	}
	for _, n := range testNetNets {
		if inCIDR(ip, n) {
			return false
		}
	}
	return true
}

func scanRawIPs(r *runner, t *scanText) {
	for _, l := range t.lines {
		if l.Front || l.Neg {
			continue
		}
		for _, m := range ipURLRe.FindAllStringSubmatch(l.Text, -1) {
			if publicIPv4(m[1]) {
				r.add(CodeRawIPURL, t.abs, l.No, "URL points at the public address %s instead of a host name", m[1])
			}
		}
	}
}

var (
	fetchCmdRe = regexp.MustCompile(`(?i)\b(?:curl|wget|git\s+clone|git\s+remote\s+add|pip3?\s+install|npm\s+(?:install|i|config)|iwr|invoke-webrequest|invoke-restmethod)\b`)
	httpURLRe  = regexp.MustCompile(`(?i)\bhttp://([^\s/:"'<>)\]$]+)(?::\d+)?`)
)

func scanInsecureHTTP(r *runner, t *scanText) {
	for _, l := range t.lines {
		if l.Front || l.Neg || !fetchCmdRe.MatchString(l.Text) {
			continue
		}
		for _, m := range httpURLRe.FindAllStringSubmatch(l.Text, -1) {
			host := strings.ToLower(m[1])
			if lo, priv := hostClass(host); lo || priv || !strings.Contains(host, ".") {
				continue
			}
			r.add(CodeInsecureHTTP, t.abs, l.No, "fetches http://%s over plain HTTP; use https so the download cannot be altered in transit", host)
		}
	}
}

func checkInsecureConfig(r *runner) {
	for _, s := range r.mcpServers() {
		if s.disabled || s.url == "" || !strings.HasPrefix(strings.ToLower(s.url), "http://") {
			continue
		}
		u, err := url.Parse(s.url)
		if err != nil {
			continue
		}
		if lo, priv := hostClass(u.Hostname()); lo || priv {
			continue
		}
		lines := r.fileLines(s.file)
		r.add(CodeInsecureHTTP, s.file, lineContaining(lines, s.name), "MCP server %q connects over plain http://%s; use https", s.name, u.Host)
	}
	cfgPath := r.configFilePath()
	if cfgPath == "" {
		return
	}
	lines := r.fileLines(cfgPath)
	check := func(kind, name, source string) {
		if !strings.HasPrefix(strings.ToLower(source), "http://") {
			return
		}
		if u, err := url.Parse(source); err == nil {
			if lo, priv := hostClass(u.Hostname()); lo || priv {
				return
			}
		}
		r.add(CodeInsecureHTTP, cfgPath, lineContaining(lines, `"`+name+`"`), "%s %q is fetched over plain HTTP; use an https or git source", kind, name)
	}
	for i := range r.cfg.Includes {
		check("include", r.cfg.Includes[i].Name, r.cfg.Includes[i].Source)
	}
	for i := range r.cfg.InstalledSkills {
		check("installed skill", r.cfg.InstalledSkills[i].Name, r.cfg.InstalledSkills[i].Source)
	}
}

var (
	escapeRe     = regexp.MustCompile(`(?:\\x[0-9a-fA-F]{2}){4,}|(?:\\u[0-9a-fA-F]{4}){4,}`)
	escapeSkipFn = map[string]bool{"regex": true, "regexp": true, "js": true, "javascript": true, "ts": true, "typescript": true, "json": true, "c": true, "cpp": true, "go": true, "rust": true, "java": true}
	escapeSkipEx = map[string]bool{".js": true, ".ts": true, ".json": true, ".c": true, ".h": true, ".go": true, ".rs": true, ".java": true, ".mjs": true, ".cjs": true}
)

func scanEscapes(r *runner, t *scanText) {
	if !t.md && escapeSkipEx[strings.ToLower(filepath.Ext(t.abs))] {
		return
	}
	for _, l := range t.lines {
		if l.Front || l.Neg || (l.Fenced && escapeSkipFn[l.Lang]) {
			continue
		}
		if escapeRe.MatchString(l.Text) {
			r.add(CodeEscapeObfuscated, t.abs, l.No, "a run of character escapes spells out text a reviewer cannot read; write the string plainly")
		}
	}
}
