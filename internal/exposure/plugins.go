package exposure

import (
	"os"
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
)

// PluginMode selects the repository-marketplace plugins a session loads.
type PluginMode string

// The plugin modes.
const (
	// PluginsNone loads no plugin.
	PluginsNone PluginMode = "none"
	// PluginsEnabled loads the plugins the root project settings enable, plus
	// the requested ones.
	PluginsEnabled PluginMode = "enabled"
	// PluginsAll loads every plugin of the marketplace (the opt-in worst case).
	PluginsAll PluginMode = "all"
)

// PluginModes lists the modes.
func PluginModes() []PluginMode { return []PluginMode{PluginsNone, PluginsEnabled, PluginsAll} }

// Valid reports whether m is a known mode.
func (m PluginMode) Valid() bool {
	for _, k := range PluginModes() {
		if m == k {
			return true
		}
	}
	return false
}

// MarketplaceFile is where the repository marketplace index lives.
const MarketplaceFile = ".claude-plugin/" + pluginimport.IndexFileName

// enabledSep separates a plugin from its marketplace in an enabledPlugins key.
const enabledSep = "@"

// PluginRef is a marketplace plugin and its directory.
type PluginRef struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// PluginSummary is the repository marketplace as the inventory read it.
type PluginSummary struct {
	Mode PluginMode `json:"mode"`
	// Marketplace is the name of the repository marketplace ("" without one).
	Marketplace string `json:"marketplace"`
	// Available are the marketplace plugins with an in-repository directory, in
	// index order.
	Available []PluginRef `json:"available"`
	// Enabled are the available plugins the root project settings enable.
	Enabled []string `json:"enabled"`
	// Loaded are the plugins whose skills the inventory lists, in index order.
	Loaded []string `json:"loaded"`
}

// catalog is the repository marketplace and what the settings enable.
type catalog struct {
	name      string
	available []PluginRef
	enabled   map[string]bool
}

// catalog reads the root marketplace. Only plugins whose source is a
// directory of the repository count: a remote source is not in the checkout.
// A plugin is enabled when the merged root settings set
// `<plugin>@<marketplace>` to true and the marketplace lists it.
func (b *builder) catalog() catalog {
	c := catalog{enabled: map[string]bool{}}
	r := b.read(MarketplaceFile)
	if r.status != readOK {
		return c
	}
	ix, err := pluginimport.ParseIndex([]byte(r.text), pluginimport.IndexClaude)
	if err != nil {
		b.problem(MarketplaceFile, err.Error())
		return c
	}
	c.name = ix.Name
	listed := map[string]bool{}
	for i := range ix.Plugins {
		e := &ix.Plugins[i]
		listed[e.Name] = true
		if e.Problem != "" || e.Source.Kind != pluginimport.SourcePath || !b.isDir(e.Source.Path) {
			continue
		}
		c.available = append(c.available, PluginRef{Name: e.Name, Path: e.Source.Path})
	}
	for key, on := range b.rootSettings.enabled {
		plugin, market, found := strings.Cut(key, enabledSep)
		if on && found && c.name != "" && market == c.name && listed[plugin] {
			c.enabled[plugin] = true
		}
	}
	return c
}

// isDir reports whether a relative path is a directory of the repository
// reached without a symlink.
func (b *builder) isDir(rel string) bool {
	abs, err := pluginimport.SafeJoin(b.root, rel)
	if err != nil {
		return false
	}
	info, err := os.Lstat(abs)
	return err == nil && info.IsDir()
}

// selected lists the plugins a mode loads, in index order.
func (c *catalog) selected(mode PluginMode, requested []string) []PluginRef {
	switch mode {
	case PluginsAll:
		return c.available
	case PluginsNone:
		return nil
	}
	wanted := map[string]bool{}
	for k := range c.enabled {
		wanted[k] = true
	}
	for _, r := range requested {
		wanted[r] = true
	}
	var out []PluginRef
	for _, p := range c.available {
		if wanted[p.Name] {
			out = append(out, p)
		}
	}
	return out
}

func (c *catalog) summary(mode PluginMode, requested []string) PluginSummary {
	s := PluginSummary{Mode: mode, Marketplace: c.name, Available: append([]PluginRef{}, c.available...), Enabled: []string{}, Loaded: []string{}}
	for _, p := range c.available {
		if c.enabled[p.Name] {
			s.Enabled = append(s.Enabled, p.Name)
		}
	}
	for _, p := range c.selected(mode, requested) {
		s.Loaded = append(s.Loaded, p.Name)
	}
	return s
}

// pluginSkills lists the skills of the loaded plugins: each
// `<plugin>/skills/<name>/SKILL.md`, named `plugin:skill` in the listing.
// skillOverrides do not apply to them.
func (b *builder) pluginSkills(plugins []PluginRef) []Artifact {
	var out []Artifact
	for _, p := range plugins {
		prefix := p.Path + "/" + dirSkills + "/"
		var files []string
		for _, f := range b.files {
			rest, ok := strings.CutPrefix(f, prefix)
			if dir, file, found := strings.Cut(rest, "/"); ok && found && dir != "" && file == skillFile {
				files = append(files, f)
			}
		}
		sort.Strings(files)
		for _, f := range files {
			a, ok := b.markdownFile(KindSkill, SourcePlugin, f, RootScope, path.Base(path.Dir(f)))
			if !ok {
				continue
			}
			a.Plugin, a.PluginPath = p.Name, p.Path
			a.QualifiedName = p.Name + commandSep + a.Name
			if a.ListingText != "" {
				a.ListingText = a.QualifiedName + strings.TrimPrefix(a.ListingText, a.Name)
				a.Text = a.ListingText
			}
			out = append(out, a)
		}
	}
	return out
}
