package agentplugins

import (
	"fmt"
	"maps"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Wire server types (§7.2.1).
const (
	typeStdio          = "stdio"
	typeStreamableHTTP = "streamable-http"
	typeSSE            = "sse"
)

// The only placeholders Agent Plugins clients expand, and only in args, env
// values and cwd (§9.2).
const (
	phRoot = "${PLUGIN_ROOT}"
	phData = "${PLUGIN_DATA}"
)

var placeholderRE = regexp.MustCompile(`\$\{[^}]*\}`)

// wireServer is one mcp.json server entry, the closed union of the stdio and
// remote variants. Field order is the output order.
type wireServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type wireMCP struct {
	Schema     string                `json:"$schema"`
	MCPServers map[string]wireServer `json:"mcpServers"`
}

// checkMode selects how strictly placeholders and credentials are judged. A
// placeholder other than PLUGIN_ROOT/PLUGIN_DATA is valid in a package (it is
// passed literally), so Validate warns; Build refuses to package one because
// the ai-rulez placeholder would never be expanded.
type checkMode int

const (
	modeBuild checkMode = iota
	modeValidate
)

type issue struct {
	code string
	sev  Severity
	msg  string
}

func invalid(format string, a ...any) issue {
	return issue{CodeServerInvalid, SeverityError, fmt.Sprintf(format, a...)}
}

func hasError(issues []issue) bool {
	for _, i := range issues {
		if i.sev == SeverityError {
			return true
		}
	}
	return false
}

// toWire maps a model server onto the mcp.json variant set, rewriting what
// has a lossless portable form, and reports whether it can be packaged.
func toWire(s MCPServer) (wireServer, []issue, bool) {
	if s.Enabled != nil && !*s.Enabled {
		return wireServer{}, []issue{{CodeServerDisabled, SeverityInfo, "server is disabled (enabled = false) and is not packaged"}}, false
	}
	transport := strings.ToLower(s.Transport)
	if transport == "" {
		transport = TransportStdio
		if s.Command == "" && s.URL != "" {
			transport = TransportHTTP
		}
	}
	var (
		ws     wireServer
		issues []issue
	)
	switch transport {
	case TransportStdio:
		ws, issues = stdioToWire(s)
	case TransportHTTP, TransportStreamableHTTP, TransportSSE:
		ws = wireServer{Type: typeStreamableHTTP, URL: s.URL, Headers: s.Headers}
		if transport == TransportSSE {
			ws.Type = typeSSE
		}
		if s.Command != "" || len(s.Args) > 0 || len(s.Env) > 0 || s.Cwd != "" {
			issues = append(issues, issue{CodeFieldIgnored, SeverityWarning,
				"command, args, env and cwd do not apply to a remote server and are not packaged"})
		}
	default:
		return wireServer{}, []issue{invalid("unknown transport %q (use stdio, http, streamable-http or sse)", s.Transport)}, false
	}
	issues = append(issues, checkServer(ws, modeBuild)...)
	return ws, issues, !hasError(issues)
}

func stdioToWire(s MCPServer) (wireServer, []issue) {
	ws := wireServer{Type: typeStdio, Command: s.Command, Args: s.Args, Cwd: s.Cwd}
	var issues []issue
	if s.URL != "" || len(s.Headers) > 0 {
		issues = append(issues, issue{CodeFieldIgnored, SeverityWarning,
			"url and headers do not apply to a stdio server and are not packaged"})
	}
	if rest, ok := strings.CutPrefix(s.Command, phRoot+"/"); ok {
		ws.Command = "./" + rest
		issues = append(issues, issue{CodePlaceholderRewritten, SeverityInfo, fmt.Sprintf(
			"command %q is written as %q: clients resolve ./ paths against the plugin root and expand no placeholder in command",
			s.Command, ws.Command)})
	}
	for _, k := range slices.Sorted(maps.Keys(s.Env)) {
		v := s.Env[k]
		if v == "${"+k+"}" {
			issues = append(issues, issue{CodePlaceholderRewritten, SeverityWarning, fmt.Sprintf(
				"env %s = %q is dropped: Agent Plugins expands no environment placeholder, so the client must supply %s itself",
				k, v, k)})
			continue
		}
		if ws.Env == nil {
			ws.Env = map[string]string{}
		}
		ws.Env[k] = v
	}
	return ws, issues
}

// checkServer applies the rules of §7.2.1 and §9.2 that the schema cannot
// express to one server entry.
func checkServer(ws wireServer, m checkMode) []issue {
	switch ws.Type {
	case typeStdio:
		return checkStdio(ws, m)
	case typeStreamableHTTP, typeSSE:
		return checkRemote(ws, m)
	default:
		return []issue{invalid("unknown server type %q", ws.Type)}
	}
}

func checkStdio(ws wireServer, m checkMode) []issue {
	var issues []issue
	if msg := commandProblem(ws.Command); msg != "" {
		issues = append(issues, invalid("%s", msg))
	}
	for i, a := range ws.Args {
		issues = append(issues, placeholderIssues(m, fmt.Sprintf("args[%d]", i), a)...)
	}
	for _, k := range slices.Sorted(maps.Keys(ws.Env)) {
		if k == "PLUGIN_ROOT" || k == "PLUGIN_DATA" {
			issues = append(issues, invalid("env must not set %s; the client supplies it", k))
			continue
		}
		issues = append(issues, placeholderIssues(m, "env "+k, ws.Env[k])...)
	}
	if ws.Cwd != "" {
		if msg := cwdProblem(ws.Cwd); msg != "" {
			issues = append(issues, invalid("%s", msg))
		}
		issues = append(issues, placeholderIssues(m, "cwd", ws.Cwd)...)
	}
	return issues
}

func commandProblem(cmd string) string {
	switch {
	case cmd == "":
		return "command is required for a stdio server"
	case strings.Contains(cmd, "${"):
		return fmt.Sprintf("command %q contains a placeholder; clients expand none in command "+
			"(use a ./ plugin-relative path for a bundled executable)", cmd)
	case strings.IndexFunc(cmd, unicode.IsSpace) >= 0:
		return fmt.Sprintf("command %q is not a single executable token; move its arguments to args", cmd)
	case strings.HasPrefix(cmd, "./"):
		rel := cmd[2:]
		if !within(rel) || strings.Trim(rel, "./") == "" {
			return fmt.Sprintf("command %q must name a file inside the plugin root", cmd)
		}
	case strings.ContainsAny(cmd, `/\`):
		return fmt.Sprintf("command %q must be a bare executable name or a ./ plugin-relative path", cmd)
	}
	return ""
}

func cwdProblem(cwd string) string {
	var rel string
	switch {
	case cwd == phRoot || cwd == phData:
		return ""
	case strings.HasPrefix(cwd, "./"):
		rel = cwd[2:]
	case strings.HasPrefix(cwd, phRoot+"/"):
		rel = cwd[len(phRoot)+1:]
	case strings.HasPrefix(cwd, phData+"/"):
		rel = cwd[len(phData)+1:]
	default:
		return fmt.Sprintf("cwd %q must be a ./ plugin-relative path or start with ${PLUGIN_ROOT} or ${PLUGIN_DATA}", cwd)
	}
	if !within(rel) {
		return fmt.Sprintf("cwd %q escapes its root", cwd)
	}
	return ""
}

// placeholderIssues reports placeholders other than PLUGIN_ROOT/PLUGIN_DATA,
// which clients pass on literally.
func placeholderIssues(m checkMode, field, value string) []issue {
	var issues []issue
	for _, ph := range placeholderRE.FindAllString(value, -1) {
		if ph == phRoot || ph == phData {
			continue
		}
		issues = append(issues, foreignPlaceholder(m, field, ph))
	}
	return issues
}

func foreignPlaceholder(m checkMode, field, ph string) issue {
	if m == modeBuild {
		return issue{CodePlaceholder, SeverityError, fmt.Sprintf(
			"%s uses %s, which Agent Plugins clients do not expand; only %s and %s are expanded, in args, env values and cwd",
			field, ph, phRoot, phData)}
	}
	return issue{CodePlaceholder, SeverityWarning, fmt.Sprintf(
		"%s contains %s, which clients pass on literally", field, ph)}
}

func checkRemote(ws wireServer, m checkMode) []issue {
	var issues []issue
	if phs := placeholderRE.FindAllString(ws.URL, -1); len(phs) > 0 {
		for _, ph := range phs {
			issues = append(issues, foreignPlaceholder(m, "url", ph))
		}
		if m == modeBuild {
			return issues
		}
	}
	if msg := urlProblem(ws.URL); msg != "" {
		issues = append(issues, invalid("%s", msg))
	}
	seen := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(ws.Headers)) {
		value := ws.Headers[name]
		lower := strings.ToLower(name)
		switch {
		case !validHeaderName(name):
			issues = append(issues, invalid("header name %q is not a valid HTTP field name", name))
		case !validHeaderValue(value):
			issues = append(issues, invalid("header %s has a value with control characters", name))
		case seen[lower] != "":
			issues = append(issues, invalid("headers %s and %s differ only in case", seen[lower], name))
		}
		seen[lower] = name
		for _, ph := range placeholderRE.FindAllString(name+value, -1) {
			issues = append(issues, foreignPlaceholder(m, "header "+name, ph))
		}
		if lower == "authorization" || lower == "proxy-authorization" || lower == "cookie" {
			sev := SeverityError
			if m == modeValidate {
				sev = SeverityWarning
			}
			issues = append(issues, issue{CodeCredentialHeader, sev, fmt.Sprintf(
				"header %s carries credentials; Agent Plugins headers are visible package data and must not hold secrets", name)})
		}
	}
	return issues
}

func urlProblem(raw string) string {
	if raw == "" {
		return "url is required for a remote server"
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return fmt.Sprintf("url %q is not a valid URL", raw)
	case (u.Scheme != "https" && u.Scheme != "http") || u.Host == "":
		return fmt.Sprintf("url %q must be an absolute http or https URL", raw)
	case u.User != nil:
		return fmt.Sprintf("url %q must not contain user information", raw)
	case u.Fragment != "" || strings.Contains(raw, "#"):
		return fmt.Sprintf("url %q must not contain a fragment", raw)
	case u.Scheme == "http" && !loopbackHost(u.Hostname()):
		return fmt.Sprintf("url %q uses http to a non-loopback host; use https "+
			"(http is allowed only for localhost and loopback addresses)", raw)
	}
	return ""
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// validHeaderName reports whether name is an RFC 9110 token.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !isTokenChar(r) {
			return false
		}
	}
	return true
}

func isTokenChar(r rune) bool {
	if r > unicode.MaxASCII {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

func validHeaderValue(value string) bool {
	for _, r := range value {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return false
		}
	}
	return true
}
