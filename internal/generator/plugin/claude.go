package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	register(config.PluginRuntimeClaude, renderClaude)
}

// claudeManifest is the shape of .claude-plugin/plugin.json. Claude auto-discovers
// bundled skills/commands/agents by convention, so no "skills" key is emitted.
type claudeManifest struct {
	Name        string              `json:"name"`
	DisplayName string              `json:"displayName,omitempty"`
	Description string              `json:"description,omitempty"`
	Version     string              `json:"version"`
	Author      *config.Author      `json:"author,omitempty"`
	Homepage    string              `json:"homepage,omitempty"`
	Repository  string              `json:"repository,omitempty"`
	License     string              `json:"license,omitempty"`
	Keywords    []string            `json:"keywords,omitempty"`
	MCPServers  map[string]mcpEntry `json:"mcpServers,omitempty"`
}

func renderClaude(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	doc := claudeManifest{
		Name:        m.Name,
		DisplayName: m.DisplayName,
		Description: m.Description,
		Version:     m.Version,
		Author:      m.Author,
		Homepage:    m.Homepage,
		Repository:  m.Repository,
		License:     m.License,
		Keywords:    m.Keywords,
		MCPServers:  mcpServersFor(m, config.PluginRuntimeClaude),
	}

	manifest, err := jsonOutput(filepath.Join(baseDir, ".claude-plugin", "plugin.json"), doc)
	if err != nil {
		return nil, err
	}
	outputs := []config.OutputFile{manifest}

	// Claude auto-discovers skills/commands/agents from top-level directories.
	content, err := bundleContent(m, baseDir, contentLayout{Skills: true, Commands: true, Agents: true})
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, content...)

	// Root hooks/hooks.json.
	hooks, err := renderHooksFile(m, config.PluginRuntimeClaude, filepath.Join(baseDir, "hooks", "hooks.json"))
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, hooks...)

	// Claude-only status-line script passthrough.
	status, err := renderStatusline(m, baseDir)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, status...)

	return outputs, nil
}
