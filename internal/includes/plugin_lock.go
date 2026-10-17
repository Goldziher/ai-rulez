package includes

import (
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// MarketplaceSource is the source key a plugin installed from a marketplace
// carries in the lock; the repository the plugin really came from is the entry's Origin.
func MarketplaceSource(name string) string { return "marketplace:" + name }

func findMarketplace(cfg *config.Config, name string) *config.InstalledMarketplaceConfig {
	for i := range cfg.InstalledMarketplaces {
		if cfg.InstalledMarketplaces[i].Name == name {
			return &cfg.InstalledMarketplaces[i]
		}
	}
	return nil
}

// MarketplaceWant is the lock want of an installed marketplace. ok is false for
// a local directory, which lives in the repository and needs no pin.
func MarketplaceWant(m *config.InstalledMarketplaceConfig) (w lockfile.Want, ok bool) {
	switch config.ClassifyPluginSource(m.Source) {
	case config.PluginSourceIndexURL:
		return lockfile.Want{Kind: lockfile.KindMarketplace, Name: m.Name, Source: IndexSource(m.Source)}, true
	case config.PluginSourceGit, config.PluginSourceGitHub:
		w = lockfile.Want{Kind: lockfile.KindMarketplace, Name: m.Name, Source: lockSource("", m.Source), Path: m.Path, Ref: m.Ref}
		return withVersion(w, m.VersionSpec()), true
	}
	return lockfile.Want{}, false
}

// PluginWant is the lock want of an installed plugin. A plugin installed from a
// marketplace is keyed by the marketplace (its own repository is only known once
// the index is read, and recorded as the entry's Origin); ok is false for a
// plugin read from a local directory.
func PluginWant(cfg *config.Config, p *config.InstalledPluginConfig) (w lockfile.Want, ok bool) {
	if p.Marketplace != "" {
		m := findMarketplace(cfg, p.Marketplace)
		if m == nil {
			return lockfile.Want{}, false
		}
		if _, lockable := MarketplaceWant(m); !lockable {
			return lockfile.Want{}, false
		}
		return lockfile.Want{Kind: lockfile.KindPlugin, Name: p.Name, Source: MarketplaceSource(p.Marketplace)}, true
	}
	switch config.ClassifyPluginSource(p.Source) {
	case config.PluginSourceGit, config.PluginSourceGitHub:
		w = lockfile.Want{Kind: lockfile.KindPlugin, Name: p.Name, Source: lockSource("", p.Source), Path: p.Path, Ref: p.Ref}
		return withVersion(w, p.VersionSpec()), true
	}
	return lockfile.Want{}, false
}

// pluginWants lists the marketplaces and plugins of cfg the lock covers, plus the
// plugins only a resolution could name (see resolutionState.addWant).
func pluginWants(cfg *config.Config) []lockfile.Want {
	var wants []lockfile.Want
	seen := map[string]bool{}
	add := func(w lockfile.Want) {
		if k := w.Kind + "\x00" + w.Name; !seen[k] {
			seen[k] = true
			wants = append(wants, w)
		}
	}
	for i := range cfg.InstalledMarketplaces {
		if w, ok := MarketplaceWant(&cfg.InstalledMarketplaces[i]); ok {
			add(w)
		}
	}
	for i := range cfg.InstalledPlugins {
		if w, ok := PluginWant(cfg, &cfg.InstalledPlugins[i]); ok {
			add(w)
		}
	}
	extra := stateFor(cfg).extraWants(cfg.BaseDir)
	for i := range extra {
		add(extra[i])
	}
	return wants
}

// cachedPluginTree locates the cached tree of a marketplace or plugin without
// fetching: the directory that is digested and the cache directory that holds
// its bookkeeping.
func cachedPluginTree(cfg *config.Config, w lockfile.Want) (dir, cacheDir, treeKind string) {
	if w.Kind == lockfile.KindMarketplace {
		treeKind = contentlock.KindMarketplace
	} else {
		treeKind = contentlock.KindInstalledPlugin
	}
	if d := stateFor(cfg).dirFor(cfg.BaseDir, w.Kind, w.Name); d != "" {
		return d, cacheDirOf(d), treeKind
	}
	switch w.Kind {
	case lockfile.KindMarketplace:
		m := findMarketplace(cfg, w.Name)
		if m == nil {
			return "", "", treeKind
		}
		if config.ClassifyPluginSource(m.Source) == config.PluginSourceIndexURL {
			cacheDir, err := repoCacheDir(cfg.Host, marketplaceCachePrefix, w.Name, w.Source, "", "")
			if err != nil || !exists(filepath.Join(cacheDir, IndexCacheFile)) {
				return "", "", treeKind
			}
			return cacheDir, cacheDir, treeKind
		}
		cacheDir, err := repoCacheDir(cfg.Host, marketplaceCachePrefix, w.Name, config.PluginGitURL(m.Source), "", w.Ref)
		if err != nil {
			return "", "", treeKind
		}
		return existingDir(cacheDir, m.Path), cacheDir, treeKind
	case lockfile.KindPlugin:
		for i := range cfg.InstalledPlugins {
			p := &cfg.InstalledPlugins[i]
			if p.Name != w.Name || p.Source == "" {
				continue
			}
			sparse := ""
			if p.Path != "" {
				sparse = p.Path
			}
			cacheDir, err := repoCacheDir(cfg.Host, pluginCachePrefix, w.Name, config.PluginGitURL(p.Source), sparse, w.Ref)
			if err != nil {
				return "", "", treeKind
			}
			return existingDir(cacheDir, p.Path), cacheDir, treeKind
		}
	}
	return "", "", treeKind
}

// cacheDirOf is the clone directory of a tree dir below the plugin or
// marketplace cache root: the cache keeps its bookkeeping file at the clone root.
func cacheDirOf(dir string) string {
	for cur := dir; ; cur = filepath.Dir(cur) {
		if exists(filepath.Join(cur, cacheMetaFile)) {
			return cur
		}
		if parent := filepath.Dir(cur); parent == cur {
			return dir
		}
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func existingDir(root, sub string) string {
	dir := root
	if sub != "" {
		dir = filepath.Join(root, filepath.FromSlash(sub))
	}
	if info, err := os.Lstat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// commitless reports whether a want has no commit to pin: an https
// marketplace.json is pinned by the digest of its bytes only.
func commitless(w lockfile.Want) bool {
	return w.Kind == lockfile.KindMarketplace && config.ClassifyPluginSource(w.Source) == config.PluginSourceIndexURL
}

// viaRefreshedMarketplace reports whether the named plugin is installed from a
// marketplace that is being re-resolved: the index it was read from moves, so
// its own pin does too.
func viaRefreshedMarketplace(cfg *config.Config, name string) bool {
	for i := range cfg.InstalledPlugins {
		if p := &cfg.InstalledPlugins[i]; p.Name == name && p.Marketplace != "" {
			return cfg.LockPolicy.Refreshing(lockfile.KindMarketplace, p.Marketplace)
		}
	}
	return false
}
