package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	register(config.PluginRuntimeGemini, renderGemini)
}

// defaultGeminiContextFile is the context filename Gemini loads when the plugin
// does not override it.
const defaultGeminiContextFile = "GEMINI.md"

// geminiManifest is the shape of gemini-extension.json. Gemini uses inline MCP
// servers (with ${extensionPath}) and an inline hooks block, and points at a
// markdown context file rather than a skills directory.
type geminiManifest struct {
	Name            string                    `json:"name"`
	Version         string                    `json:"version"`
	Description     string                    `json:"description,omitempty"`
	ContextFileName string                    `json:"contextFileName,omitempty"`
	MCPServers      map[string]mcpEntry       `json:"mcpServers,omitempty"`
	Hooks           map[string][]hookGroupDoc `json:"hooks,omitempty"`
}

func renderGemini(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	contextFile := defaultGeminiContextFile
	if m.Gemini != nil && m.Gemini.ContextFileName != "" {
		contextFile = m.Gemini.ContextFileName
	}

	doc := geminiManifest{
		Name:            m.Name,
		Version:         m.Version,
		Description:     m.Description,
		ContextFileName: contextFile,
		MCPServers:      mcpServersFor(m, config.PluginRuntimeGemini),
		Hooks:           hooksBlock(m, config.PluginRuntimeGemini),
	}

	out, err := jsonOutput(filepath.Join(baseDir, "gemini-extension.json"), doc)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{out}, nil
}
