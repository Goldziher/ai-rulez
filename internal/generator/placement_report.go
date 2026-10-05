package generator

import (
	"slices"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/samber/oops"
)

// placementKindCommand labels a command row of the placement report.
const placementKindCommand = "command"

// Placement destinations reported for an item.
const (
	// DestinationCore: generated into the assistants' own directories.
	DestinationCore = "core"
	// DestinationPlugin: kept out of the assistants' directories, shipped by plugins.
	DestinationPlugin = "plugin"
)

// PlacementItem is where one skill or command ends up.
type PlacementItem struct {
	Type        string   `json:"type"` // skill | command
	Name        string   `json:"name"`
	Domain      string   `json:"domain,omitempty"`
	Destination string   `json:"destination"`       // core | plugin
	Plugins     []string `json:"plugins,omitempty"` // plugins that bundle the item
	Issue       string   `json:"issue,omitempty"`   // why a plugin-only item may never reach anyone
}

// PlacementReport lists every skill and command of a profile with its
// destination, and the plugin-only items nobody can reach.
type PlacementReport struct {
	Profile string          `json:"profile"`
	Items   []PlacementItem `json:"items"`
}

// Issues returns the items that carry an issue.
func (r *PlacementReport) Issues() []PlacementItem {
	var out []PlacementItem
	for _, it := range r.Items {
		if it.Issue != "" {
			out = append(out, it)
		}
	}
	return out
}

// PlacementReport resolves, for the active profile, where every skill and
// command goes: core (the assistants' directories) or plugin-only, together
// with the plugins that bundle it. A plugin-only item is flagged when no plugin
// bundles it, or when none of the plugins that do is enabled through
// [claude.settings] and no catalog skill lists them, because then nothing tells
// anyone the plugin exists.
func (g *Generator) PlacementReport(profile string) (*PlacementReport, error) {
	active := g.resolveProfile(profile)
	tree, err := g.getContentForProfile(active)
	if err != nil {
		return nil, err
	}
	plan, err := plugin.PlanDomainPlugins(g.config, tree)
	if err != nil {
		return nil, oops.Wrapf(err, "plan domain plugins")
	}
	skillPlugins, commandPlugins := plugin.BundleMembership(g.config, tree, plan)
	enabled := g.enabledPluginNames()
	catalog := g.config.Marketplace != nil && g.config.Marketplace.CatalogSkill != nil && g.config.Marketplace.CatalogSkill.Enabled

	report := &PlacementReport{Profile: active}
	add := func(typ string, items []config.ContentFile, membership map[string][]string) {
		for i := range items {
			item := items[i]
			dest := providers.ResolvePlacement(g.config, typ, item, tree)
			row := PlacementItem{
				Type:        placementTypeLabel(typ),
				Name:        item.Name,
				Domain:      skillOwnerDomain(tree, typ, item),
				Destination: DestinationCore,
				Plugins:     membership[item.Name],
			}
			if dest == config.PlacementPlugin {
				row.Destination = DestinationPlugin
				row.Issue = placementIssue(row.Plugins, enabled, catalog)
			}
			report.Items = append(report.Items, row)
		}
	}
	add(providers.OutputTypeSkills, presets.AllSkills(tree), skillPlugins)
	add(providers.OutputTypeCommands, presets.AllCommands(tree), commandPlugins)
	sort.SliceStable(report.Items, func(i, j int) bool {
		a, b := report.Items[i], report.Items[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Name < b.Name
	})
	return report, nil
}

func placementTypeLabel(typ string) string {
	if typ == providers.OutputTypeCommands {
		return placementKindCommand
	}
	return "skill"
}

func placementIssue(plugins, enabled []string, catalog bool) string {
	if len(plugins) == 0 {
		return "no plugin bundles this item, so it is not generated anywhere"
	}
	if catalog {
		return ""
	}
	for _, p := range plugins {
		if slices.Contains(enabled, p) {
			return ""
		}
	}
	return "its plugin is not enabled through [claude.settings] enable_plugins and no [marketplace.catalog_skill] lists it"
}

// enabledPluginNames lists the plugins [claude.settings] enables.
func (g *Generator) enabledPluginNames() []string {
	if !g.config.ManagesClaudeSettings() {
		return nil
	}
	return g.config.Claude.Settings.EnablePlugins
}

// skillOwnerDomain returns the domain an item was loaded from ("" for root).
func skillOwnerDomain(tree *config.ContentTree, typ string, item config.ContentFile) string {
	names := make([]string, 0, len(tree.Domains))
	for name := range tree.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		d := tree.Domains[name]
		files := d.Skills
		if typ == providers.OutputTypeCommands {
			files = d.Commands
		}
		for i := range files {
			if item.Path != "" && files[i].Path == item.Path {
				return name
			}
		}
	}
	return ""
}
