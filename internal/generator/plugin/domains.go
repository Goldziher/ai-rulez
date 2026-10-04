package plugin

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/targetmatch"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

const (
	// DomainPluginsDir is the directory under the marketplace output root that
	// holds one subdirectory per generated domain plugin.
	DomainPluginsDir = "plugins"

	defaultDomainPluginVersion = "1.0.0"
)

var domainPluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// PlannedPlugin is one plugin generated from the domain tree, resolved against
// the [marketplace] block and the [plugin] defaults.
type PlannedPlugin struct {
	Name           string
	Description    string
	Version        string
	Category       string
	Keywords       []string
	DefaultEnabled *bool
	Runtimes       []string
	Relevance      *config.PluginRelevance
	// Domains lists the domains the plugin draws content from.
	Domains  []string
	Skills   []config.ContentFile
	Commands []config.ContentFile
	Agents   []config.ContentFile
}

// Source is the plugin's marketplace source, relative to the marketplace root.
func (p *PlannedPlugin) Source() string {
	return "./" + DomainPluginsDir + "/" + p.Name
}

// Dir is the plugin's directory under the marketplace output root.
func (p *PlannedPlugin) Dir(outputRoot string) string {
	return filepath.Join(outputRoot, DomainPluginsDir, p.Name)
}

// ContentCount is the number of bundled skills, commands and agents.
func (p *PlannedPlugin) ContentCount() int {
	return len(p.Skills) + len(p.Commands) + len(p.Agents)
}

// matchesAny reports whether name matches one of the patterns (name or glob).
func matchesAny(patterns []string, names ...string) bool {
	return targetmatch.Match(patterns, nil, names...)
}

