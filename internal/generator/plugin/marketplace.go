package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// marketplaceDoc is the shape of .claude-plugin/marketplace.json.
type marketplaceDoc struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Owner       *config.Author      `json:"owner,omitempty"`
	Plugins     []marketplacePlugin `json:"plugins"`
}

// marketplacePlugin is one entry in a marketplace's plugins[] array.
type marketplacePlugin struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Version     string         `json:"version,omitempty"`
	Source      string         `json:"source"`
	Category    string         `json:"category,omitempty"`
	License     string         `json:"license,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Author      *config.Author `json:"author,omitempty"`
	// DefaultEnabled and Relevance are Claude Code marketplace entry fields.
	DefaultEnabled *bool           `json:"defaultEnabled,omitempty"`
	Relevance      *relevanceEntry `json:"relevance,omitempty"`
}

// relevanceEntry is a marketplace entry's relevance object.
type relevanceEntry struct {
	Topic   string         `json:"topic,omitempty"`
	Signals relevanceSigns `json:"signals"`
}

type relevanceSigns struct {
	CWD          []string           `json:"cwd,omitempty"`
	CLI          []string           `json:"cli,omitempty"`
	Hosts        []string           `json:"hosts,omitempty"`
	FilesRead    []string           `json:"filesRead,omitempty"`
	ManifestDeps []manifestDepEntry `json:"manifestDeps,omitempty"`
}

type manifestDepEntry struct {
	File    string `json:"file"`
	Pattern string `json:"pattern"`
}

func relevanceFor(r *config.PluginRelevance) *relevanceEntry {
	signs := relevanceSigns{CWD: r.Signals.CWD, CLI: r.Signals.CLI, Hosts: r.Signals.Hosts, FilesRead: r.Signals.FilesRead}
	for _, dep := range r.Signals.ManifestDeps {
		signs.ManifestDeps = append(signs.ManifestDeps, manifestDepEntry{File: dep.File, Pattern: dep.Pattern})
	}
	return &relevanceEntry{Topic: r.Topic, Signals: signs}
}

// renderMarketplace emits .claude-plugin/marketplace.json for a single-plugin
// repo (source "./"). Monorepo marketplaces (Market.Members set) are assembled
// separately by the monorepo driver, so this returns nothing in that case.
func renderMarketplace(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	if len(m.Market.Members) > 0 {
		return nil, nil
	}

	doc := marketplaceDoc{
		Name:        m.Market.Name,
		Description: m.Market.Description,
		Owner:       m.Market.Owner,
		Plugins: []marketplacePlugin{
			pluginEntry(m, "./"),
		},
	}

	out, err := jsonOutput(filepath.Join(baseDir, ".claude-plugin", "marketplace.json"), doc)
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{out}, nil
}

// MemberEntry is one plugin in a monorepo marketplace: its name, description,
// and source directory relative to the marketplace root.
type MemberEntry struct {
	Name        string
	Description string
	Source      string
	Category    string
	// Detailed adds Version, Category, Keywords, DefaultEnabled and Relevance to
	// the Claude marketplace entry. Monorepo members keep the minimal entry.
	Detailed bool
	// Codex marks a plugin that ships a Codex bundle.
	Codex          bool
	Version        string
	Keywords       []string
	DefaultEnabled *bool
	Relevance      *config.PluginRelevance
}

const defaultCodexMarketplaceCategory = "Developer Tools"

type codexMarketplaceDoc struct {
	Name      string                    `json:"name"`
	Interface codexMarketplaceInterface `json:"interface"`
	Plugins   []codexMarketplacePlugin  `json:"plugins"`
}

type codexMarketplaceInterface struct {
	DisplayName string `json:"displayName"`
}

type codexMarketplacePlugin struct {
	Name     string                 `json:"name"`
	Source   codexMarketplaceSource `json:"source"`
	Policy   codexMarketplacePolicy `json:"policy"`
	Category string                 `json:"category"`
}

type codexMarketplaceSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type codexMarketplacePolicy struct {
	Installation   string `json:"installation"`
	Authentication string `json:"authentication"`
}

// RenderMonorepoMarketplace emits the root .claude-plugin/marketplace.json for a
// multi-plugin monorepo. Entries use the minimal {name, source, description}
// shape unless a member carries version, category, keywords, defaultEnabled or
// relevance (domain plugins do); each member ships its own full plugin manifests under its source dir.
func RenderMonorepoMarketplace(market MarketInfo, members []MemberEntry, baseDir string) (config.OutputFile, error) {
	plugins := make([]marketplacePlugin, 0, len(members))
	for i := range members {
		member := &members[i]
		entry := marketplacePlugin{
			Name:        member.Name,
			Description: member.Description,
			Source:      member.Source,
		}
		if member.Detailed {
			entry.Version = member.Version
			entry.Category = member.Category
			entry.Keywords = member.Keywords
			entry.DefaultEnabled = member.DefaultEnabled
		}
		if member.Detailed && member.Relevance != nil {
			entry.Relevance = relevanceFor(member.Relevance)
		}
		plugins = append(plugins, entry)
	}
	doc := marketplaceDoc{
		Name:        market.Name,
		Description: market.Description,
		Owner:       market.Owner,
		Plugins:     plugins,
	}
	return jsonOutput(filepath.Join(baseDir, ".claude-plugin", "marketplace.json"), doc)
}

// RenderCodexMonorepoMarketplace emits the repository-local Codex marketplace.
func RenderCodexMonorepoMarketplace(
	market MarketInfo,
	members []MemberEntry,
	baseDir string,
) (config.OutputFile, error) {
	plugins := make([]codexMarketplacePlugin, 0, len(members))
	for i := range members {
		member := &members[i]
		category := member.Category
		if category == "" {
			category = defaultCodexMarketplaceCategory
		}
		plugins = append(plugins, codexMarketplacePlugin{
			Name: member.Name,
			Source: codexMarketplaceSource{
				Source: "local",
				Path:   member.Source,
			},
			Policy: codexMarketplacePolicy{
				Installation:   "AVAILABLE",
				Authentication: "ON_INSTALL",
			},
			Category: category,
		})
	}
	doc := codexMarketplaceDoc{
		Name:      market.Name,
		Interface: codexMarketplaceInterface{DisplayName: market.Name},
		Plugins:   plugins,
	}
	return jsonOutput(filepath.Join(baseDir, ".agents", "plugins", "marketplace.json"), doc)
}

// pluginEntry builds a marketplace plugins[] entry for m at the given source path.
func pluginEntry(m *Manifest, source string) marketplacePlugin {
	return marketplacePlugin{
		Name:        m.Name,
		Description: m.Description,
		Version:     m.Version,
		Source:      source,
		Category:    m.Category,
		License:     m.License,
		Homepage:    m.Homepage,
		Keywords:    m.Keywords,
		Tags:        m.Tags,
		Author:      m.Author,
	}
}
