package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	register(config.PluginRuntimeFactory, renderFactory)
}

// factoryManifest is the shape of .factory-plugin/plugin.json. Factory is purely
// metadata-driven: no MCP servers, skills, or interface block.
type factoryManifest struct {
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Description string         `json:"description,omitempty"`
	Author      *config.Author `json:"author,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Repository  string         `json:"repository,omitempty"`
	License     string         `json:"license,omitempty"`
	Category    string         `json:"category,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	BrandColor  string         `json:"brandColor,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	Logo        string         `json:"logo,omitempty"`
}

func renderFactory(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	doc := factoryManifest{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Homepage:    m.Homepage,
		Repository:  m.Repository,
		License:     m.License,
		Category:    m.Category,
		Keywords:    m.Keywords,
		BrandColor:  m.BrandColor,
		Icon:        m.Icon,
		Logo:        m.Logo,
	}

	out, err := jsonOutput(filepath.Join(baseDir, ".factory-plugin", "plugin.json"), doc)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{out}, nil
}
