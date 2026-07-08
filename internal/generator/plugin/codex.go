package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	register(config.PluginRuntimeCodex, renderCodex)
}

// codexMCPRef is the relative path Codex's plugin.json uses to reference its
// external MCP server file.
const codexMCPRef = "./.codex-plugin/.mcp.json"

// codexManifest is the shape of .codex-plugin/plugin.json. Codex references its
// MCP servers via an external file (mcpServers is a string path) and carries a
// rich interface{} UI block.
type codexManifest struct {
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Description string         `json:"description,omitempty"`
	Author      *config.Author `json:"author,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Repository  string         `json:"repository,omitempty"`
	License     string         `json:"license,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	Skills      string         `json:"skills,omitempty"`
	MCPServers  string         `json:"mcpServers,omitempty"`
	Interface   *interfaceDoc  `json:"interface,omitempty"`
}

func renderCodex(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	pluginDir := filepath.Join(baseDir, ".codex-plugin")

	var mcpRef string
	if len(m.MCP) > 0 {
		mcpRef = codexMCPRef
	}

	doc := codexManifest{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Homepage:    m.Homepage,
		Repository:  m.Repository,
		License:     m.License,
		Keywords:    m.Keywords,
		Skills:      skillsDirRef(m),
		MCPServers:  mcpRef,
		Interface:   buildInterface(m.Interface),
	}

	manifest, err := jsonOutput(filepath.Join(pluginDir, "plugin.json"), doc)
	if err != nil {
		return nil, err
	}
	outputs := []config.OutputFile{manifest}

	// External MCP server file: the bare {name: {command, args}} map, with the
	// canonical ${PLUGIN_ROOT} preserved (Codex resolves it natively).
	if servers := mcpServersFor(m, config.PluginRuntimeCodex); servers != nil {
		mcpFile, err := jsonOutput(filepath.Join(pluginDir, ".mcp.json"), servers)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, mcpFile)
	}

	content, err := bundleContent(m, baseDir, contentLayout{Root: ".codex-plugin", Skills: true, Commands: true, Agents: true})
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, content...)

	return outputs, nil
}
