package includes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// This file is the part of the includes package the marketplace and plugin
// resolvers (internal/pluginresolve) build on: they sit above includes because
// they read plugin layouts through internal/importer, which imports includes.

// Index file name written into the cache directory of an https marketplace.
const (
	IndexCacheFile = "marketplace.json"
	// MaxIndexBytes bounds a marketplace.json fetched over https.
	MaxIndexBytes    = 4 << 20
	indexHTTPLimit   = 30 * time.Second
	maxIndexRedirect = 3
)

// ListKey is the config table that declares entries of a lock kind.
func ListKey(kind string) string {
	if kind == lockfile.KindMarketplace {
		return "installed_marketplaces"
	}
	return "installed_plugins"
}

// LoadLock reads the project's lock for a load that resolves marketplaces and
// plugins (see loadLockFor).
func LoadLock(cfg *config.Config) (*lockfile.File, error) { return loadLockFor(cfg) }

// PolicyContext is ctx as cfg's lock policy asks (offline when it says so).
func PolicyContext(ctx context.Context, cfg *config.Config) context.Context {
	return policyContext(ctx, cfg)
}

// Tolerated reports whether a source that cannot be resolved is tolerated.
func Tolerated(ctx context.Context) bool { return tolerated(ctx) }

// StrictLock reports whether an unresolvable source must fail the run.
func StrictLock(cfg *config.Config) bool { return strictLock(cfg) }

// Refreshing reports whether `ai-rulez lock` is re-resolving the named entry.
func Refreshing(cfg *config.Config, kind, name string) bool { return refreshing(cfg, kind, name) }

// Memoized runs fetch the first time key is seen in cfg's load and again (which
// must read the local cache only) on every later call, replaying a first
// failure: a generate run that loads the project twice fetches once.
func Memoized[T any](cfg *config.Config, key string, fetch, again func() (T, error)) (T, error) {
	e := memoFor(cfg).entry("plugin:" + key)
	var out T
	first := false
	e.once.Do(func() {
		first = true
		out, e.err = fetch()
	})
	if first || e.err != nil {
		return out, e.err
	}
	return again()
}

// MemoKey identifies one configured source in the memo.
func MemoKey(cfg *config.Config, source any, hasToken bool) string {
	return memoKey(cfg.BaseDir, source, hasToken)
}

// CheckPin checks a tree that was not fetched on its own (a plugin inside a
// marketplace repository) against the lock entry of w and records what it
// resolved to. origin is recorded on the lock entry.
func CheckPin(cfg *config.Config, lock *lockfile.File, w lockfile.Want, origin, commit, digest, dir string) error {
	st := stateFor(cfg)
	st.addWant(cfg.BaseDir, w, origin)
	p, err := pinFor(cfg, lock, w)
	if err != nil {
		return err
	}
	st.recordDir(cfg.BaseDir, w.Kind, w.Name, dir)
	return p.check(st, cfg.BaseDir, w.Kind, w.Name, commit, digest)
}

// LocalDir resolves a directory of the local filesystem that an installed
// marketplace or plugin names. The committed config must stay inside the
// project (the machine-local overlay and the user scope may leave it), and a
// symlinked root or path is refused.
func LocalDir(cfg *config.Config, kind, name, source, sub string) (string, error) {
	abs := source
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cfg.BaseDir, abs)
	}
	abs = filepath.Clean(abs)
	if err := checkLocalInside(cfg, ListKey(kind), name, abs); err != nil {
		return "", err
	}
	dir := abs
	if sub != "" {
		dir = filepath.Join(abs, filepath.FromSlash(sub))
	}
	info, err := os.Lstat(abs)
	switch {
	case err != nil:
		return "", oops.With("path", abs).Wrapf(err, "%s %q: read the local directory", kind, name)
	case info.Mode()&os.ModeSymlink != 0:
		return "", oops.With("path", abs).Errorf("%s %q: %s is a symlink, and symlinks are not followed", kind, name, abs)
	case !info.IsDir():
		return "", oops.With("path", abs).Errorf("%s %q: %s is not a directory", kind, name, abs)
	}
	if pathHasSymlink(cfg.Log(), abs, dir) {
		return "", oops.With("path", dir).Errorf("%s %q: %s goes through a symlink, and symlinks are not followed", kind, name, dir)
	}
	if st, err := os.Lstat(dir); err != nil || !st.IsDir() {
		return "", oops.With("path", dir).Errorf("%s %q: directory %s not found", kind, name, dir)
	}
	stateFor(cfg).recordDir(cfg.BaseDir, kind, name, dir)
	return dir, nil
}

