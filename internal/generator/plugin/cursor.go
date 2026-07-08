package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	register(config.PluginRuntimeCursor, renderCursor)
}

// cursorManifest is the shape of .cursor-plugin/plugin.json. Unlike Claude,
// Cursor references its skills directory explicitly.
type cursorManifest struct {
	Name        string              `json:"name"`
	DisplayName string              `json:"displayName,omitempty"`
	Description string              `json:"description,omitempty"`
	Version     string              `json:"version"`
	Author      *config.Author      `json:"author,omitempty"`
	Homepage    string              `json:"homepage,omitempty"`
	Repository  string              `json:"repository,omitempty"`
	License     string              `json:"license,omitempty"`
	Keywords    []string            `json:"keywords,omitempty"`
	Skills      string              `json:"skills,omitempty"`
	MCPServers  map[string]mcpEntry `json:"mcpServers,omitempty"`
}

func renderCursor(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	pluginDir := filepath.Join(baseDir, ".cursor-plugin")

	doc := cursorManifest{
		Name:        m.Name,
		DisplayName: m.DisplayName,
		Description: m.Description,
		Version:     m.Version,
		Author:      m.Author,
		Homepage:    m.Homepage,
		Repository:  m.Repository,
		License:     m.License,
		Keywords:    m.Keywords,
		Skills:      skillsDirRef(m),
		MCPServers:  mcpServersFor(m, config.PluginRuntimeCursor),
	}

	manifest, err := jsonOutput(filepath.Join(pluginDir, "plugin.json"), doc)
	if err != nil {
		return nil, err
	}
	outputs := []config.OutputFile{manifest}

	content, err := bundleContent(m, baseDir, contentLayout{Root: ".cursor-plugin", Skills: true, Commands: true})
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, content...)

	hooks, err := renderHooksFile(m, config.PluginRuntimeCursor, filepath.Join(pluginDir, "hooks", "hooks.json"))
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, hooks...)

	return outputs, nil
}

// skillsDirRef returns the "./skills/" reference used by runtimes that point at
// a bundled skills directory, or "" when the plugin has no skills.
func skillsDirRef(m *Manifest) string {
	if len(m.Skills) == 0 {
		return ""
	}
	return "./skills/"
}
