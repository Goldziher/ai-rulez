package includes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// Cache directory prefixes of fetched marketplaces and plugins.
const (
	marketplaceCachePrefix = "marketplaces"
	pluginCachePrefix      = "plugins"
)

// RepoTreeRequest asks for a directory of a git repository, pinned by
// ai-rulez.lock under (Kind, Name). The installed-marketplace and
// installed-plugin resolvers use it; it is the same fetch, cache, pin and
// version machinery installed skills use.
type RepoTreeRequest struct {
	// Kind is lockfile.KindMarketplace or lockfile.KindPlugin.
	Kind string
	Name string
	// URL is the repository (a github shorthand already expanded). A credential in it is refused.
	URL string
	// Path is the directory of the repository the request exposes ("" = the root).
	Path string
	// Sparse clones only Path instead of the whole repository.
	Sparse  bool
	Ref     string
	Version config.VersionSpec
	// LockSource is the source key the lock records ("" = the redacted URL).
	LockSource string
	// Origin is recorded on the lock entry (see lockfile.Entry.Origin).
	Origin string
	// KeyedByLock makes the lock entry independent of what is fetched: it is
	// keyed by LockSource, LockPath and LockRef, and Version is ignored. A plugin
	// installed from a marketplace is pinned this way ("marketplace:<name>"), the
	// index being what names the repository to fetch.
	KeyedByLock       bool
	LockPath, LockRef string
}

// RepoTree is a fetched, pin-checked directory.
type RepoTree struct {
	// Dir is the directory the request exposes inside the cache.
	Dir string
	// Commit is the commit the cache is at.
	Commit string
	// Digest is the tree digest of Dir.
	Digest string
}

func (r *RepoTreeRequest) treeKind() string {
	if r.Kind == lockfile.KindMarketplace {
		return contentlock.KindMarketplace
	}
	return contentlock.KindInstalledPlugin
}

func (r *RepoTreeRequest) cachePrefix() string {
	if r.Kind == lockfile.KindMarketplace {
		return marketplaceCachePrefix
	}
	return pluginCachePrefix
}

// want is the lock want of the request.
func (r *RepoTreeRequest) want() lockfile.Want {
	if r.KeyedByLock {
		return lockfile.Want{Kind: r.Kind, Name: r.Name, Source: r.LockSource, Path: r.LockPath, Ref: r.LockRef}
	}
	src := r.LockSource
	if src == "" {
		src = lockSource("", r.URL)
	}
	return withVersion(lockfile.Want{Kind: r.Kind, Name: r.Name, Source: src, Path: r.Path, Ref: r.Ref}, r.Version)
}

// repoCacheDir isolates repository, sparse path and ref, like skill caches do.
func repoCacheDir(host ambient.Host, prefix, name, repoURL, sparsePath, ref string) (string, error) {
	sum := sha256.Sum256([]byte(normalizeGitURL(repoURL) + "\x00" + sparsePath + "\x00" + ref))
	dir := SafeCacheName(name) + "-" + hex.EncodeToString(sum[:])[:cacheHashLen]
	return config.CacheDirIn(host.Env, prefix, dir) //nolint:wrapcheck // already contextual
}

type repoFetch struct {
	req         *RepoTreeRequest
	url         string
	ref         string
	cacheDir    string
	accessToken string
	pin         *pin
	baseDir     string
	state       *resolutionState
	log         logger.Logger
}

func (f *repoFetch) sparse() string {
	if f.req.Sparse && f.req.Path != "" {
		return strings.TrimPrefix(f.req.Path, "/") + "/"
	}
	return ""
}

func (f *repoFetch) resolvedRef() string {
	if f.ref != "" {
		return f.ref
	}
	return refHead
}

// FetchRepoTree fetches (or, offline, reads from the cache) the repository
// directory req names, checks it against the lock and records what it resolved
// to for `ai-rulez lock`.
func FetchRepoTree(ctx context.Context, cfg *config.Config, lock *lockfile.File, req *RepoTreeRequest, token string) (RepoTree, error) {
	if err := validateGitURL(stripGitPlus(req.URL)); err != nil {
		return RepoTree{}, err
	}
	if path, ok := lockfile.FileURLPath(req.URL); ok && !lockfile.AllowFileURLsOutside(cfg.Host.Env) {
		if err := checkInsideProject(cfg, cfg.BaseDir, "source", req.Name, path); err != nil {
			return RepoTree{}, err
		}
	}
	if err := cfg.CheckSource("installed plugin source", req.Name, req.URL); err != nil {
		return RepoTree{}, err //nolint:wrapcheck // already contextual
	}
	w := req.want()
	stateFor(cfg).addWant(cfg.BaseDir, w, req.Origin)
	p, err := pinFor(cfg, lock, w)
	if err != nil {
		return RepoTree{}, err
	}
	url := stripGitPlus(req.URL)
	var ref string
	if req.KeyedByLock {
		ref = p.effectiveRef(req.Ref)
	} else if ref, err = versionRef(ctx, cfg, lock, w, p, url, token, cfg.BaseDir); err != nil {
		return RepoTree{}, err
	}
	sparse := ""
	if req.Sparse {
		sparse = req.Path
	}
	cacheRef := w.Ref
	if req.KeyedByLock {
		cacheRef = req.Ref
	}
	cacheDir, err := repoCacheDir(cfg.Host, req.cachePrefix(), req.Name, url, sparse, cacheRef)
	if err != nil {
		return RepoTree{}, oops.With("name", req.Name).Wrapf(err, "failed to determine cache directory")
	}
	f := &repoFetch{req: req, url: url, ref: ref, cacheDir: cacheDir, accessToken: token,
		pin: p, baseDir: cfg.BaseDir, state: stateFor(cfg), log: cfg.Log()}
	tree, err := f.run(ctx)
	if err != nil {
		return RepoTree{}, err
	}
	f.state.recordDir(cfg.BaseDir, req.Kind, req.Name, tree.Dir)
	return tree, nil
}

