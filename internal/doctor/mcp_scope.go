package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// checkMCPScope compares the project's MCP servers with the same-named servers
// in the user and local scope of each configured harness, reports generated
// servers that differ only by name while pointing at the same url, and reports a
// user-scope server whose launch command does not resolve. It reads files outside
// the project, so it runs only when Options.ReadUserMCP is set.
func checkMCPScope(_ context.Context, s *state) []Finding {
	if s.cfg == nil || !s.opts.ReadUserMCP {
		return nil
	}
	return MCPScopeFindings(s.cfg, s.opts.LookPath)
}

// MCPScopeFindings is the mcp-scope check on its own, for callers that already
// hold the configuration (validate --strict). It reads user files, so callers
// must gate it behind an explicit opt-in. A nil lookPath resolves binaries on
// PATH.
func MCPScopeFindings(cfg *config.Config, lookPath func(string) (string, error)) []Finding {
	if cfg == nil {
		return nil
	}
	if lookPath == nil {
		lookPath = runner.LookPath
	}
	project := projectScopeServers(cfg)
	var out []Finding
	out = append(out, duplicateURLFindings(project)...)
	home, err := ambient.OS().UserHomeDir()
	if err != nil {
		return out // no home directory: nothing to compare the project against
	}
	return append(out, scopeConflictFindings(cfg, project, home, lookPath)...)
}

// mcpScopeEntry is an MCP server as written in a JSON MCP document. A harness
// spells the transport either "type" (Claude, VS Code) or "transport".
type mcpScopeEntry struct {
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	URL       string   `json:"url"`
	Type      string   `json:"type"`
	Transport string   `json:"transport"`
}

// mcpScopeServer is one MCP server read from a project or user file.
type mcpScopeServer struct {
	Name      string
	Scope     string // "project", "user" or "local"
	Origin    string // the file it was read from, for the message
	Command   string
	Args      []string
	URL       string
	Transport string
}

// endpoint is the identity OAuth tokens and connections are stored against: the
// url of a remote server, else its launch command line.
func (s mcpScopeServer) endpoint() string {
	if s.remote() {
		return s.URL
	}
	return strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
}

// remote reports whether the server speaks http or sse rather than stdio.
func (s mcpScopeServer) remote() bool {
	switch strings.ToLower(s.Transport) {
	case config.TransportHTTP, config.TransportSSE:
		return true
	}
	return s.URL != "" && s.Command == ""
}

// harnessMCPScope is where a configured harness keeps the user and local scope
// MCP servers that shadow the project's, and how to remove one. Only harnesses
// whose user-scope MCP file this repository knows how to read are listed; a
// configured harness missing here is skipped rather than guessed at.
type harnessMCPScope struct {
	shadowing func(home, projectDir string) map[string]mcpScopeServer
	remove    func(scope, name, origin string) string
}

// harnessMCPScopes lists the harnesses with a known user/local MCP scope. Claude
// Code keeps both in ~/.claude.json: the top-level mcpServers is the user scope,
// and projects[<dir>].mcpServers is the local scope. Its user-scope file is the
// global_mcp_path its provider spec declares (internal/generator/providers).
// OpenCode keeps one user-level opencode.json and no per-project MCP scope.
func harnessMCPScopes() map[string]harnessMCPScope {
	return map[string]harnessMCPScope{
		string(config.PresetClaude): {
			shadowing: claudeShadowing,
			remove: func(scope, name, _ string) string {
				return "claude mcp remove " + name + " -s " + scope
			},
		},
		string(config.PresetOpenCode): {
			shadowing: opencodeShadowing,
			remove: func(_, name, origin string) string {
				return fmt.Sprintf("remove %q from %s", name, origin)
			},
		},
	}
}

