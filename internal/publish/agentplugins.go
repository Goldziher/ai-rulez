package publish

import (
	"strings"
	"testing/fstest"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
)

// agentPluginsManifest is the root manifest of an Agent Plugins package.
const agentPluginsManifest = "plugin.json"

// agentPluginFiles is the Agent Plugins package inside a bundle: the files the
// specification defines (plugin.json, mcp.json, skills/) and the extension
// namespace directories. It is nil when the bundle has no root plugin.json, the
// mark of the agent-plugins, copilot and root-layout codex runtimes.
func agentPluginFiles(files []File) fstest.MapFS {
	var mem fstest.MapFS
	for _, f := range files {
		if f.Path == agentPluginsManifest {
			mem = fstest.MapFS{}
			break
		}
	}
	for _, f := range files {
		first, _, nested := strings.Cut(f.Path, "/")
		if mem != nil && (f.Path == agentPluginsManifest || f.Path == "mcp.json" || (nested && (first == "skills" || agentplugins.ValidNamespace(first)))) {
			mem[f.Path] = &fstest.MapFile{Data: f.Data}
		}
	}
	return mem
}

// checkAgentPlugins validates the Agent Plugins package of a bundle the way a
// conformant client loads it, so a package that clients would reject or partly
// skip is not published. A finding of error severity fails the build with the
// AR9O rule that names it; the message lists every error. An unexpanded
// placeholder counts as an error here.
func checkAgentPlugins(files []File) error {
	mem := agentPluginFiles(files)
	if mem == nil {
		return nil
	}
	res := agentplugins.Validate(mem)
	var (
		code string
		msgs []string
	)
	for _, f := range res.Findings {
		// A placeholder clients do not expand is only a warning to Validate (the
		// package is loadable), but ai-rulez never publishes one: it would reach
		// the server as literal text.
		if f.Severity != agentplugins.SeverityError && f.Code != agentplugins.CodePlaceholder {
			continue
		}
		if code == "" {
			code = agentplugins.RuleCode(f.Code)
		}
		msgs = append(msgs, agentplugins.RuleCode(f.Code)+" "+f.Path+": "+f.Message)
	}
	if len(msgs) == 0 {
		return nil
	}
	return newError(code, ExitGate, "fix the package (see docs/agent-plugins.md), or drop agent-plugins from the plugin runtimes",
		"the Agent Plugins package breaks the specification: %s", strings.Join(msgs, "; "))
}
