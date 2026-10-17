package contentlock

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// collectLocalPlugins pins the tree of every installed_marketplaces and
// installed_plugins entry whose source is a local directory. A remote one is
// pinned by commit and digest; a local one is read unpinned otherwise, so editing
// its hooks.json or a hook script would change the generated settings without
// touching the lock. A plugin installed from a marketplace is covered by the
// marketplace's tree. A tree that cannot be pinned is recorded as a problem.
func (c *collector) collectLocalPlugins() {
	for i := range c.cfg.InstalledMarketplaces {
		mp := &c.cfg.InstalledMarketplaces[i]
		if config.ClassifyPluginSource(mp.Source) != config.PluginSourceLocal {
			continue
		}
		c.pinLocalTree(KindLocalMarketplace, KindMarketplace, mp.Name, mp.Source, "")
	}
	for i := range c.cfg.InstalledPlugins {
		pl := &c.cfg.InstalledPlugins[i]
		if pl.Marketplace != "" || config.ClassifyPluginSource(pl.Source) != config.PluginSourceLocal {
			continue
		}
		c.pinLocalTree(KindLocalPlugin, KindInstalledPlugin, pl.Name, pl.Source, pl.Path)
	}
}

// pinLocalTree digests the directory source/sub as digestKind and records it as
// an item of itemKind.
func (c *collector) pinLocalTree(itemKind, digestKind, name, source, sub string) {
	dir := source
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(c.cfg.BaseDir, dir)
	}
	if sub != "" {
		dir = filepath.Join(dir, filepath.FromSlash(sub))
	}
	digest, err := DigestDir(digestKind, filepath.Clean(dir))
	if err != nil {
		c.problems = append(c.problems, itemKind+" "+name+" cannot be pinned: "+err.Error())
		return
	}
	path := filepath.ToSlash(source)
	if sub != "" {
		path += "/" + filepath.ToSlash(sub)
	}
	c.items = append(c.items, lockfile.Item{Kind: itemKind, ID: name, Path: path, Digest: digest})
}