func sortedDomainNames(tree *config.ContentTree) []string {
	names := make([]string, 0, len(tree.Domains))
	for name := range tree.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// bundleable drops content that cannot be copied into a bundle: builtin content
// has no source file.
func bundleable(files []config.ContentFile) []config.ContentFile {
	out := make([]config.ContentFile, 0, len(files))
	for i := range files {
		if strings.HasPrefix(files[i].Path, "builtin://") {
			continue
		}
		out = append(out, files[i])
	}
	return out
}

// mergeByName appends files to dst, skipping names already present so the
// earlier source wins.
func mergeByName(dst []config.ContentFile, seen map[string]bool, files []config.ContentFile) []config.ContentFile {
	for i := range files {
		if seen[files[i].Name] {
			continue
		}
		seen[files[i].Name] = true
		dst = append(dst, files[i])
	}
	return dst
}

// includeDomainContent returns root content followed by the content of every
// domain matching patterns (domains in name order). A root item wins over a
// same-named domain item, and an earlier domain over a later one.
func includeDomainContent(tree *config.ContentTree, patterns []string) (skills, commands, agents []config.ContentFile) {
	skills = append(skills, tree.Skills...)
	commands = append(commands, tree.Commands...)
	agents = append(agents, tree.Agents...)
	if len(patterns) == 0 {
		return skills, commands, agents
	}
	seenS, seenC, seenA := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range tree.Skills {
		seenS[f.Name] = true
	}
	for _, f := range tree.Commands {
		seenC[f.Name] = true
	}
	for _, f := range tree.Agents {
		seenA[f.Name] = true
	}
	for _, name := range sortedDomainNames(tree) {
		if !matchesAny(patterns, name) || tree.Domains[name].Builtin {
			continue
		}
		d := tree.Domains[name]
		skills = mergeByName(skills, seenS, bundleable(d.Skills))
		commands = mergeByName(commands, seenC, bundleable(d.Commands))
		agents = mergeByName(agents, seenA, bundleable(d.Agents))
	}
	return skills, commands, agents
}

// PlanDomainPlugins resolves the plugins the [marketplace] block generates from
// tree: one per selected domain (from_domains), then the hand-declared ones,
// where a declared plugin replaces a derived one of the same name. The result
// is sorted by name. Plugins without bundleable content are skipped with a
// warning.
func PlanDomainPlugins(cfg *config.Config, tree *config.ContentTree) ([]PlannedPlugin, error) {
	mkt := cfg.Marketplace
	if !mkt.HasDomainPlugins() || tree == nil {
		return nil, nil
	}
	byName := map[string]PlannedPlugin{}

	if fd := mkt.FromDomains; fd.IsEnabled() {
		for _, domain := range sortedDomainNames(tree) {
			if len(fd.Include) > 0 && !matchesAny(fd.Include, domain) {
				continue
			}
			if matchesAny(fd.Exclude, domain) {
				continue
			}
			d := tree.Domains[domain]
			if d.Builtin {
				continue
			}
			name := fd.NamePrefix + strings.ToLower(domain)
			if !domainPluginName.MatchString(name) {
				return nil, oops.With("domain", domain).With("plugin", name).
					Hint("Rename the domain directory or exclude it with from_domains.exclude").
					Errorf("domain %q does not form a valid plugin name %q", domain, name)
			}
			p := PlannedPlugin{
				Name:        name,
				Description: fmt.Sprintf("Skills, commands and agents of the %s domain.", domain),
				Domains:     []string{domain},
				Skills:      bundleable(d.Skills),
				Commands:    bundleable(d.Commands),
				Agents:      bundleable(d.Agents),
			}
			applyDefaults(&p, &fd.PluginDefaults, cfg)
			byName[name] = p
		}
	}

	for i := range mkt.Plugins {
		p, err := planDeclared(cfg, tree, &mkt.Plugins[i])
		if err != nil {
			return nil, err
		}
		byName[p.Name] = p
	}

	plan := make([]PlannedPlugin, 0, len(byName))
	for name := range byName {
		p := byName[name]
		if p.ContentCount() == 0 {
			logger.Warn("Skipping domain plugin without bundleable content", "plugin", p.Name)
			continue
		}
		plan = append(plan, p)
	}
	sort.Slice(plan, func(i, j int) bool { return plan[i].Name < plan[j].Name })
	return plan, nil
}

func planDeclared(cfg *config.Config, tree *config.ContentTree, decl *config.MarketplacePlugin) (PlannedPlugin, error) {
	p := PlannedPlugin{Name: decl.Name, Description: decl.Description, Relevance: decl.Relevance}
	seenS, seenC, seenA := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, domain := range sortedDomainNames(tree) {
		if !matchesAny(decl.Domains, domain) {
			continue
		}
		d := tree.Domains[domain]
		p.Domains = append(p.Domains, domain)
		p.Skills = mergeByName(p.Skills, seenS, bundleable(d.Skills))
		p.Commands = mergeByName(p.Commands, seenC, bundleable(d.Commands))
		p.Agents = mergeByName(p.Agents, seenA, bundleable(d.Agents))
	}
	if len(decl.Domains) > 0 && len(p.Domains) == 0 {
		logger.Warn("Marketplace plugin names domains that do not exist", "plugin", decl.Name, "domains", decl.Domains)
	}
	p.Skills = mergeByName(p.Skills, seenS, pickByName(bundleable(tree.Skills), decl.Skills))
	p.Commands = mergeByName(p.Commands, seenC, pickByName(bundleable(tree.Commands), decl.Commands))
	p.Agents = mergeByName(p.Agents, seenA, pickByName(bundleable(tree.Agents), decl.Agents))
	if p.Description == "" {
		p.Description = fmt.Sprintf("Skills, commands and agents for %s.", decl.Name)
	}
	applyDefaults(&p, &decl.PluginDefaults, cfg)
	return p, nil
}

func pickByName(files []config.ContentFile, patterns []string) []config.ContentFile {
	if len(patterns) == 0 {
		return nil
	}
	var out []config.ContentFile
	for i := range files {
		if matchesAny(patterns, files[i].Name) {
			out = append(out, files[i])
		}
	}
	return out
}

// applyDefaults fills version, category, keywords, default_enabled and runtimes
// from the given defaults, then from the [plugin] block.
func applyDefaults(p *PlannedPlugin, d *config.PluginDefaults, cfg *config.Config) {
	root := cfg.Plugin
	p.Version = firstNonEmpty(d.Version, rootString(root, func(r *config.PluginAuthoring) string { return r.Version }), defaultDomainPluginVersion)
	p.Category = firstNonEmpty(d.Category, rootString(root, func(r *config.PluginAuthoring) string { return r.Category }))
	p.Keywords = d.Keywords
	if len(p.Keywords) == 0 && root != nil {
		p.Keywords = root.Keywords
	}
	p.DefaultEnabled = d.DefaultEnabled
	switch {
	case len(d.Runtimes) > 0:
		p.Runtimes = d.Runtimes
	case root != nil:
		p.Runtimes = root.ResolvedRuntimes()
	default:
		p.Runtimes = config.AllPluginRuntimes
	}
}

func rootString(root *config.PluginAuthoring, get func(*config.PluginAuthoring) string) string {
	if root == nil {
		return ""
	}
	return get(root)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// BuildDomainManifest builds the Manifest of one planned plugin. Author,
// license, homepage and repository come from the [plugin] block when present.
// The plugin bundles no MCP servers or hooks.
func BuildDomainManifest(cfg *config.Config, p *PlannedPlugin) *Manifest {
	m := &Manifest{
		Name:        p.Name,
		Description: p.Description,
		Version:     p.Version,
		Category:    p.Category,
		Keywords:    p.Keywords,
		Runtimes:    p.Runtimes,
		Skills:      p.Skills,
		Commands:    p.Commands,
		Agents:      p.Agents,
		Market:      ResolveMarketInfo(cfg.Marketplace),
		Config:      cfg,
		SourceDir:   cfg.BaseDir,
	}
	if root := cfg.Plugin; root != nil {
		m.Author = root.Author
		m.Homepage = root.Homepage
		m.Repository = root.Repository
		m.License = root.License
	}
	if m.Market.Owner == nil {
		m.Market.Owner = m.Author
	}
	return m
}

// MemberEntryFor is the marketplace entry of a planned plugin.
func MemberEntryFor(p *PlannedPlugin) MemberEntry {
	return MemberEntry{
		Name:           p.Name,
		Description:    p.Description,
		Source:         p.Source(),
		Detailed:       true,
		Codex:          slices.Contains(p.Runtimes, config.PluginRuntimeCodex),
		Category:       p.Category,
		Version:        p.Version,
		Keywords:       p.Keywords,
		DefaultEnabled: p.DefaultEnabled,
		Relevance:      p.Relevance,
	}
}

// BundledNames returns the skill and command names some plugin ships: the
// planned domain plugins, and the root [plugin] bundle when the marketplace does
// not replace it.
func BundledNames(cfg *config.Config, tree *config.ContentTree, plan []PlannedPlugin) (skills, commands map[string]bool) {
	skills, commands = map[string]bool{}, map[string]bool{}
	skillPlugins, commandPlugins := BundleMembership(cfg, tree, plan)
	for name := range skillPlugins {
		skills[name] = true
	}
	for name := range commandPlugins {
		commands[name] = true
	}
	return skills, commands
}

// BundleMembership maps each skill and command name to the sorted names of the
// plugins that ship it: the planned domain plugins, and the root [plugin] bundle
// when the marketplace does not replace it.
func BundleMembership(cfg *config.Config, tree *config.ContentTree, plan []PlannedPlugin) (skills, commands map[string][]string) {
	skills, commands = map[string][]string{}, map[string][]string{}
	add := func(dst map[string][]string, name, plugin string) {
		if !slices.Contains(dst[name], plugin) {
			dst[name] = append(dst[name], plugin)
			sort.Strings(dst[name])
		}
	}
	for i := range plan {
		for j := range plan[i].Skills {
			add(skills, plan[i].Skills[j].Name, plan[i].Name)
		}
		for j := range plan[i].Commands {
			add(commands, plan[i].Commands[j].Name, plan[i].Name)
		}
	}
	mkt := cfg.Marketplace
	if cfg.Plugin != nil && (mkt == nil || (len(mkt.Members) == 0 && !mkt.HasDomainPlugins())) {
		rootSkills, rootCommands, _ := includeDomainContent(tree, cfg.Plugin.IncludeDomains)
		for i := range rootSkills {
			add(skills, rootSkills[i].Name, cfg.Plugin.Name)
		}
		for i := range rootCommands {
			add(commands, rootCommands[i].Name, cfg.Plugin.Name)
		}
	}
	return skills, commands
}
