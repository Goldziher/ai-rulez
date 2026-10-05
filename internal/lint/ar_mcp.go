package lint

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Codes for the MCP configuration checks.
const (
	CodeMCPConfigInvalid = "AR602"
	CodeMCPUnpinned      = "AR012"
	CodeSecretInConfig   = "AR015"
)

func init() {
	registerRules(
		RuleInfo{CodeMCPUnpinned, "mcp-unpinned-package", SeverityWarning, "an MCP server (or settings entry) runs a package through npx, uvx, pipx or docker without pinning its version"},
		RuleInfo{CodeSecretInConfig, "secret-in-env-or-header", SeverityError, "an MCP server env value, header, command-line flag or settings env holds a literal credential instead of a ${VAR} reference"},
		RuleInfo{CodeMCPConfigInvalid, "mcp-config-invalid", SeverityError, "an MCP server definition is malformed: missing command or url, unknown transport, wrong field types, duplicate name, empty server or deprecated SSE transport"},
	)
	registerRunCheck(checkMCPConfig)
}

// mcpServer is an MCP server definition read from any supported source.
type mcpServer struct {
	file      string
	name      string
	transport string // as written; "" when absent
	command   string
	args      []string
	url       string
	env       map[string]string
	headers   map[string]string
	// problems found while decoding (field types), JSON sources only.
	typeProblems []string
	disabled     bool
}

// mcpJSONFiles are the hand-authored MCP files checked when tracked or present:
// path relative to the root and the key that holds the servers.
var mcpJSONFiles = []struct{ rel, key string }{
	{".mcp.json", keyMCPServers},
	{".cursor/mcp.json", keyMCPServers},
	{".vscode/mcp.json", "servers"},
}

func (r *runner) mcpServers() []*mcpServer {
	var out []*mcpServer
	if cfgPath := r.configFilePath(); cfgPath != "" {
		names := make([]string, 0, len(r.cfg.MCPServers))
		for n := range r.cfg.MCPServers {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			s := r.cfg.MCPServers[n]
			if s == nil {
				continue
			}
			out = append(out, &mcpServer{file: cfgPath, name: n, transport: s.Transport, command: s.Command, args: s.Args, url: s.URL, env: s.Env, headers: s.Headers, disabled: !s.IsEnabled()})
		}
	}
	for _, f := range mcpJSONFiles {
		p := filepath.Join(r.rootAbs(), filepath.FromSlash(f.rel))
		data, err := readSmallFile(p)
		if err != nil {
			continue
		}
		for _, srv := range decodeMCPJSON(p, f.key, data) {
			if _, generated := r.cfg.MCPServers[srv.name]; generated {
				continue // the file mirrors config.toml, which is checked at its source
			}
			out = append(out, srv)
		}
	}
	return out
}

func decodeMCPJSON(file, key string, data []byte) []*mcpServer { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return nil
	}
	var servers map[string]map[string]json.RawMessage
	if raw, ok := root[key]; !ok || json.Unmarshal(raw, &servers) != nil {
		return nil
	}
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []*mcpServer
	for _, n := range names {
		f := servers[n]
		s := &mcpServer{file: file, name: n}
		str := func(k string, dst *string) {
			if raw, ok := f[k]; ok {
				if json.Unmarshal(raw, dst) != nil {
					s.typeProblems = append(s.typeProblems, fmt.Sprintf("%q must be a string", k))
				}
			}
		}
		str(hookTypeCommand, &s.command)
		str("url", &s.url)
		str("type", &s.transport)
		if s.transport == "" {
			str("transport", &s.transport)
		}
		if raw, ok := f["args"]; ok && json.Unmarshal(raw, &s.args) != nil {
			s.typeProblems = append(s.typeProblems, `"args" must be a list of strings`)
		}
		for _, k := range []string{keyEnv, "headers"} {
			if raw, ok := f[k]; ok {
				m := map[string]string{}
				if json.Unmarshal(raw, &m) != nil {
					s.typeProblems = append(s.typeProblems, fmt.Sprintf("%q must map names to strings", k))
				}
				if k == keyEnv {
					s.env = m
				} else {
					s.headers = m
				}
			}
		}
		if raw, ok := f["disabled"]; ok {
			_ = json.Unmarshal(raw, &s.disabled) //nolint:errcheck // a non-boolean stays false
		}
		if raw, ok := f["enabled"]; ok {
			var en bool
			if json.Unmarshal(raw, &en) == nil && !en {
				s.disabled = true
			}
		}
		out = append(out, s)
	}
	return out
}

var mcpNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func effectiveTransport(s *mcpServer) string {
	t := strings.ToLower(s.transport)
	switch {
	case t == "streamable-http" || t == "streamablehttp" || t == "streamable_http":
		return transportHTTP
	case t != "":
		return t
	case s.url != "" && s.command == "":
		return transportHTTP
	}
	return "stdio"
}