// checkLocalInside is checkInsideProject for the installed_marketplaces and
// installed_plugins tables.
func checkLocalInside(cfg *config.Config, listKey, name, abs string) error {
	if cfg != nil && (cfg.UserScope || overlaySetsField(cfg, listKey, name, "source")) {
		return nil
	}
	v := viewFor(cfg, cfg.BaseDir)
	abs = realPath(v, abs)
	project := realPath(v, cfg.BaseDir)
	rel, err := filepath.Rel(project, abs)
	if err != nil || safefs.RelEscapes(rel) || filepath.IsAbs(rel) {
		return oops.With("name", name).With("path", abs).With("project", project).
			Hint("Put the path in config.local.toml (machine-local, not committed), or copy the content into the project").
			Wrapf(config.ErrIncludeOutsideProject, "%s %q: local path %s is outside the project %s; a local source in the project config must stay inside the project", listKey, name, abs, project)
	}
	return nil
}

// IndexRequest asks for a marketplace.json served over https.
type IndexRequest struct {
	Name string
	URL  string
	// HTTP is the client to use (nil: a client with a timeout that follows
	// redirects within the host over https only). Tests pass the httptest client.
	HTTP *http.Client
}

// IndexSource is the URL as the lock records it: no credentials, query or fragment.
func IndexSource(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return RedactURL(raw)
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = nil, "", "", "", false
	return u.String()
}

func (r *IndexRequest) client() *http.Client {
	c := &http.Client{Timeout: indexHTTPLimit}
	if r.HTTP != nil {
		copied := *r.HTTP
		c = &copied
	}
	// Whatever client is used, a redirect stays on the same https host.
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxIndexRedirect {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
			return errors.New("a marketplace redirect must stay on the same https host")
		}
		return nil
	}
	return c
}

// FetchIndexURL downloads the marketplace.json at req.URL into the cache and
// checks it against the lock (digest only: a URL has no commit). Offline it reads
// the cached copy.
func FetchIndexURL(ctx context.Context, cfg *config.Config, lock *lockfile.File, req *IndexRequest) (RepoTree, error) {
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return RepoTree{}, oops.With("url", RedactURL(req.URL)).Errorf("marketplace %q: the index URL must be https without credentials", req.Name)
	}
	w := lockfile.Want{Kind: lockfile.KindMarketplace, Name: req.Name, Source: IndexSource(req.URL)}
	st := stateFor(cfg)
	st.addWant(cfg.BaseDir, w, "")
	p, err := pinFor(cfg, lock, w)
	if err != nil {
		return RepoTree{}, err
	}
	cacheDir, err := repoCacheDir(cfg.Host, marketplaceCachePrefix, req.Name, w.Source, "", "")
	if err != nil {
		return RepoTree{}, oops.With("name", req.Name).Wrapf(err, "failed to determine cache directory")
	}
	mu := lockForFetch(cacheDir)
	mu.Lock()
	defer mu.Unlock()
	cached := exists(filepath.Join(cacheDir, IndexCacheFile))
	switch {
	case config.OfflineIncludes(ctx):
		if !cached {
			return RepoTree{}, oops.With("url", w.Source).Wrapf(ErrNotCached, "%s: no cached marketplace found for '%s'", offlineReason(ctx), req.Name)
		}
	default:
		data, netErr := downloadIndex(ctx, req, cfg.Log())
		switch {
		case netErr == nil:
			// The bytes are checked against the pin before they replace the cache.
			if err := installIndex(data, cacheDir, func(tmp string) error {
				digest, err := contentlock.DigestDir(contentlock.KindMarketplace, tmp)
				if err != nil {
					return oops.With("name", req.Name).Wrapf(err, "digest marketplace content")
				}
				return p.check(st, cfg.BaseDir, lockfile.KindMarketplace, req.Name, "", digest)
			}); err != nil {
				return RepoTree{}, err
			}
		case cached:
			cfg.Log().Warn("Could not fetch the marketplace index, using the cached copy", "name", req.Name, "error", netErr)
		default:
			return RepoTree{}, netErr
		}
	}
	digest, err := contentlock.DigestDir(contentlock.KindMarketplace, cacheDir)
	if err != nil {
		return RepoTree{}, oops.With("name", req.Name).Wrapf(err, "digest marketplace content")
	}
	if err := p.check(st, cfg.BaseDir, lockfile.KindMarketplace, req.Name, "", digest); err != nil {
		return RepoTree{}, err
	}
	st.recordDir(cfg.BaseDir, lockfile.KindMarketplace, req.Name, cacheDir)
	return RepoTree{Dir: cacheDir, Digest: digest}, nil
}

