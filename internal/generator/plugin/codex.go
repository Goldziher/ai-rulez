package plugin

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/samber/oops"
)

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

// mergedMCPFile renders the mcpServers key of the .mcp.json at path, merging into
// the document already there.
func mergedMCPFile(path string, servers map[string]mcpEntry) (config.OutputFile, error) {
	result, err := jsonmerge.Apply(path, []jsonmerge.OwnedKey{{Name: "mcpServers", Value: servers, Members: true}})
	if err != nil {
		return config.OutputFile{}, oops.With("path", path).Wrapf(err, "merge MCP servers into .mcp.json")
	}
	return config.OutputFile{
		Path:           path,
		RawContent:     []byte(result.Body),
		PartiallyOwned: result.PartiallyOwned,
		MergeClaims:    result.Claims,
	}, nil
}

// codexRootLayout reports whether the Codex runtime writes the Agent Plugins
// root plugin.json (manifest = "root" or "both").
func codexRootLayout(m *Manifest) bool {
	return slices.Contains(m.Runtimes, config.PluginRuntimeCodex) && m.Codex.ManifestLayout() != config.CodexManifestLegacy
}

func renderCodex(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	layout := m.Codex.ManifestLayout()
	if layout == config.CodexManifestLegacy {
		return renderCodexLegacy(m, baseDir)
	}
	root, err := agentPluginsCore(m, baseDir)
	if err != nil {
		return nil, err
	}
	assets, err := bundleCodexAssets(m, baseDir)
	if err != nil {
		return nil, err
	}
	root = append(root, assets...)
	if layout == config.CodexManifestRoot {
		return root, nil
	}
	legacy, err := renderCodexLegacy(m, baseDir)
	if err != nil {
		return nil, err
	}
	return append(root, legacy...), nil
}

func renderCodexLegacy(m *Manifest, baseDir string) ([]config.OutputFile, error) {
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

	// External MCP server file: the {name: {command, args}} map, with the
	// canonical ${PLUGIN_ROOT} preserved (Codex resolves it natively). A .mcp.json
	// the author already keeps in the repository is merged into server by server,
	// like every other writer of that file, so their own servers and keys stay.
	if servers := mcpServersFor(m, config.PluginRuntimeCodex); servers != nil {
		mcpFile, err := mergedMCPFile(filepath.Join(baseDir, ".mcp.json"), servers)
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
