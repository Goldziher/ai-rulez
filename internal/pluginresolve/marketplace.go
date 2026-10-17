package pluginresolve

import (
	"context"
	"net/http"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
)

// market is a resolved marketplace: where its tree is and what its index lists.
type market struct {
	cfg *config.InstalledMarketplaceConfig
	// dir is the marketplace root: relative plugin sources are confined to it.
	dir string
	// commit is the commit of the repository dir belongs to ("" for a local
	// directory or an https index).
	commit string
	idx    *pluginimport.Index
	// kind is the source form (config.PluginSource*).
	kind string
}

// HTTPClient is the client an https marketplace.json is fetched with; tests
// set it on the context with WithHTTPClient.
type httpClientKey struct{}

// WithHTTPClient returns ctx carrying the client an https marketplace index is
// fetched with (the httptest TLS client in tests).
func WithHTTPClient(ctx context.Context, c *http.Client) context.Context {
	return context.WithValue(ctx, httpClientKey{}, c)
}

func httpClientFrom(ctx context.Context) *http.Client {
	c, _ := ctx.Value(httpClientKey{}).(*http.Client) //nolint:errcheck // absent means the default client
	return c
}

func (r *resolver) findMarketplace(name string) *config.InstalledMarketplaceConfig {
	for i := range r.cfg.InstalledMarketplaces {
		if r.cfg.InstalledMarketplaces[i].Name == name {
			return &r.cfg.InstalledMarketplaces[i]
		}
	}
	return nil
}

// marketplace resolves the named marketplace once per run.
func (r *resolver) marketplace(ctx context.Context, name string) (*market, error) {
	if m, ok := r.markets[name]; ok {
		return m, nil
	}
	mp := r.findMarketplace(name)
	if mp == nil {
		return nil, oops.Errorf("marketplace %q is not an installed_marketplaces entry", name)
	}
	m, err := includes.Memoized(r.cfg, "marketplace:"+includes.MemoKey(r.cfg, mp, r.token != ""),
		func() (*market, error) { return r.readMarketplace(ctx, mp) },
		func() (*market, error) { return r.readMarketplace(config.WithOfflineIncludes(ctx), mp) })
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	r.markets[name] = m
	return m, nil
}

func (r *resolver) readMarketplace(ctx context.Context, mp *config.InstalledMarketplaceConfig) (*market, error) {
	m := &market{cfg: mp, kind: config.ClassifyPluginSource(mp.Source)}
	switch m.kind {
	case config.PluginSourceLocal:
		dir, err := includes.LocalDir(r.cfg, lockfile.KindMarketplace, mp.Name, mp.Source, mp.Path)
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		m.dir = dir
		m.idx, err = pluginimport.ReadIndex(dir)
		return m, wrapIndex(mp.Name, err)
	case config.PluginSourceIndexURL:
		tree, err := includes.FetchIndexURL(ctx, r.cfg, r.lock, &includes.IndexRequest{Name: mp.Name, URL: mp.Source, HTTP: httpClientFrom(ctx)})
		if err != nil {
			return nil, err //nolint:wrapcheck // already contextual
		}
		m.dir = tree.Dir
		m.idx, err = pluginimport.ReadIndexFile(tree.Dir)
		return m, wrapIndex(mp.Name, err)
	}
	want, _ := includes.MarketplaceWant(mp)
	tree, err := includes.FetchRepoTree(ctx, r.cfg, r.lock, &includes.RepoTreeRequest{
		Kind: lockfile.KindMarketplace, Name: mp.Name, URL: config.PluginGitURL(mp.Source), Path: mp.Path, Ref: mp.Ref,
		Version: mp.VersionSpec(), LockSource: want.Source,
	}, r.token)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	m.dir, m.commit = tree.Dir, tree.Commit
	m.idx, err = pluginimport.ReadIndex(tree.Dir)
	return m, wrapIndex(mp.Name, err)
}

func wrapIndex(name string, err error) error {
	if err == nil {
		return nil
	}
	return oops.Wrapf(err, "read the index of marketplace %q", name)
}