func (f *repoFetch) run(ctx context.Context) (RepoTree, error) {
	mu := lockForFetch(f.cacheDir)
	mu.Lock()
	defer mu.Unlock()
	dir, err := f.locate(ctx)
	if err != nil {
		return RepoTree{}, err
	}
	tree, err := f.checkPin(dir)
	if retryable(ctx, err) {
		// The cache may be damaged: fetch the pinned commit again before failing.
		_ = os.Remove(filepath.Join(f.cacheDir, cacheMetaFile)) //nolint:errcheck // best-effort; a stale meta only skips the retry
		if dir, err = f.locate(ctx); err != nil {
			return RepoTree{}, err
		}
		tree, err = f.checkPin(dir)
	}
	return tree, err
}

// checkPin records what the cache holds and verifies it against the lock.
func (f *repoFetch) checkPin(dir string) (RepoTree, error) {
	digest, err := contentlock.DigestDir(f.req.treeKind(), dir)
	if err != nil {
		return RepoTree{}, oops.With("name", f.req.Name).Wrapf(err, "digest %s content", f.req.Kind)
	}
	commit := ""
	if meta, metaErr := readCacheMeta(f.cacheDir); metaErr == nil && meta != nil {
		commit = meta.RemoteHEADSHA
	}
	if err := f.pin.check(f.state, f.baseDir, f.req.Kind, f.req.Name, commit, digest); err != nil {
		return RepoTree{}, err
	}
	return RepoTree{Dir: dir, Commit: commit, Digest: digest}, nil
}

// entryDir is the directory the request exposes inside the cache, or "".
func (f *repoFetch) entryDir() string {
	dir := f.cacheDir
	if f.req.Path != "" {
		dir = filepath.Join(f.cacheDir, filepath.FromSlash(f.req.Path))
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || pathHasSymlink(f.log, f.cacheDir, dir) {
		return ""
	}
	return dir
}

func (f *repoFetch) locate(ctx context.Context) (string, error) {
	f.log.Debug("Fetching "+f.req.Kind, "name", f.req.Name, "repo", RedactURL(f.url), "path", f.req.Path, "ref", f.ref)
	if config.OfflineIncludes(ctx) {
		if dir := f.entryDir(); dir != "" {
			return dir, nil
		}
		return "", oops.With("repo", RedactURL(f.url)).With("cache_dir", f.cacheDir).
			Wrapf(ErrNotCached, "%s: no cached %s found for '%s'", offlineReason(ctx), f.req.Kind, f.req.Name)
	}
	ref := f.resolvedRef()
	isSHA := isFullSHA(ref)
	currentSHA := ref
	if !isSHA {
		var err error
		currentSHA, err = remoteHEADSHA(ctx, f.url, ref, f.accessToken)
		if err != nil {
			if dir := f.entryDir(); dir != "" {
				f.log.Warn("ls-remote failed, using the cached "+f.req.Kind, "name", f.req.Name, "error", err)
				return dir, nil
			}
			return "", oops.With("repo", RedactURL(f.url)).Wrapf(err, "failed to get remote HEAD for %s %q", f.req.Kind, f.req.Name)
		}
	}
	if isCacheHit(f.cacheDir, currentSHA) {
		if dir := f.entryDir(); dir != "" {
			return dir, nil
		}
	}
	if err := f.refresh(ctx, ref, isSHA, currentSHA); err != nil {
		return "", err
	}
	if dir := f.entryDir(); dir != "" {
		return dir, nil
	}
	return "", oops.With("repo", RedactURL(f.url)).With("path", f.req.Path).
		Errorf("%s %q: directory %q not found in the repository", f.req.Kind, f.req.Name, f.req.Path)
}

func (f *repoFetch) refresh(ctx context.Context, ref string, isSHA bool, currentSHA string) error {
	if err := requireGit(ctx); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.cacheDir), cacheDirMode); err != nil {
		return oops.With("cache_dir", f.cacheDir).Wrapf(err, "failed to create cache directory")
	}
	tmpDir, err := os.MkdirTemp(filepath.Dir(f.cacheDir), filepath.Base(f.cacheDir)+".tmp-*")
	if err != nil {
		return oops.With("cache_dir", f.cacheDir).Wrapf(err, "failed to create cache directory")
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort; a no-op after a successful swap

	if err := cloneFor(isSHA)(ctx, f.url, ref, f.sparse(), tmpDir, f.accessToken); err != nil {
		return oops.With("repo", RedactURL(f.url)).With("path", f.req.Path).Wrapf(err, "failed to clone %s %q", f.req.Kind, f.req.Name)
	}
	hashes, _ := computeFileHashes(tmpDir) //nolint:errcheck // best-effort; missing hashes degrade to a full refetch next run
	_ = writeCacheMeta(tmpDir, &CacheMeta{ //nolint:errcheck // best-effort; failing to persist meta causes a refetch next run
		RemoteHEADSHA: currentSHA,
		FetchedAt:     ambient.FromContext(ctx).Now(),
		FileHashes:    hashes,
	})
	if err := swapDir(tmpDir, f.cacheDir); err != nil {
		return oops.With("cache_dir", f.cacheDir).Wrapf(err, "failed to install the %s cache", f.req.Kind)
	}
	return nil
}