// claudeShadowing reads the user and local MCP servers Claude Code keeps in
// ~/.claude.json. A missing or unreadable file is not an error: it simply holds
// no shadowing server.
func claudeShadowing(home, projectDir string) map[string]mcpScopeServer {
	path := filepath.Join(home, ".claude.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		MCPServers map[string]mcpScopeEntry `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers map[string]mcpScopeEntry `json:"mcpServers"`
		} `json:"projects"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	origin := displayHomePath(home, path)
	out := map[string]mcpScopeServer{}
	for name, e := range doc.MCPServers {
		out[name] = entryServer(name, "user", origin, e)
	}
	for dir, proj := range doc.Projects {
		if filepath.Clean(dir) != filepath.Clean(projectDir) {
			continue
		}
		for name, e := range proj.MCPServers {
			out[name] = entryServer(name, "local", origin, e)
		}
	}
	return out
}

// entryServer turns a decoded JSON entry into a scope server.
func entryServer(name, scope, origin string, e mcpScopeEntry) mcpScopeServer {
	transport := e.Transport
	if transport == "" {
		transport = e.Type
	}
	return mcpScopeServer{
		Name: name, Scope: scope, Origin: origin,
		Command: e.Command, Args: append([]string(nil), e.Args...),
		URL: e.URL, Transport: transport,
	}
}

// opencodeServersKey is the canonical nested map of OpenCode v2 servers.
const opencodeServersKey = "servers"

// opencodeEntry is a server as OpenCode's opencode.json spells it: the v2 shape
// (`mcp.servers.<name>`, switched off with `disabled`) and the v1 compat shape
// (`mcp.<name>`, switched off with `enabled`) share the command array.
type opencodeEntry struct {
	Type     string   `json:"type"`
	URL      string   `json:"url"`
	Command  []string `json:"command"`
	Disabled bool     `json:"disabled"`
	Enabled  *bool    `json:"enabled"`
}

// active reports whether OpenCode starts the server.
func (e opencodeEntry) active() bool {
	if e.Disabled {
		return false
	}
	return e.Enabled == nil || *e.Enabled
}

// opencodeUserConfigPath is the user-level opencode.json OpenCode reads:
// $XDG_CONFIG_HOME/opencode/opencode.json, or ~/.config/opencode/opencode.json.
func opencodeUserConfigPath(home string) string {
	base := ambient.Getenv(ambient.OS(), "XDG_CONFIG_HOME")
	if base == "" || !filepath.IsAbs(base) {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "opencode", "opencode.json")
}

// opencodeShadowing reads the user-level MCP servers OpenCode keeps in
// opencode.json, in both shapes it accepts: the canonical `mcp.servers` map and
// the v1 compat `mcp.<name>` members. A missing or unreadable file is not an
// error: it simply holds no shadowing server.
func opencodeShadowing(home, _ string) map[string]mcpScopeServer {
	path := opencodeUserConfigPath(home)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		MCP map[string]json.RawMessage `json:"mcp"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	origin := displayHomePath(home, path)
	out := map[string]mcpScopeServer{}
	add := func(servers map[string]opencodeEntry) {
		for name, e := range servers {
			if !e.active() {
				continue
			}
			server := mcpScopeServer{Name: name, Scope: "user", Origin: origin, URL: e.URL, Transport: e.Type}
			if len(e.Command) > 0 {
				server.Command = e.Command[0]
				server.Args = append([]string(nil), e.Command[1:]...)
			}
			out[name] = server
		}
	}
	if raw, ok := doc.MCP[opencodeServersKey]; ok {
		var servers map[string]opencodeEntry
		if json.Unmarshal(raw, &servers) == nil {
			add(servers)
		}
	}
	for name, raw := range doc.MCP {
		if name == opencodeServersKey || name == "timeout" {
			continue
		}
		if _, nested := out[name]; nested {
			continue
		}
		var entry opencodeEntry
		if json.Unmarshal(raw, &entry) == nil {
			add(map[string]opencodeEntry{name: entry})
		}
	}
	return out
}

// projectMCPFile is the committed MCP document the project scope lives in.
const projectMCPFile = ".mcp.json"