func checkMCPConfig(r *runner) {
	r.checkSettingsSecrets()
	servers := r.mcpServers()
	if len(servers) == 0 {
		return
	}
	byName := map[string][]*mcpServer{}
	for _, s := range servers {
		if !s.disabled {
			key := s.file + "\x00" + strings.ToLower(s.name)
			byName[key] = append(byName[key], s)
		}
	}
	for _, s := range servers {
		if s.disabled {
			continue
		}
		lines := r.fileLines(s.file)
		at := lineContaining(lines, s.name)
		r.checkMCPShape(s, at, byName)
		r.checkMCPPins(s, at)
		r.checkMCPSecrets(s, lines, at)
	}
}

func (r *runner) checkMCPShape(s *mcpServer, at int, byName map[string][]*mcpServer) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	bad := func(format string, args ...any) {
		r.add(CodeMCPConfigInvalid, s.file, at, "MCP server %q: %s", s.name, fmt.Sprintf(format, args...))
	}
	warn := func(format string, args ...any) {
		r.addSev(SeverityWarning, CodeMCPConfigInvalid, s.file, at, "MCP server %q: %s", s.name, fmt.Sprintf(format, args...))
	}
	info := func(format string, args ...any) {
		r.addSev(SeverityInfo, CodeMCPConfigInvalid, s.file, at, "MCP server %q: %s", s.name, fmt.Sprintf(format, args...))
	}
	for _, p := range s.typeProblems {
		bad("%s", p)
	}
	t := effectiveTransport(s)
	switch t {
	case "stdio":
		switch {
		case s.command == "" && s.url == "":
			bad("defines neither a command (stdio) nor a url (http), so it is empty")
		case s.command == "":
			bad("transport is stdio but it sets no command (did you mean transport \"http\"?)")
		}
	case transportHTTP, "sse":
		if t == "sse" {
			warn("the SSE transport is deprecated in the MCP specification; use streamable http")
		}
		switch u, err := url.Parse(s.url); {
		case s.url == "":
			bad("transport is %s but it sets no url", t)
		case err != nil || (u.Scheme != transportHTTP && u.Scheme != "https") || u.Host == "":
			if !strings.Contains(s.url, "${") {
				bad("url %q is not an absolute http(s) URL", s.url)
			}
		}
		if s.command != "" {
			warn("sets a command, which a %s server ignores", t)
		}
	default:
		bad("unknown transport %q (use stdio, http or sse)", s.transport)
	}
	if !mcpNameRe.MatchString(s.name) {
		info("the name has characters outside letters, digits, _ and -; Claude Code rewrites them in the mcp__<server>__<tool> names, so allowed-tools entries must use the rewritten name")
	}
	if dup := byName[s.file+"\x00"+strings.ToLower(s.name)]; len(dup) > 1 && dup[0].file == s.file && dup[0].name == s.name && s.name != "" {
		for _, other := range dup[1:] {
			warn("is also defined as %q in the same file; names are matched case-insensitively and one definition is dropped", other.name)
		}
	}
}

func (r *runner) checkMCPPins(s *mcpServer, at int) {
	if s.command == "" {
		return
	}
	argv := append([]string{s.command}, s.args...)
	if pkg, why := pinProblem(argv, false, true); pkg != "" {
		r.add(CodeMCPUnpinned, s.file, at, "MCP server %q runs %q, but %s; pin it (for example %s) so a new release cannot change what the server does", s.name, pkg, why, pinExample(s.command, pkg))
	}
}

func pinExample(command, pkg string) string {
	switch path.Base(command) {
	case "uvx", "pipx", "uv":
		return pkg + "==1.2.3"
	case cmdDocker, "podman":
		return pkg + "@sha256:..."
	}
	name, _ := pinOf(pkg)
	return name + "@1.2.3"
}

var (
	secretKeyRe      = regexp.MustCompile(`(?i)(secret|token|passw(?:or)?d|passwd|credential|api[_-]?key|apikey|private[_-]?key|access[_-]?key|auth|bearer|cookie|session)`)
	notSecretKeyRe   = regexp.MustCompile(`(?i)(?:_|-|^)(?:url|uri|path|file|dir|host|port|user|username|region|id|enabled|mode|type|scheme|endpoint|name|version|prefix|timeout)$`)
	headerSecretRe   = regexp.MustCompile(`(?i)^(?:authorization|proxy-authorization|cookie|x-api-key|api-key|apikey|x-auth-token|x-access-token|x-[a-z0-9-]*(?:token|key|secret|password))$`)
	envRefRe         = regexp.MustCompile(`\$\{?[A-Za-z_][A-Za-z0-9_:-]*\}?|\{env:[^}]+\}|%[A-Za-z_][A-Za-z0-9_]*%|\$\(|<[^>]+>|\{\{[^}]+\}\}`)
	placeholderValRe = regexp.MustCompile(`(?i)^(?:your[-_ ].*|.*[-_]here|x{3,}|\*{3,}|changeme|replace[-_ ]?me|todo|example|redacted|none|null|true|false|\d{1,6})$`)
	authSchemeRe     = regexp.MustCompile(`(?i)^(?:bearer|basic|token|apikey|digest)\s+(.*)$`)
	flagSecretRe     = regexp.MustCompile(`(?i)^--?(?:[a-z-]*(?:token|secret|password|passwd|api-?key|apikey|auth)[a-z-]*)(?:=(.*))?$`)
	urlCredRe        = regexp.MustCompile(`(?i)://[^/\s:@]+:([^/\s@]+)@|[?&](?:api[_-]?key|apikey|token|access[_-]?token|key|secret|password|auth)=([^&\s]+)`)
)

