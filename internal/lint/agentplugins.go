package lint

import (
	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
)

// Codes of the Agent Plugins package check (docs/agent-plugins.md). They are
// reported by `validate --strict` and `publish` for a project that builds an
// Agent Plugins package (the agent-plugins runtime, or copilot and the codex
// root layout, which share it).
const (
	CodeAgentPluginManifest    = "AR9O0"
	CodeAgentPluginSkill       = "AR9O1"
	CodeAgentPluginMCP         = "AR9O2"
	CodeAgentPluginPlaceholder = "AR9O3"
	CodeAgentPluginDropped     = "AR9O4"
	CodeAgentPluginUnsafe      = "AR9O5"
)

// AgentPluginFinding is one finding of the Agent Plugins library, anchored to a
// file of the project (the plugin.json the package writes).
type AgentPluginFinding struct {
	// File is the absolute path the finding is reported against.
	File    string
	Finding agentplugins.Finding
}

// WithAgentPlugins supplies the Agent Plugins package findings to report as AR9O*.
func WithAgentPlugins(findings []AgentPluginFinding) Option {
	return func(r *runner) { r.agentPlugins = findings }
}

// checkAgentPlugins reports the package findings. Info findings record a
// lossless rewrite and are left out; the rest keep the severity the library
// gave them, which is what a conformant client does with the content.
func (r *runner) checkAgentPlugins() {
	for _, f := range r.agentPlugins {
		var sev Severity
		switch f.Finding.Severity {
		case agentplugins.SeverityError:
			sev = SeverityError
		case agentplugins.SeverityWarning:
			sev = SeverityWarning
		default:
			continue
		}
		r.addWithSeverity(agentplugins.RuleCode(f.Finding.Code), sev, f.File, 1, "%s: %s", f.Finding.Path, f.Finding.Message)
	}
}

func registerAgentPlugins(s *ruleSet) {
	for _, code := range []string{
		CodeAgentPluginManifest, CodeAgentPluginSkill, CodeAgentPluginMCP,
		CodeAgentPluginPlaceholder, CodeAgentPluginDropped, CodeAgentPluginUnsafe,
	} {
		SetAnalyzer(code, AnalyzerPlugin, ScopeBundle)
	}
	s.addRules(
		RuleInfo{CodeAgentPluginManifest, "agent-plugins-manifest-invalid", SeverityError, "plugin.json is missing or does not match the Agent Plugins schema, names an unsupported spec version, or carries an invalid extension namespace"},
		RuleInfo{CodeAgentPluginSkill, "agent-plugins-skill-invalid", SeverityError, "a skill of the Agent Plugins package breaks the Agent Skills rules (name, description, frontmatter) or sits outside skills/<name>/SKILL.md, so clients skip it"},
		RuleInfo{CodeAgentPluginMCP, "agent-plugins-mcp-invalid", SeverityError, "mcp.json or one of its servers does not match the Agent Plugins schema or section 7.2 (command, url, headers), so clients skip it"},
		RuleInfo{CodeAgentPluginPlaceholder, "agent-plugins-placeholder-unsupported", SeverityError, "an MCP server uses a ${VAR} placeholder; Agent Plugins clients expand only ${PLUGIN_ROOT} and ${PLUGIN_DATA}, in args, env values and cwd"},
		RuleInfo{CodeAgentPluginDropped, "agent-plugins-content-dropped", SeverityWarning, "a field or server the Agent Plugins package cannot carry was not packaged: a disabled server, or command, args, env and cwd on a remote server"},
		RuleInfo{CodeAgentPluginUnsafe, "agent-plugins-package-unsafe", SeverityError, "a file or link of the Agent Plugins package resolves outside the plugin root or cannot be read"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeAgentPluginManifest: {
			Why:  "A conformant client rejects a plugin whose plugin.json fails the official schema, so ai-rulez checks the file it writes against the vendored schema of the selected spec version.",
			Bad:  "`[plugin] spec = \"2.0.0\"`, or a plugin name with an upper-case letter",
			Good: "`spec = \"1.1.0\"` (or leave it unset for 1.0.0) and a name of lower-case letters, digits, `-` and `.`",
		},
		CodeAgentPluginSkill: {
			Why:  "Clients skip a skill whose SKILL.md lacks a name that matches its directory or a description, so the package would ship a skill nobody can load.",
			Bad:  "A skill whose frontmatter has `name` but no `description`",
			Good: "Give the skill a `description` and a `name` equal to its directory name",
		},
		CodeAgentPluginMCP: {
			Why:  "Clients skip an MCP server whose entry breaks the schema, and disable MCP when mcp.json itself is invalid. A stdio command must be one token (a bare name or a ./ path), and an HTTP url must be HTTPS or localhost.",
			Bad:  "`command = \"npx -y server\"` or `url = \"http://example.com/mcp\"`",
			Good: "`command = \"npx\"`, `args = [\"-y\", \"server\"]`, and an `https://` url",
		},
		CodeAgentPluginPlaceholder: {
			Why:  "Agent Plugins expands only ${PLUGIN_ROOT} and ${PLUGIN_DATA}. A ${API_KEY} placeholder would reach the server as that literal text, so ai-rulez drops the server instead of packaging a launch that cannot work.",
			Bad:  "`args = [\"--key\", \"${API_KEY}\"]` in a plugin MCP server",
			Good: "Let the server read API_KEY from its environment (an `env` entry `API_KEY = \"${API_KEY}\"` is omitted and left to the client), or use ${PLUGIN_ROOT}/${PLUGIN_DATA}",
		},
		CodeAgentPluginDropped: {
			Why:  "The package format has no way to say that a server is disabled, and a remote server has no command or env, so those fields are left out rather than packaged wrongly.",
			Bad:  "`enabled = false` on a server of a project that publishes an Agent Plugins package",
			Good: "Remove the server from the plugin, or enable it",
		},
		CodeAgentPluginUnsafe: {
			Why:  "A package must hold only files below its root, so a symlink that leaves the root, or a file that cannot be read, is reported and not packaged.",
			Bad:  "A skill directory that is a symlink to `~/skills`",
			Good: "Copy the skill into the project",
		},
	})
}