// downloadIndex reads the marketplace.json at req.URL, bounded in size.
func downloadIndex(ctx context.Context, req *IndexRequest, log logger.Logger) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, http.NoBody)
	if err != nil {
		return nil, oops.With("url", IndexSource(req.URL)).Wrapf(err, "build the marketplace request")
	}
	httpReq.Header.Set("Accept", "application/json")
	log.Debug("Fetching marketplace index", "name", req.Name, "url", IndexSource(req.URL))
	resp, err := req.client().Do(httpReq)
	if err != nil {
		return nil, oops.With("url", IndexSource(req.URL)).Wrapf(errors.New(RedactURL(err.Error())), "fetch the marketplace index")
	}
	defer resp.Body.Close() //nolint:errcheck // read-only
	if resp.StatusCode != http.StatusOK {
		return nil, oops.With("url", IndexSource(req.URL)).Errorf("marketplace %q: HTTP %d from the index URL", req.Name, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxIndexBytes+1))
	if err != nil {
		return nil, oops.With("url", IndexSource(req.URL)).Wrapf(err, "read the marketplace index")
	}
	if len(data) > MaxIndexBytes {
		return nil, oops.With("url", IndexSource(req.URL)).Errorf("marketplace %q: the index is larger than %d bytes", req.Name, MaxIndexBytes)
	}
	return data, nil
}

// installIndex writes data to a temporary directory, lets verify judge it and
// only then swaps it into the cache, so bytes the lock rejects never replace a
// good copy.
func installIndex(data []byte, cacheDir string, verify func(tmp string) error) error {
	if err := os.MkdirAll(filepath.Dir(cacheDir), cacheDirMode); err != nil {
		return oops.With("cache_dir", cacheDir).Wrapf(err, "failed to create cache directory")
	}
	tmp, err := os.MkdirTemp(filepath.Dir(cacheDir), filepath.Base(cacheDir)+".tmp-*")
	if err != nil {
		return oops.With("cache_dir", cacheDir).Wrapf(err, "failed to create cache directory")
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // best-effort; a no-op after a successful swap
	if err := os.WriteFile(filepath.Join(tmp, IndexCacheFile), data, 0o600); err != nil {
		return oops.With("cache_dir", cacheDir).Wrapf(err, "write the marketplace index")
	}
	if err := verify(tmp); err != nil {
		return err
	}
	if err := swapDir(tmp, cacheDir); err != nil {
		return oops.With("cache_dir", cacheDir).Wrapf(err, "failed to install the marketplace cache")
	}
	return nil
}