// literalSecret reports whether value is a literal credential rather than a
// reference or placeholder.
func literalSecret(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" || envRefRe.MatchString(v) || placeholderValRe.MatchString(v) {
		return false
	}
	return true
}

func (r *runner) checkMCPSecrets(s *mcpServer, lines []string, at int) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	report := func(where, key string) {
		line := at
		for i := at; i-1 < len(lines) && i > 0; i++ {
			if strings.Contains(lines[i-1], key) {
				line = i
				break
			}
		}
		r.add(CodeSecretInConfig, s.file, line, "MCP server %q: %s %q holds a literal credential; reference an environment variable instead (for example \"${%s}\")", s.name, where, key, envNameFor(key))
	}
	for _, k := range sortedKeys(s.env) {
		v := s.env[k]
		if !literalSecret(v) {
			continue
		}
		if (secretKeyRe.MatchString(k) && !notSecretKeyRe.MatchString(k) && len(v) >= 8) || matchesBuiltinSecret(v) {
			report(keyEnv, k)
		}
	}
	for _, k := range sortedKeys(s.headers) {
		v := strings.TrimSpace(s.headers[k])
		if m := authSchemeRe.FindStringSubmatch(v); m != nil {
			v = m[1]
		}
		if !literalSecret(v) {
			continue
		}
		if (headerSecretRe.MatchString(k) && len(v) >= 8) || matchesBuiltinSecret(v) {
			report("header", k)
		}
	}
	for i, a := range s.args {
		if m := flagSecretRe.FindStringSubmatch(a); m != nil {
			val := m[1]
			if val == "" && !strings.Contains(a, "=") && i+1 < len(s.args) {
				val = s.args[i+1]
			}
			if literalSecret(val) && len(val) >= 8 {
				report("argument", strings.SplitN(a, "=", 2)[0])
				continue
			}
		}
		if matchesBuiltinSecret(a) {
			report("argument", "#"+fmt.Sprint(i+1))
		}
	}
	if m := urlCredRe.FindStringSubmatch(s.url); m != nil {
		if v := m[1] + m[2]; literalSecret(v) && len(v) >= 6 {
			report("url", "url")
		}
	}
}

func envNameFor(key string) string {
	up := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.TrimLeft(key, "-")))
	if up == "" || strings.HasPrefix(up, "#") {
		return "TOKEN"
	}
	return up
}

func matchesBuiltinSecret(v string) bool {
	for _, p := range builtinSecrets {
		if p.re.MatchString(v) {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// checkSettingsSecrets looks for literal credentials in the project
// .claude/settings.json: the top-level env block and the headers of http hooks.
func (r *runner) checkSettingsSecrets() { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	file := filepath.Join(r.rootAbs(), ".claude", "settings.json")
	data, err := readSmallFile(file)
	if err != nil {
		return
	}
	var root struct {
		Env   map[string]any                          `json:"env"`
		Hooks map[string][]map[string]json.RawMessage `json:"hooks"`
	}
	if json.Unmarshal(data, &root) != nil {
		return
	}
	lines := r.fileLines(file)
	env := map[string]string{}
	for k, v := range root.Env {
		if sv, ok := v.(string); ok {
			env[k] = sv
		}
	}
	for _, k := range sortedKeys(env) {
		v := env[k]
		if literalSecret(v) && ((secretKeyRe.MatchString(k) && !notSecretKeyRe.MatchString(k) && len(v) >= 8) || matchesBuiltinSecret(v)) {
			r.add(CodeSecretInConfig, file, lineContaining(lines, quoteNeedle(k)), "settings env %q holds a literal credential; keep it in your shell environment or settings.local.json instead", k)
		}
	}
	events := make([]string, 0, len(root.Hooks))
	for e := range root.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)
	for _, e := range events {
		for _, g := range root.Hooks[e] {
			var handlers []map[string]json.RawMessage
			if raw, ok := g[keyHooks]; !ok || json.Unmarshal(raw, &handlers) != nil {
				continue
			}
			for _, h := range handlers {
				var headers map[string]string
				if raw, ok := h["headers"]; !ok || json.Unmarshal(raw, &headers) != nil {
					continue
				}
				for _, k := range sortedKeys(headers) {
					v := strings.TrimSpace(headers[k])
					if m := authSchemeRe.FindStringSubmatch(v); m != nil {
						v = m[1]
					}
					if literalSecret(v) && ((headerSecretRe.MatchString(k) && len(v) >= 8) || matchesBuiltinSecret(v)) {
						r.add(CodeSecretInConfig, file, lineContaining(lines, quoteNeedle(k)), "%s hook header %q holds a literal credential; reference an environment variable instead", e, k)
					}
				}
			}
		}
	}
}
