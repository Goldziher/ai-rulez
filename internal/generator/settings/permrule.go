package settings

import (
	"fmt"
	"regexp"
	"strings"
)

// ToolKind groups Claude Code tools by the capability a permission rule
// governs, which is what the other harnesses' permission surfaces are keyed on.
type ToolKind string

// Tool kinds of a parsed Claude permission rule.
const (
	KindShell  ToolKind = "shell"  // Bash
	KindRead   ToolKind = "read"   // Read
	KindEdit   ToolKind = "edit"   // Edit, Write, MultiEdit, NotebookEdit
	KindFetch  ToolKind = "fetch"  // WebFetch
	KindSearch ToolKind = "search" // WebSearch
	KindMCP    ToolKind = "mcp"    // mcp__server__tool
	KindAgent  ToolKind = "agent"  // Task, Agent (subagent launch)
	KindOther  ToolKind = "other"  // any tool without a cross-harness equivalent
)

// PathAnchor says what a path specifier is relative to, following Claude Code's
// path rule syntax.
type PathAnchor string

// Path anchors: `./x` and `x` are relative to the working directory, `/x` to the
// project root, `//x` is absolute and `~/x` is relative to the home directory.
const (
	AnchorCwd      PathAnchor = "cwd"
	AnchorProject  PathAnchor = "project"
	AnchorAbsolute PathAnchor = "absolute"
	AnchorHome     PathAnchor = "home"
)

// PathPattern is the specifier of a Read, Edit or Write rule with its anchor
// resolved. Glob never carries the anchor's own prefix (`./`, `/`, `~/`); an
// absolute path keeps its leading slash.
type PathPattern struct {
	Anchor PathAnchor
	Glob   string
}

// Rule is one Claude Code permission rule ("Bash(npm run test:*)",
// "Read(./.env)", "WebFetch(domain:example.com)", "mcp__github__create_issue")
// split into the tool and its specifier. It is the shared input of the
// per-harness permission translators.
type Rule struct {
	// Raw is the rule as written, trimmed.
	Raw string
	// Tool is the Claude tool name, or "mcp" for an MCP rule.
	Tool string
	Kind ToolKind
	// Specifier is the text between the parentheses; empty for a bare rule.
	Specifier string
	// Bare is true when the rule names a tool without a specifier, so it covers
	// every call of the tool.
	Bare bool

	// Command is a Bash specifier without the legacy `:*` suffix; Prefix is true
	// when that suffix was present.
	Command string
	Prefix  bool
	// Path is the resolved specifier of a Read or Edit rule.
	Path PathPattern
	// Domain is the host of a WebFetch(domain:...) rule.
	Domain string
	// Server and MCPTool name an MCP rule. MCPTool is empty when the rule covers
	// every tool of the server (`mcp__server` and `mcp__server__*`).
	Server  string
	MCPTool string
}

var toolName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

const (
	mcpPrefix    = "mcp__"
	prefixSuffix = ":*"
	domainPrefix = "domain:"
)

// toolKinds maps the Claude tool names that have a cross-harness meaning.
var toolKinds = map[string]ToolKind{
	"Bash":         KindShell,
	"Read":         KindRead,
	"Edit":         KindEdit,
	"Write":        KindEdit,
	"MultiEdit":    KindEdit,
	"NotebookEdit": KindEdit,
	"WebFetch":     KindFetch,
	"WebSearch":    KindSearch,
	"Task":         KindAgent,
	"Agent":        KindAgent,
}

// ParseRule parses a Claude Code permission rule: `Tool`, `Tool(specifier)` or
// `mcp__server[__tool]`. Specifier text is kept verbatim apart from the typed
// fields derived from it (Command, Path, Domain).
func ParseRule(raw string) (Rule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Rule{}, fmt.Errorf("empty permission rule")
	}
	name, spec, hasSpec, err := splitRule(raw)
	if err != nil {
		return Rule{}, err
	}
	if strings.HasPrefix(name, mcpPrefix) {
		if hasSpec {
			return Rule{}, fmt.Errorf("permission rule %q: an MCP rule takes no specifier", raw)
		}
		return parseMCPRule(raw, name)
	}
	if !toolName.MatchString(name) {
		return Rule{}, fmt.Errorf("permission rule %q: %q is not a tool name", raw, name)
	}
	kind, ok := toolKinds[name]
	if !ok {
		kind = KindOther
	}
	rule := Rule{Raw: raw, Tool: name, Kind: kind, Specifier: spec, Bare: !hasSpec}
	switch kind {
	case KindShell:
		if hasSpec {
			rule.Command, rule.Prefix = strings.CutSuffix(spec, prefixSuffix)
		}
	case KindRead, KindEdit:
		if hasSpec {
			rule.Path = parsePath(spec)
		}
	case KindFetch:
		if domain, ok := strings.CutPrefix(spec, domainPrefix); ok {
			rule.Domain = strings.ToLower(strings.TrimSpace(domain)) // hostnames are case-insensitive; the patterns of most harnesses are not
		}
	}
	return rule, nil
}

