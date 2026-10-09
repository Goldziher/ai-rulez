// Package parity is the checked-in map between the ai-rulez command line and
// the MCP tools. Every runnable command and every tool belongs to exactly one
// capability here, either paired or deliberately left on one side with a stated
// reason. tests/parity walks the real Cobra tree and the real tool lists and
// fails when this table and the code disagree, so a new command or tool cannot
// ship without a decision about the other side.
package parity

import (
	"sort"
	"strings"
)

// Kind says where a capability lives.
type Kind string

const (
	// Paired capabilities exist on both sides and are compared.
	Paired Kind = "paired"
	// CLIOnly capabilities are deliberately not tools; Reason says why.
	CLIOnly Kind = "cli-only"
	// MCPOnly capabilities are deliberately not commands; Reason says why.
	MCPOnly Kind = "mcp-only"
)

// Server names the MCP surface a tool belongs to.
type Server string

const (
	// Authoring is `ai-rulez mcp`, the tools that edit and check a project.
	Authoring Server = "authoring"
	// Skills is `ai-rulez mcp --serve-skills`, the read-only skill-serving surface.
	Skills Server = "skills"
)

// FlagPair names a CLI flag and the tool argument that carries the same input.
// A flag and an argument that are equal once hyphens become underscores need no
// pair spelled out by Arg; Flag alone is enough.
type FlagPair struct {
	Flag string
	// Arg is the tool argument; empty means the flag name with "-" as "_".
	Arg string
	// TypeNote explains a flag and an argument of different JSON types, such as a
	// comma-separated string flag and an array argument. Empty means they match.
	TypeNote string
	// EnumNote explains argument enum values the usage text of the flag omits.
	EnumNote string
}

// Exclusion names inputs that exist on one side only, with the reason.
type Exclusion struct {
	Names  []string
	Reason string
}

// Capability is one row of the table.
type Capability struct {
	ID   string
	Kind Kind
	// CLI lists command paths without the root, such as "okf validate". A path
	// ending in " *" stands for the command and every command below it.
	CLI []string
	// Mode is the flag that selects this capability on a command that does
	// several things, such as "list" on approve. It is empty otherwise.
	Mode string
	// Tool is the MCP tool name; empty for CLIOnly.
	Tool string
	// Server is the MCP surface of Tool; Authoring when empty.
	Server Server
	// Reason is required for CLIOnly and MCPOnly rows.
	Reason string
	// Flags lists the inputs both sides take. On a Paired row every other flag of
	// the command must be in CLIFlags and every other tool argument in ToolArgs.
	Flags []FlagPair
	// CLIFlags are flags of the command with no tool argument.
	CLIFlags []Exclusion
	// ToolArgs are tool arguments with no flag.
	ToolArgs []Exclusion
	// Behavior names the behavior check that runs both sides on one fixture and
	// compares their documents; empty when there is none.
	Behavior string
}

// ServerOrDefault is the capability's MCP surface.
func (c Capability) ServerOrDefault() Server {
	if c.Server == "" {
		return Authoring
	}
	return c.Server
}

// GlobalCLIFlags are flags that no tool takes, on every paired command.
func GlobalCLIFlags() []Exclusion {
	return []Exclusion{
		{Names: []string{"format"}, Reason: "a tool always answers with one JSON document"},
		{Names: []string{"yes"}, Reason: "a tool never prompts; destructive tools carry the destructive annotation"},
		{Names: []string{"config", "debug", "quiet", "token", "policy", "policy-mode", "policy-digest", "policy-offline", "policy-max-stale",
			"policy-tofu", "discover-org", "policy-require-signed", "policy-signer-key", "policy-signer-identity", "policy-signer-issuer", "policy-trusted-root"},
			Reason: "root flags of the process; the MCP server takes them once at start, and config_file is the tool-side config"},
	}
}

// GlobalToolArgs are arguments every project tool takes with no flag.
func GlobalToolArgs() []Exclusion {
	return []Exclusion{
		{Names: []string{"working_directory"}, Reason: "a tool names its project directory; a command runs in the current directory"},
		{Names: []string{"config_file"}, Reason: "the CLI takes the configuration through the root --config flag or a positional config-file"},
	}
}

// ByTool indexes capabilities by tool and server.
func ByTool(caps []Capability) map[Server]map[string]Capability {
	out := map[Server]map[string]Capability{Authoring: {}, Skills: {}}
	for i := range caps {
		if c := caps[i]; c.Tool != "" {
			out[c.ServerOrDefault()][c.Tool] = c
		}
	}
	return out
}

// Normalize turns a flag name into the tool argument spelling.
func Normalize(name string) string { return strings.ReplaceAll(name, "-", "_") }

// ArgOf is the tool argument of a flag pair.
func (p FlagPair) ArgOf() string {
	if p.Arg != "" {
		return p.Arg
	}
	return Normalize(p.Flag)
}

// Sorted returns the capabilities ordered by ID, the order the docs list them in.
func Sorted(caps []Capability) []Capability {
	out := append([]Capability(nil), caps...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
