package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

func init() {
	register(config.PluginRuntimeCopilot, renderCopilot)
}

// copilotNamespaceDir is the vendor directory the Agent Plugins layout gives
// Copilot-specific components.
const copilotNamespaceDir = "com.github.copilot"

// renderCopilot emits a GitHub Copilot plugin in the Agent Plugins 1.0 layout
// (https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference):
// a root plugin.json carrying the standard's $schema, skills/, mcp.json, and
// custom agents as com.github.copilot/agents/<name>.agent.md. The single-plugin
// marketplace index (.github/plugin/marketplace.json) is written alongside.
//
// Commands and hooks are not emitted: the reference lists
// com.github.copilot/commands/ and com.github.copilot/hooks/hooks.json but
// documents neither file format, and ai-rulez will not guess one.
func renderCopilot(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	outputs, err := agentPluginsCore(m, baseDir)
	if err != nil {
		return nil, err
	}
	for i := range m.Agents {
		out, err := passthroughContent(&m.Agents[i],
			filepath.Join(baseDir, copilotNamespaceDir, "agents", m.Agents[i].Name+".agent.md"))
		if err != nil {
			return nil, err
		}
		outputs = appendIf(outputs, out)
	}
	if len(m.Commands) > 0 {
		logger.Warn("The copilot runtime does not bundle commands: their file format is not documented", "plugin", m.Name, "commands", len(m.Commands))
	}
	if len(m.Hooks) > 0 {
		logger.Warn("The copilot runtime does not bundle hooks: their file format is not documented", "plugin", m.Name, "hooks", len(m.Hooks))
	}
	return outputs, nil
}
