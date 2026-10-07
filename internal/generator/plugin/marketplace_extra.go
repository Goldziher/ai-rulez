package plugin

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// cursorMarketplaceDoc is .cursor-plugin/marketplace.json as documented at
// https://cursor.com/docs/reference/plugins: name and owner.name are required,
// metadata is optional, and each plugin entry needs a name and names its
// directory in source.
type cursorMarketplaceDoc struct {
	Name     string              `json:"name"`
	Owner    config.Author       `json:"owner"`
	Metadata *marketplaceMeta    `json:"metadata,omitempty"`
	Plugins  []marketplacePlugin `json:"plugins"`
}

// marketplaceMeta is the optional metadata object Cursor and Copilot share.
type marketplaceMeta struct {
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

// copilotMarketplaceDoc is the marketplace.json Copilot CLI reads: name, owner
// and plugins are required, metadata is optional.
type copilotMarketplaceDoc = cursorMarketplaceDoc

// marketplaceOwner returns the owner both formats require, falling back to the
// marketplace name (with a warning) when none is configured.
func marketplaceOwner(log logger.Logger, market MarketInfo, format string) config.Author {
	if market.Owner != nil && market.Owner.Name != "" {
		return *market.Owner
	}
	logger.Or(log).Warn("The marketplace has no owner; using the marketplace name. Set [marketplace.owner] or [plugin.author]",
		"format", format, "marketplace", market.Name)
	return config.Author{Name: market.Name}
}

// relativePluginSource turns a marketplace source ("./plugins/x", "./") into the
// plain relative path Cursor documents ("plugins/x", ".").
func relativePluginSource(source string) string {
	s := strings.TrimPrefix(filepath.ToSlash(source), "./")
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return "."
	}
	return s
}

// RenderCursorMarketplace emits .cursor-plugin/marketplace.json for members
// and domain plugins.
func RenderCursorMarketplace(log logger.Logger, market MarketInfo, members []MemberEntry, baseDir string) (config.OutputFile, error) {
	plugins := make([]marketplacePlugin, 0, len(members))
	for i := range members {
		member := &members[i]
		entry := marketplacePlugin{Name: member.Name, Description: member.Description, Source: relativePluginSource(member.Source)}
		if member.Detailed {
			entry.Version, entry.Category, entry.Keywords = member.Version, member.Category, member.Keywords
		}
		plugins = append(plugins, entry)
	}
	return jsonOutput(filepath.Join(baseDir, ".cursor-plugin", "marketplace.json"), cursorMarketplaceDoc{
		Name:     market.Name,
		Owner:    marketplaceOwner(log, market, "cursor"),
		Metadata: metaFor(market.Description, ""),
		Plugins:  plugins,
	})
}

func metaFor(description, version string) *marketplaceMeta {
	if description == "" && version == "" {
		return nil
	}
	return &marketplaceMeta{Description: description, Version: version}
}

// renderSingleMarketplaces emits the per-runtime marketplace indexes for a
// single-plugin repository (source "./"), beyond the Claude one: the Codex and
// Cursor indexes when their [plugin.*] switches ask for them, and the Copilot
// index with the copilot runtime.
func renderSingleMarketplaces(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	if len(m.Market.Members) > 0 {
		return nil, nil
	}
	var outputs []config.OutputFile
	if slices.Contains(m.Runtimes, config.PluginRuntimeCodex) && m.Codex != nil && m.Codex.Marketplace {
		out, err := RenderCodexMonorepoMarketplace(m.Market, []MemberEntry{{
			Name: m.Name, Source: "./", Category: m.Category,
		}}, baseDir)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}
	if slices.Contains(m.Runtimes, config.PluginRuntimeCursor) && m.Cursor != nil && m.Cursor.Marketplace {
		out, err := jsonOutput(filepath.Join(baseDir, ".cursor-plugin", "marketplace.json"), cursorMarketplaceDoc{
			Name:     m.Market.Name,
			Owner:    marketplaceOwner(m.log(), m.Market, "cursor"),
			Metadata: metaFor(m.Market.Description, ""),
			Plugins:  []marketplacePlugin{pluginEntry(m, ".")},
		})
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}
	if slices.Contains(m.Runtimes, config.PluginRuntimeCopilot) {
		out, err := jsonOutput(filepath.Join(baseDir, ".github", "plugin", "marketplace.json"), copilotMarketplaceDoc{
			Name:     m.Market.Name,
			Owner:    marketplaceOwner(m.log(), m.Market, "copilot"),
			Metadata: metaFor(m.Market.Description, ""),
			Plugins:  []marketplacePlugin{pluginEntry(m, "./")},
		})
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}
	return outputs, nil
}
