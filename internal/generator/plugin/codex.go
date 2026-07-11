package plugin

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

func init() {
	register(config.PluginRuntimeCodex, renderCodex)
}

// codexMCPRef is the relative path Codex's plugin.json uses to reference its
// external MCP server file.
const codexMCPRef = "./.mcp.json"

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

type codexMCPDoc struct {
	MCPServers map[string]mcpEntry `json:"mcpServers"`
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
		mcpFile, err := jsonOutput(filepath.Join(baseDir, ".mcp.json"), codexMCPDoc{MCPServers: servers})
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, mcpFile)
	}

	content, err := bundleContent(m, baseDir, contentLayout{Skills: true})
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, content...)

	assets, err := bundleCodexAssets(m, baseDir)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, assets...)

	return outputs, nil
}

func bundleCodexAssets(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	if m.Interface == nil {
		return nil, nil
	}
	paths := []string{m.Interface.ComposerIcon, m.Interface.Logo, m.Interface.LogoDark}
	paths = append(paths, m.Interface.Screenshots...)
	var outputs []config.OutputFile
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(strings.TrimPrefix(path, "./"))
		if filepath.IsAbs(path) || clean == ".." || strings.HasPrefix(filepath.ToSlash(clean), "../") {
			return nil, oops.With("path", path).Errorf("Codex asset path must stay inside the plugin root")
		}
		out, err := passthroughFile(m.SourceDir, clean, filepath.Join(baseDir, clean))
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}
	return outputs, nil
}
