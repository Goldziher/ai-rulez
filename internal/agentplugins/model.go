package agentplugins

import (
	"cmp"
	"slices"
)

// Plugin is one Agent Plugins package.
type Plugin struct {
	Metadata   Metadata
	Skills     []Skill
	MCPServers []MCPServer
	Extensions []Extension
	// Files are other files in the plugin root, keyed by slash path: a LICENSE,
	// or an executable a plugin-relative stdio command runs (bin/server). They
	// cannot live under skills/, at plugin.json or mcp.json, or in an extension
	// namespace directory.
	Files map[string][]byte
}

// Metadata is the portable plugin.json metadata (§5.3, §5.4).
type Metadata struct {
	Name        string
	Version     string
	Description string
	Author      *Author
	Homepage    string
	Repository  string
	License     string
	Keywords    []string
}

// Author is the plugin.json author object.
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Skill is one Agent Skills skill, written to skills/<Name>/.
type Skill struct {
	Name    string
	SkillMD []byte
	// Files are the skill's other files (scripts/, references/, assets/),
	// keyed by slash path relative to the skill directory.
	Files map[string][]byte
}

// MCP transports. TransportHTTP is ai-rulez's name for Streamable HTTP.
const (
	TransportStdio          = "stdio"
	TransportHTTP           = "http"
	TransportStreamableHTTP = "streamable-http"
	TransportSSE            = "sse"
)

// MCPServer mirrors the fields of an ai-rulez [[mcp_servers]] entry that
// Agent Plugins can carry, plus Cwd, which the specification adds.
type MCPServer struct {
	Name        string
	Description string
	Command     string
	Args        []string
	Env         map[string]string
	// Transport is stdio, http (Streamable HTTP), streamable-http or sse. Empty
	// means stdio, or Streamable HTTP when only URL is set.
	Transport string
	URL       string
	Headers   map[string]string
	Enabled   *bool
	Cwd       string
}

// Extension is the content of one client extension namespace.
type Extension struct {
	// Namespace is a reverse-domain identifier, e.g. com.anthropic.claude-code.
	Namespace string
	// Manifest is written as plugin.json extensions.<Namespace>; nil writes none.
	Manifest map[string]any
	// Files are written under <Namespace>/, keyed by slash path.
	Files map[string][]byte
}

// Severity grades a Finding.
type Severity string

// Severities. An error is content a conformant client rejects or skips (or,
// from Build, content that was dropped); a warning is valid but likely not what
// the author meant; info records a lossless rewrite.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Finding codes. They are stable identifiers callers map to their own codes.
const (
	CodeManifestMissing      = "manifest-missing"
	CodeManifestInvalid      = "manifest-invalid"
	CodeManifestUnknownField = "manifest-unknown-field"
	CodeUnsupportedSpec      = "unsupported-spec"
	CodeExtensionsInvalid    = "extensions-invalid"
	CodeNamespaceInvalid     = "namespace-invalid"
	CodeSkillsLocation       = "skills-location-invalid"
	CodeSkillMissing         = "skill-md-missing"
	CodeSkillInvalid         = "skill-invalid"
	CodeSkillUnknownField    = "skill-unknown-field"
	CodeMCPInvalid           = "mcp-invalid"
	CodeMCPSpecMismatch      = "mcp-spec-mismatch"
	CodeServerInvalid        = "mcp-server-invalid"
	CodeServerDisabled       = "mcp-server-disabled"
	CodeCommandNotBundled    = "command-not-bundled"
	CodePlaceholder          = "placeholder-unsupported"
	CodePlaceholderRewritten = "placeholder-rewritten"
	CodeCredentialHeader     = "credential-header"
	CodeFieldIgnored         = "field-ignored"
	CodePathEscape           = "path-escape"
	CodeFileTooLarge         = "file-too-large"
	CodeUnreadable           = "unreadable"
)

// Finding is one problem or rewrite. Path is the package path it concerns, with
// a JSON pointer fragment for an MCP server (mcp.json#/mcpServers/<name>).
type Finding struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Message  string   `json:"message"`
}

func sortFindings(fs []Finding) {
	slices.SortStableFunc(fs, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Message, b.Message))
	})
}

// HasErrors reports whether any finding is an error.
func HasErrors(fs []Finding) bool {
	return slices.ContainsFunc(fs, func(f Finding) bool { return f.Severity == SeverityError })
}
