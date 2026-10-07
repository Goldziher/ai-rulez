// Package agentplugins builds, validates and imports Agent Plugins packages
// (https://agent-plugins.org), versions 1.0.0 and 1.1.0.
//
// A package is a directory with a root plugin.json, skills under skills/<name>/,
// MCP servers in mcp.json, and client-specific content under reverse-domain
// extension namespaces. Build turns a Plugin into the package files, Validate
// checks a package against the vendored official schemas and the discovery and
// failure-boundary rules of the specification, and Import reads a package back
// into a Plugin. The package has no dependency on the ai-rulez config loader.
//
// # Extension namespaces
//
// Agent Plugins standardizes only skills and MCP servers. Content that only one
// harness understands goes under that harness's namespace, either as manifest
// data (plugin.json extensions.<namespace>) or as files (<namespace>/). The
// namespaces ai-rulez writes:
//
//   - NamespaceClaudeCode ("com.anthropic.claude-code"): Claude Code agents,
//     commands and hooks, laid out as in a Claude Code plugin:
//     agents/<name>.md, commands/<name>.md and hooks/hooks.json. The namespace
//     is the reverse of anthropic.com, the vendor's domain, plus the product.
//   - NamespaceCodex ("com.openai"): the Codex plugin interface block, as
//     manifest data under extensions.com.openai.interface. It keeps the
//     namespace the plugin generator already writes.
//   - NamespaceAIRulez ("io.github.goldziher.ai-rulez"): ai-rulez content that
//     no single harness owns (rules/<name>.md, context/<name>.md), so an import
//     can restore it. The namespace is the reverse of goldziher.github.io, the
//     domain the project controls.
//
// Clients ignore namespaces they do not implement, so these never affect a
// conformant client's loading of the portable components.
package agentplugins

// Extension namespaces ai-rulez writes. See the package documentation.
const (
	NamespaceClaudeCode = "com.anthropic.claude-code"
	NamespaceCodex      = "com.openai"
	NamespaceAIRulez    = "io.github.goldziher.ai-rulez"
)
