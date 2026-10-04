package providers

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/targetmatch"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

// generatedPathScheme prefixes the path of a synthetic skill (the catalog); such
// a skill is always core, since no plugin bundles it.
const generatedPathScheme = "generated://"

// placementFrontmatterKey is the frontmatter key that overrides [placement] for
// one skill or command.
const placementFrontmatterKey = "placement"

// placementAllows implements the placement_core filter: it reports whether a
// skill or command is generated into this provider's own directory.
//
// Commands always honor frontmatter `targets`; skills do only with
// `[placement] honor_targets`, because they never did before. An item is then
// dropped when its placement is "plugin" (shipped only through a plugin).
func (g *Generator) placementAllows(typ string, item config.ContentFile, content *config.ContentTree, cfg *config.Config) bool {
	honorTargets := typ == OutputTypeCommands || (cfg != nil && cfg.Placement != nil && cfg.Placement.HonorTargets)
	if honorTargets && item.Metadata != nil && !targetmatch.Allow(item.Metadata.Targets, []string{g.Spec.Name}) {
		return false
	}
	return ResolvePlacement(cfg, typ, item, content) == config.PlacementCore
}

// ResolvePlacement returns "core" or "plugin" for an item: frontmatter
// `placement`, then a [placement].core match, then a [placement].plugin match,
// then [placement].default, then core.
func ResolvePlacement(cfg *config.Config, typ string, item config.ContentFile, content *config.ContentTree) string {
	if item.Metadata != nil {
		switch v := strings.ToLower(strings.TrimSpace(item.Metadata.Extra[placementFrontmatterKey])); v {
		case config.PlacementCore, config.PlacementPlugin:
			return v
		case "":
		default:
			logger.Warn("Ignoring unknown placement in frontmatter; use core or plugin", "item", item.Path, "placement", v)
		}
	}
	if cfg == nil || cfg.Placement == nil || strings.HasPrefix(item.Path, generatedPathScheme) {
		return config.PlacementCore
	}
	p := cfg.Placement
	id := computeItemID(typ, item)
	keys := []string{id}
	if domain := originDomain(content, typ, item); domain != "" {
		keys = append(keys, "domains/"+domain+"/"+id)
	}
	switch {
	case targetmatch.Match(p.Core, nil, keys...):
		return config.PlacementCore
	case targetmatch.Match(p.Plugin, nil, keys...):
		return config.PlacementPlugin
	case p.Default == config.PlacementPlugin:
		return config.PlacementPlugin
	}
	return config.PlacementCore
}

// ItemID is the name an item is placed and written under: the skill directory,
// or the sanitized command name.
func ItemID(typ string, item config.ContentFile) string { return computeItemID(typ, item) }

// originDomain returns the domain an item was loaded from, or "" for root and
// builtin content (a builtin domain is not a domains/<name> directory).
func originDomain(content *config.ContentTree, typ string, item config.ContentFile) string {
	if content == nil {
		return ""
	}
	for name, d := range content.Domains {
		if d.Builtin {
			continue
		}
		var files []config.ContentFile
		switch typ {
		case OutputTypeSkills:
			files = d.Skills
		case OutputTypeCommands:
			files = d.Commands
		}
		for i := range files {
			if files[i].Path == item.Path && item.Path != "" {
				return name
			}
		}
	}
	return ""
}
