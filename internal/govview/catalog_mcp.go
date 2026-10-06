package govview

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// MCPValue describes one environment variable or HTTP header of an MCP server
// without its value: the name, the variable it references when the value is a
// pure ${VAR} placeholder, and whether a literal value is configured. The value
// itself never enters the catalog.
type MCPValue struct {
	Name string `json:"name"`
	// Ref is the variable a ${VAR} placeholder names; empty for a literal value.
	Ref string `json:"ref,omitempty"`
	// Literal says the value is written into the configuration rather than
	// referenced from the environment.
	Literal bool `json:"literal"`
}

// CatalogMCPServer is one MCP server of the project, described by what a
// reviewer needs and nothing that could leak a launch secret: no arguments, no
// URL, no env or header values.
type CatalogMCPServer struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Transport is stdio, http or sse.
	Transport string `json:"transport"`
	// CommandBasename is the executable's file name, without its directory or arguments.
	CommandBasename string `json:"command_basename,omitempty"`
	Enabled         bool   `json:"enabled"`
	// Profiles restricts the server to these profiles; empty means every profile.
	Profiles []string `json:"profiles"`
	// Pinned is true when the launch names an exact package version or image
	// digest, false when a package runner launches an unpinned one, and null when
	// the notion does not apply (a local executable or a remote server).
	Pinned   *bool      `json:"pinned"`
	Env      []MCPValue `json:"env"`
	Headers  []MCPValue `json:"headers"`
	Warnings []string   `json:"warnings"`
}

var (
	// mcpPlaceholderRE matches a value that is exactly one ${VAR} reference.
	mcpPlaceholderRE = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
	// mcpPinRE matches an exact version ("pkg@1.2.3", "pkg==1.2.3") or a digest ("img@sha256:...").
	mcpPinRE    = regexp.MustCompile(`(?:(?:@|==)v?[0-9]+(?:\.[0-9]+)+(?:[-+.][0-9A-Za-z.-]+)?|@sha256:[0-9a-f]{64})$`)
	mcpSecretRE = regexp.MustCompile(`(?i)token|secret|password|passwd|credential|api[_-]?key|auth|bearer`)
	// mcpRunners are the commands that download and run a package or image.
	mcpRunners = []string{"npx", "bunx", "pnpx", "uvx", "pipx", "dlx", "deno", "docker", "podman"}
)

// catalogMCPServers describes the project's MCP servers, sorted by name.
func catalogMCPServers(cfg *config.Config) []CatalogMCPServer {
	servers := cfg.EffectiveMCPServers()
	out := make([]CatalogMCPServer, 0, len(servers))
	seen := map[string]int{}
	for i := range servers {
		s := &servers[i]
		ref := "mcp/" + s.Name
		seen[ref]++
		if n := seen[ref]; n > 1 {
			ref += "#" + strconv.Itoa(n)
		}
		out = append(out, CatalogMCPServer{
			Ref: ref, Name: s.Name, Description: s.Description, Transport: s.GetTransport(),
			CommandBasename: commandBasename(s.Command), Enabled: s.IsEnabled(),
			Profiles: sortedStrings(s.Profiles), Pinned: mcpPinned(s),
			Env: mcpValues(s.Env), Headers: mcpValues(s.Headers),
			Warnings: mcpWarnings(s),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sortedStrings(s []string) []string {
	out := slices.Clone(s)
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	return out
}

func commandBasename(cmd string) string {
	cmd = strings.TrimSpace(strings.ReplaceAll(cmd, `\`, "/"))
	if cmd == "" {
		return ""
	}
	if fields := strings.Fields(cmd); len(fields) > 0 {
		cmd = fields[0] // a command written with its arguments inline: keep the executable only
	}
	return cmd[strings.LastIndex(cmd, "/")+1:]
}

func mcpValues(m map[string]string) []MCPValue {
	out := make([]MCPValue, 0, len(m))
	for name, value := range m {
		v := MCPValue{Name: name}
		if match := mcpPlaceholderRE.FindStringSubmatch(strings.TrimSpace(value)); match != nil {
			v.Ref = match[1]
		} else {
			v.Literal = true
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// mcpPinned reports whether a package-runner launch pins an exact version; nil
// when the server is not launched by a package runner.
func mcpPinned(s *config.MCPServer) *bool {
	if s.GetTransport() != config.TransportStdio {
		return nil
	}
	base := strings.TrimSuffix(strings.ToLower(commandBasename(s.Command)), ".cmd")
	if !slices.Contains(mcpRunners, base) {
		return nil
	}
	pinned := false
	for _, arg := range s.Args {
		if !strings.HasPrefix(arg, "-") && mcpPinRE.MatchString(arg) {
			pinned = true
			break
		}
	}
	return &pinned
}

func mcpWarnings(s *config.MCPServer) []string {
	out := []string{}
	if p := mcpPinned(s); p != nil && !*p {
		out = append(out, "launch is not pinned to an exact version or digest")
	}
	for _, group := range []struct {
		kind   string
		values map[string]string
	}{{"environment variable", s.Env}, {"header", s.Headers}} {
		names := make([]string, 0, len(group.values))
		for name, value := range group.values {
			if mcpSecretRE.MatchString(name) && !mcpPlaceholderRE.MatchString(strings.TrimSpace(value)) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, group.kind+" "+name+" looks like a credential but holds a literal value: reference it as ${VAR}")
		}
	}
	return out
}
