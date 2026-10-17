package pluginresolve

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
)

// pluginDelivery is what one installed plugin delivers.
type pluginDelivery = pluginimport.Result

// located is a plugin directory and what to read it with.
type located struct {
	dir string
	// inline are the component fields the marketplace entry declares itself.
	inline map[string]json.RawMessage
}

func (r *resolver) plugin(ctx context.Context, p *config.InstalledPluginConfig) (*pluginDelivery, error) {
	key := "plugin:" + includes.MemoKey(r.cfg, p, r.token != "")
	return includes.Memoized(r.cfg, key,
		func() (*pluginDelivery, error) { return r.readPlugin(ctx, p) },
		func() (*pluginDelivery, error) { return r.readPlugin(config.WithOfflineIncludes(ctx), p) })
}

func (r *resolver) readPlugin(ctx context.Context, p *config.InstalledPluginConfig) (*pluginDelivery, error) {
	loc, err := r.locate(ctx, p)
	if err != nil {
		return nil, err
	}
	scanner := func(sctx context.Context, dir, name string) (config.ContentFile, error) {
		return includes.ScanInstalledSkillDir(logger.WithContext(sctx, r.cfg.Host.Log), dir, name)
	}
	res, err := pluginimport.Load(ctx, loc.dir, &pluginimport.Options{
		Name: p.Name, Format: p.Format, Wants: p.Wants, NamePrefix: p.NamePrefix,
		EnableHooks: p.EnableHooks, EnableMCP: p.MCPEnabled(), Trust: r.cfg.PluginTrust(p),
		Rewrite: relFromProject(r.cfg, loc.dir), Skills: scanner, Inline: loc.inline,
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if res.Empty() && res.HooksDisabled == 0 {
		r.log.Warn("Installed plugin delivered nothing", "plugin", p.Name, "format", res.Format)
	}
	return res, nil
}

// locate finds (fetching if needed) the directory of a plugin and checks it
// against the lock.
func (r *resolver) locate(ctx context.Context, p *config.InstalledPluginConfig) (located, error) {
	if p.Marketplace != "" {
		return r.locateInMarketplace(ctx, p)
	}
	if config.ClassifyPluginSource(p.Source) == config.PluginSourceLocal {
		dir, err := includes.LocalDir(r.cfg, lockfile.KindPlugin, p.Name, p.Source, p.Path)
		return located{dir: dir}, err
	}
	want, _ := includes.PluginWant(r.cfg, p)
	tree, err := includes.FetchRepoTree(ctx, r.cfg, r.lock, &includes.RepoTreeRequest{
		Kind: lockfile.KindPlugin, Name: p.Name, URL: config.PluginGitURL(p.Source), Path: p.Path, Sparse: p.Path != "",
		Ref: p.Ref, Version: p.VersionSpec(), LockSource: want.Source,
	}, r.token)
	return located{dir: tree.Dir}, err
}

func (r *resolver) locateInMarketplace(ctx context.Context, p *config.InstalledPluginConfig) (located, error) {
	m, err := r.marketplace(ctx, p.Marketplace)
	if err != nil {
		return located{}, err
	}
	entry, ok := m.idx.Find(p.EntryName())
	if !ok {
		return located{}, oops.Hint("Marketplace "+p.Marketplace+" lists: "+strings.Join(m.idx.SortedNames(), ", ")).
			Errorf("marketplace %q has no plugin %q", p.Marketplace, p.EntryName())
	}
	if entry.Problem != "" {
		return located{}, oops.Errorf("marketplace %q lists plugin %q, which cannot be installed: %s", p.Marketplace, entry.Name, entry.Problem)
	}
	loc := located{inline: entry.Inline}
	if entry.Source.Kind == pluginimport.SourcePath {
		loc.dir, err = r.inMarketplaceRepo(m, p, entry)
		return loc, err
	}
	src := entry.Source
	ref := src.Ref
	if src.SHA != "" {
		ref = src.SHA
	}
	tree, err := includes.FetchRepoTree(ctx, r.cfg, r.lock, &includes.RepoTreeRequest{
		Kind: lockfile.KindPlugin, Name: p.Name, URL: config.PluginGitURL(src.URL), Path: src.Subdir, Sparse: src.Subdir != "",
		Ref: ref, KeyedByLock: true, LockSource: includes.MarketplaceSource(m.cfg.Name), Origin: src.Describe(),
	}, r.token)
	loc.dir = tree.Dir
	return loc, err
}

// inMarketplaceRepo resolves a plugin that lives inside the marketplace
// repository. Its pin is the marketplace commit and the digest of its own
// directory.
func (r *resolver) inMarketplaceRepo(m *market, p *config.InstalledPluginConfig, entry *pluginimport.Entry) (string, error) {
	if m.kind == config.PluginSourceIndexURL {
		return "", oops.Errorf("plugin %q is a path inside the marketplace, but marketplace %q is a bare marketplace.json URL with no repository; use a repository source", p.Name, m.cfg.Name)
	}
	dir, err := pluginimport.SafeJoin(m.dir, entry.Source.Path)
	if err != nil {
		return "", oops.Wrapf(err, "plugin %q", p.Name)
	}
	if info, statErr := os.Lstat(dir); statErr != nil || !info.IsDir() {
		return "", oops.With("dir", dir).Errorf("plugin %q: the marketplace lists %s, which is not a directory of %q", p.Name, entry.Source.Describe(), m.cfg.Name)
	}
	if m.kind == config.PluginSourceLocal {
		return dir, nil
	}
	want, _ := includes.PluginWant(r.cfg, p)
	digest, err := contentlock.DigestDir(contentlock.KindInstalledPlugin, dir)
	if err != nil {
		return "", oops.With("dir", dir).Wrapf(err, "digest plugin %q", p.Name)
	}
	origin := fmt.Sprintf("%s#%s", includes.IndexSource(m.cfg.Source), entry.Source.Path)
	if err := includes.CheckPin(r.cfg, r.lock, want, origin, m.commit, digest, dir); err != nil {
		return "", err //nolint:wrapcheck // already contextual
	}
	return dir, nil
}
