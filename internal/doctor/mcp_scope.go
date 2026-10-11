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
	"github.com/Goldziher/ai-rulez/v5/schema"
)

// checkMCPScope compares the project's MCP servers with the same-named servers
// in the user and local scope of each configured harness, and reports generated
// servers that differ only by name while pointing at the same url. It reads
// files outside the project, so it runs only when Options.ReadUserMCP is set.
func checkMCPScope(_ context.Context, s *state) []Finding {
	if s.cfg == nil || !s.opts.ReadUserMCP {
		return nil
	}
	return MCPScopeFindings(s.cfg)
}

// MCPScopeFindings is the mcp-scope check on its own, for callers that already
// hold the configuration (validate --strict). It reads user files, so callers
// must gate it behind an explicit opt-in.
func MCPScopeFindings(cfg *config.Config) []Finding {
	if cfg == nil {
		return nil
	}
	project := projectScopeServers(cfg)
	var out []Finding
	out = append(out, duplicateURLFindings(project)...)
	home, err := ambient.OS().UserHomeDir()
	if err != nil {
		return out // no home directory: nothing to compare the project against
	}
	return append(out, scopeConflictFindings(cfg, project, home)...)
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
	remove    func(scope, name string) string
}

// harnessMCPScopes lists the harnesses with a known user/local MCP scope. Claude
// Code keeps both in ~/.claude.json: the top-level mcpServers is the user scope,
// and projects[<dir>].mcpServers is the local scope. Its user-scope file is the
// global_mcp_path its provider spec declares (internal/generator/providers).
func harnessMCPScopes() map[string]harnessMCPScope {
	return map[string]harnessMCPScope{
		string(config.PresetClaude): {
			shadowing: claudeShadowing,
			remove:    func(scope, name string) string { return "claude mcp remove " + name + " -s " + scope },
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
	entry := cfg.SelfMCPServerEntry(schema.Version)
	s := mcpScopeServer{Name: config.SelfMCPServerName, Scope: "project", Origin: projectMCPFile}
	s.Command, _ = entry["command"].(string) //nolint:errcheck // a non-string command stays empty
	if args, ok := entry["args"].([]string); ok {
		s.Args = append([]string(nil), args...)
	}
	return s
}

// scopeConflictFindings reports each project server whose name is also defined
// in a configured harness's user or local scope with a different endpoint.
func scopeConflictFindings(cfg *config.Config, project []mcpScopeServer, home string) []Finding {
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
		for _, pj := range project {
			other, ok := shadow[pj.Name]
			if !ok || pj.endpoint() == other.endpoint() {
				continue
			}
			out = append(out, Finding{
				Check: CheckMCPScope, Severity: SeverityWarning, Path: projectMCPFile,
				Message: fmt.Sprintf("MCP server %q is defined in multiple scopes with different endpoints: project (%s: %s) and %s (%s: %s). OAuth tokens are stored per endpoint, so authenticating in one context will not carry over.",
					pj.Name, pj.Origin, pj.endpoint(), other.Scope, other.Origin, other.endpoint()),
				Hint: fmt.Sprintf("remove the %s copy: %s", other.Scope, scope.remove(other.Scope, pj.Name)),
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