// splitRule separates the tool name from the parenthesised specifier.
func splitRule(raw string) (name, spec string, hasSpec bool, err error) {
	open := strings.IndexByte(raw, '(')
	if open < 0 {
		if strings.ContainsAny(raw, ")") {
			return "", "", false, fmt.Errorf("permission rule %q: unbalanced parenthesis", raw)
		}
		return raw, "", false, nil
	}
	if !strings.HasSuffix(raw, ")") {
		return "", "", false, fmt.Errorf("permission rule %q: expected the rule to end with ')'", raw)
	}
	name = raw[:open]
	if name == "" {
		return "", "", false, fmt.Errorf("permission rule %q: missing tool name", raw)
	}
	spec = strings.TrimSpace(raw[open+1 : len(raw)-1])
	if spec == "" {
		return "", "", false, fmt.Errorf("permission rule %q: empty specifier (write the bare tool name to cover every call)", raw)
	}
	return name, spec, true, nil
}

func parseMCPRule(raw, name string) (Rule, error) {
	rest := strings.TrimPrefix(name, mcpPrefix)
	server, tool, _ := strings.Cut(rest, "__")
	if server == "" {
		return Rule{}, fmt.Errorf("permission rule %q: missing MCP server name", raw)
	}
	if tool == "*" {
		tool = ""
	}
	return Rule{Raw: raw, Tool: "mcp", Kind: KindMCP, Bare: true, Server: server, MCPTool: tool}, nil
}

func parsePath(spec string) PathPattern {
	switch {
	case strings.HasPrefix(spec, "//"):
		return PathPattern{Anchor: AnchorAbsolute, Glob: spec[1:]}
	case strings.HasPrefix(spec, "~/"):
		return PathPattern{Anchor: AnchorHome, Glob: spec[2:]}
	case spec == "~":
		return PathPattern{Anchor: AnchorHome}
	case strings.HasPrefix(spec, "/"):
		return PathPattern{Anchor: AnchorProject, Glob: spec[1:]}
	case strings.HasPrefix(spec, "./"):
		return PathPattern{Anchor: AnchorCwd, Glob: spec[2:]}
	}
	return PathPattern{Anchor: AnchorCwd, Glob: spec}
}

// ShellKind classifies a Bash rule's command pattern.
type ShellKind string

// Shell pattern kinds.
const (
	// ShellAny matches every command (bare Bash, Bash(*), Bash(:*)).
	ShellAny ShellKind = "any"
	// ShellPrefix matches the literal command and the same command with any
	// arguments (Bash(git *), Bash(npm run test:*)).
	ShellPrefix ShellKind = "prefix"
	// ShellExact matches exactly the literal command.
	ShellExact ShellKind = "exact"
	// ShellGlob is any other wildcard pattern; Literal holds it verbatim.
	ShellGlob ShellKind = "glob"
)

// ShellPattern is the normalised command pattern of a Bash rule.
type ShellPattern struct {
	Kind    ShellKind
	Literal string
}

// Shell normalises the command pattern of a KindShell rule. `X:*` and `X *`
// (X free of wildcards) both mean "X with any arguments": the legacy suffix and
// the space-star form are the same rule.
func (r Rule) Shell() ShellPattern {
	cmd := r.Command
	if r.Bare || strings.Trim(cmd, "* ") == "" {
		return ShellPattern{Kind: ShellAny}
	}
	if r.Prefix {
		if !strings.Contains(cmd, "*") {
			return ShellPattern{Kind: ShellPrefix, Literal: cmd}
		}
		return ShellPattern{Kind: ShellGlob, Literal: cmd + "*"}
	}
	if head, ok := strings.CutSuffix(cmd, " *"); ok && !strings.Contains(head, "*") && head != "" {
		return ShellPattern{Kind: ShellPrefix, Literal: head}
	}
	if strings.Contains(cmd, "*") {
		return ShellPattern{Kind: ShellGlob, Literal: cmd}
	}
	return ShellPattern{Kind: ShellExact, Literal: cmd}
}