// projectScopeServers is what the harness reads as project scope: the committed
// .mcp.json (generated and hand-authored entries) plus the servers ai-rulez
// would generate that are not in it yet.
func projectScopeServers(cfg *config.Config) []mcpScopeServer {
	var out []mcpScopeServer
	seen := map[string]bool{}
	if data, err := os.ReadFile(filepath.Join(cfg.BaseDir, projectMCPFile)); err == nil {
		var doc struct {
			MCPServers map[string]mcpScopeEntry `json:"mcpServers"`
		}
		if json.Unmarshal(data, &doc) == nil {
			for name, e := range doc.MCPServers {
				out = append(out, entryServer(name, "project", projectMCPFile, e))
				seen[name] = true
			}
		}
	}
	servers := cfg.EffectiveMCPServers()
	for i := range servers {
		srv := &servers[i]
		if !srv.IsEnabled() || seen[srv.Name] {
			continue
		}
		out = append(out, mcpScopeServer{
			Name: srv.Name, Scope: "project", Origin: projectMCPFile,
			Command: srv.Command, Args: append([]string(nil), srv.Args...),
			URL: srv.URL, Transport: srv.Transport,
		})
		seen[srv.Name] = true
	}
	if cfg.HasSelfServer() && !seen[config.SelfMCPServerName] {
		out = append(out, selfScopeServer(cfg))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// selfScopeServer is the entry generation writes for ai-rulez's own MCP server.
func selfScopeServer(cfg *config.Config) mcpScopeServer {
	entry := cfg.SelfMCPServerEntry()
	s := mcpScopeServer{Name: config.SelfMCPServerName, Scope: "project", Origin: projectMCPFile}
	s.Command, _ = entry["command"].(string) //nolint:errcheck // a non-string command stays empty
	if args, ok := entry["args"].([]string); ok {
		s.Args = append([]string(nil), args...)
	}
	return s
}

// scopeConflictFindings reports each project server whose name is also defined
// in a configured harness's user or local scope with a different endpoint, and
// each user-scope server whose launch command cannot be resolved: the harness
// fails to start those with "NotFound: ChildProcess.spawn".
func scopeConflictFindings(cfg *config.Config, project []mcpScopeServer, home string, lookPath func(string) (string, error)) []Finding {
	harnesses := map[string]bool{}
	for _, n := range config.IndividualPresetNames() {
		harnesses[n] = true
	}
	var out []Finding
	var skipped []string
	for i := range cfg.Presets {
		name := cfg.Presets[i].BuiltIn
		if !cfg.Presets[i].IsBuiltIn() {
			continue
		}
		scope, known := harnessMCPScopes()[name]
		if !known {
			if harnesses[name] {
				skipped = append(skipped, name)
			}
			continue
		}
		shadow := scope.shadowing(home, cfg.BaseDir)
		out = append(out, unresolvedCommandFindings(name, shadow, lookPath)...)
		for _, pj := range project {
			other, ok := shadow[pj.Name]
			if !ok || pj.endpoint() == other.endpoint() {
				continue
			}
			out = append(out, Finding{
				Check: CheckMCPScope, Severity: SeverityWarning, Path: projectMCPFile,
				Message: fmt.Sprintf("MCP server %q is defined in multiple scopes with different endpoints: project (%s: %s) and %s (%s: %s). OAuth tokens are stored per endpoint, so authenticating in one context will not carry over.",
					pj.Name, pj.Origin, pj.endpoint(), other.Scope, other.Origin, other.endpoint()),
				Hint: fmt.Sprintf("remove the %s copy: %s", other.Scope, scope.remove(other.Scope, pj.Name, other.Origin)),
			})
		}
	}
	if len(skipped) > 0 {
		out = append(out, Finding{
			Check: CheckMCPScope, Severity: SeverityInfo,
			Message: "user and local scope not checked for " + strings.Join(dedupeSorted(skipped), ", ") + ": no known user-scope MCP file",
			Hint:    "only the harnesses whose user-scope MCP file this tool can read are inspected; the rest are skipped rather than guessed at",
		})
	}
	return out
}

// unresolvedCommandFindings reports the user-scope stdio servers of one harness
// whose launch executable cannot be resolved, so the harness cannot start them.
func unresolvedCommandFindings(harness string, shadow map[string]mcpScopeServer, lookPath func(string) (string, error)) []Finding {
	names := make([]string, 0, len(shadow))
	for name := range shadow {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []Finding
	for _, name := range names {
		server := shadow[name]
		if server.remote() || server.Command == "" {
			continue
		}
		problem := commandProblem(server.Command, lookPath)
		if problem == "" {
			continue
		}
		out = append(out, Finding{
			Check: CheckMCPScope, Severity: SeverityWarning,
			Message: fmt.Sprintf("MCP server %q in %s's %s scope (%s) runs %q, which %s; the harness cannot start it",
				server.Name, harness, server.Scope, server.Origin, server.Command, problem),
			Hint: fmt.Sprintf("install it, point the entry at a command that exists (for example npx -y ai-rulez@latest mcp), or remove it from %s", server.Origin),
		})
	}
	return out
}

// commandProblem says how a launch executable does not resolve, or "" when it
// does. An absolute path is checked as a file; anything else is looked up on
// PATH, the way the harness launches it.
func commandProblem(command string, lookPath func(string) (string, error)) string {
	if filepath.IsAbs(command) {
		if _, err := os.Stat(command); err != nil {
			return "does not exist"
		}
		return ""
	}
	if _, err := lookPath(command); err != nil {
		return "is not on PATH"
	}
	return ""
}

// duplicateURLFindings reports generated remote servers that differ only by name
// while pointing at the same url, a likely duplicate.
func duplicateURLFindings(project []mcpScopeServer) []Finding {
	byURL := map[string][]string{}
	for _, s := range project {
		if s.remote() && s.URL != "" {
			byURL[s.URL] = append(byURL[s.URL], s.Name)
		}
	}
	urls := make([]string, 0, len(byURL))
	for u := range byURL {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	var out []Finding
	for _, u := range urls {
		names := dedupeSorted(byURL[u])
		if len(names) < 2 {
			continue
		}
		out = append(out, Finding{
			Check: CheckMCPScope, Severity: SeverityWarning, Path: projectMCPFile,
			Message: fmt.Sprintf("MCP servers %s point to the same url %s; they differ only by name and are likely duplicates", quoteNames(names), u),
			Hint:    "keep one entry, or give each server a distinct url",
		})
	}
	return out
}

// quoteNames joins server names as `"a" and "b"` or `"a", "b" and "c"`.
func quoteNames(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	if len(quoted) == 2 {
		return quoted[0] + " and " + quoted[1]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}

// dedupeSorted returns the unique, sorted values of names.
func dedupeSorted(names []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// displayHomePath shortens a path under home to ~ for the finding message.
func displayHomePath(home, path string) string {
	rel, err := filepath.Rel(home, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.ToSlash(filepath.Join("~", rel))
}
