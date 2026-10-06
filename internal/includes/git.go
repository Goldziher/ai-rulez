package includes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

// errRefLookupUnavailable signals that the remote ref could not be resolved but
// a usable cache exists; Fetch then serves cached content. Only reachable from
// the branch/tag path — a pinned SHA is never downgraded to cache (#167).
var errRefLookupUnavailable = errors.New("ref lookup failed, using cached content")

const (
	rootPath   = "/"
	aiRulezDir = ".ai-rulez"
	// contextSubdir is the name of the context content subdirectory.
	contextSubdir = "context"
	// rulesSubdir is the name of the rules content subdirectory.
	rulesSubdir = "rules"
)

// SkipFetch when true causes git sources to use cached content without fetching.
// Set from the --no-fetch CLI flag.
var SkipFetch bool

// ErrNotCached is wrapped by the error an offline load returns for an include or
// installed skill that has no cached copy, so callers can tell a cache miss from
// any other load failure.
var ErrNotCached = errors.New("not in the local cache")

// fetchLocks serializes Fetch calls per cache directory. Multiple configs
// processed in parallel often reference the same shared include — without
// this, concurrent goroutines would race on RemoveAll/MkdirAll/extract into
// the same cache directory and corrupt it. Keyed by absolute cacheDir.
var (
	fetchLocksMu sync.Mutex
	fetchLocks   = map[string]*sync.Mutex{}
)

// lockForFetch returns a mutex unique to the given cache directory.
func lockForFetch(cacheDir string) *sync.Mutex {
	fetchLocksMu.Lock()
	defer fetchLocksMu.Unlock()
	if m, ok := fetchLocks[cacheDir]; ok {
		return m
	}
	m := &sync.Mutex{}
	fetchLocks[cacheDir] = m
	return m
}

// scannedTrees memoizes successful ContentTree scans for the lifetime of
// the process, keyed by absolute aiRulezDir path. Recursive multi-config
// generation often references the same shared include from many configs;
// without this, ScanContentTree would walk the same cached `.ai-rulez/`
// tree once per consumer (e.g. 18 configs × 5 includes = 90 scans).
//
// The cached tree is the unfiltered scan; per-consumer include filters are
// applied on each call via [GitSource.filterContent], which returns a new
// tree without mutating the input.
//
// Entries are invalidated when [GitSource.Fetch] refreshes the cache
// directory (the slow path deletes the entry before re-scanning).
var (
	scannedTreesMu sync.RWMutex
	scannedTrees   = map[string]*config.ContentTree{}
)

func cachedScan(aiRulezDir string) *config.ContentTree {
	scannedTreesMu.RLock()
	tree := scannedTrees[aiRulezDir]
	scannedTreesMu.RUnlock()
	return tree
}

func storeScan(aiRulezDir string, tree *config.ContentTree) {
	scannedTreesMu.Lock()
	scannedTrees[aiRulezDir] = tree
	scannedTreesMu.Unlock()
}

func invalidateScan(aiRulezDir string) {
	scannedTreesMu.Lock()
	delete(scannedTrees, aiRulezDir)
	scannedTreesMu.Unlock()
}

// cacheDirMode is the mode of include cache directories: fetched content is
// private to the user running ai-rulez.
const cacheDirMode = 0o700

// getIncludeCacheDir returns the cache directory for an include: its name plus a
// short hash of the normalized URL, so two includes that share a name but point
// at different repositories (in different projects) never share a cache, and one
// project's fetch cannot seed another's.
func getIncludeCacheDir(host ambient.Host, sourceName, repoURL string) (string, error) {
	sum := sha256.Sum256([]byte(normalizeGitURL(repoURL)))
	dir := safeCacheName(sourceName) + "-" + hex.EncodeToString(sum[:])[:12]
	return config.CacheDirIn(host.Env, "includes", dir) //nolint:wrapcheck // already contextual
}

// safeCacheName keeps a source name usable as one path segment.
func safeCacheName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if out := strings.Trim(b.String(), "."); out != "" {
		return out
	}
	return "include"
}

// GitSource represents a git repository source
type GitSource struct {
	name        string
	repoURL     string // Normalized HTTPS URL
	originalURL string // Original URL (may be SSH format)
	path        string // Path within repo to .ai-rulez/ (optional, defaults to root)
	ref         string // branch, tag, or commit (optional, defaults to main/master)
	cacheDir    string // ~/.cache/ai-rulez/includes/{name}-{url hash}/
	include     []string
	accessToken string
	pin         *pin   // ai-rulez.lock entry this source must match (nil: unpinned)
	baseDir     string // project the include belongs to, for recording what it resolved to
	okf         bool   // the repository holds an OKF bundle (at path) instead of an .ai-rulez directory
}

// NewGitSource creates a new git source
func NewGitSource(name, repoURL, path, ref, baseDir string, include []string, accessToken string) (*GitSource, error) {
	return NewGitSourceIn(ambient.Host{}, name, repoURL, path, ref, baseDir, include, accessToken)
}

// NewGitSourceIn is NewGitSource with the environment (the cache location) taken
// from host instead of the process.
func NewGitSourceIn(host ambient.Host, name, repoURL, path, ref, baseDir string, include []string, accessToken string) (*GitSource, error) {
	repoURL = stripGitPlus(repoURL)
	// Validate URL format
	if err := validateGitURL(repoURL); err != nil {
		return nil, err
	}

	// Create cache directory in system cache location
	cacheDir, err := getIncludeCacheDir(host, name, repoURL)
	if err != nil {
		return nil, oops.
			With("source_name", name).
			Wrapf(err, "failed to determine cache directory")
	}

	source := &GitSource{
		name:        name,
		repoURL:     normalizeGitURL(repoURL),
		originalURL: repoURL,
		path:        path,
		ref:         ref,
		cacheDir:    cacheDir,
		include:     include,
		accessToken: accessToken,
		baseDir:     baseDir,
	}

	return source, nil
}

// NewOKFGitSource is NewGitSource for a repository holding an Open Knowledge
// Format bundle, at path or at the repository root. The bundle is cached, pinned
// and digested like any other git include, fetched with hardened git options,
// and converted to content on each use.
func NewOKFGitSource(name, repoURL, path, ref, baseDir string, include []string, accessToken string) (*GitSource, error) {
	return NewOKFGitSourceIn(ambient.Host{}, name, repoURL, path, ref, baseDir, include, accessToken)
}

// NewOKFGitSourceIn is NewOKFGitSource with the environment taken from host.
func NewOKFGitSourceIn(host ambient.Host, name, repoURL, path, ref, baseDir string, include []string, accessToken string) (*GitSource, error) {
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return nil, oops.With("include", name).Errorf("invalid OKF bundle path %q", path)
		}
	}
	s, err := NewGitSourceIn(host, name, repoURL, path, ref, baseDir, include, accessToken)
	if err != nil {
		return nil, err
	}
	s.okf = true
	return s, nil
}

// GetType returns the source type
func (s *GitSource) GetType() SourceType {
	return SourceTypeGit
}

// GetName returns the source name
func (s *GitSource) GetName() string {
	return s.name
}

// resolvedRef returns s.ref or "HEAD" when empty.
func (s *GitSource) resolvedRef() string {
	if s.ref != "" {
		return s.ref
	}
	return refHead
}

// sparsePathSpec returns the git pathSpec to pass to sparseClone.
// Includes without a configured sub-path use ".ai-rulez/" as the narrow spec.
func (s *GitSource) sparsePathSpec() string {
	if s.okf {
		if s.path == "" || s.path == rootPath {
			return "" // the whole repository
		}
		return strings.Trim(s.path, "/") + "/"
	}
	if s.path == "" || s.path == "/" {
		return ".ai-rulez/"
	}
	return strings.Trim(s.path, "/") + "/"
}

// Fetch downloads content from git repository and returns the content tree.
//
// Safe for concurrent invocation. The fast path (cache SHA matches remote,
// or --no-fetch) is lock-free; only refresh of a stale cache is serialized
// by a per-cacheDir mutex with double-checked locking.
func (s *GitSource) Fetch(ctx context.Context) (*config.ContentTree, error) {
	tree, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	err = s.checkPin()
	if retryable(ctx, err) {
		// The cache may be damaged: fetch the pinned commit again before failing.
		_ = os.Remove(filepath.Join(s.cacheDir, cacheMetaFile)) //nolint:errcheck // best-effort; a stale meta only skips the retry
		if tree, err = s.fetch(ctx); err != nil {
			return nil, err
		}
		err = s.checkPin()
	}
	if err != nil {
		return nil, err
	}
	return tree, nil
}

// checkPin records what the cache holds and verifies it against the lock.
func (s *GitSource) checkPin() error {
	dir := s.findAIRulezDir()
	if dir == "" {
		return nil
	}
	kind := contentlock.KindInclude
	if s.okf {
		kind = contentlock.KindOKFInclude
	}
	digest, err := contentlock.DigestDir(kind, dir)
	if err != nil {
		return oops.With("include", s.name).Wrapf(err, "digest include content")
	}
	commit := ""
	if meta, metaErr := readCacheMeta(s.cacheDir); metaErr == nil && meta != nil {
		commit = meta.RemoteHEADSHA
	}
	return s.pin.check(s.baseDir, lockfile.KindInclude, s.name, commit, digest)
}

func (s *GitSource) fetch(ctx context.Context) (*config.ContentTree, error) {
	if s.okf {
		ctx = withHardenedGit(ctx)
	}
	logger.Debug("Fetching git source", "name", s.name, "repo", RedactURL(s.repoURL), "ref", s.ref, "path", s.path, "has_token", s.accessToken != "")

	if SkipFetch || config.OfflineIncludes(ctx) {
		if s.findAIRulezDir() == "" {
			return nil, oops.
				With("repo", RedactURL(s.repoURL)).
				With("cache_dir", s.cacheDir).
				Wrapf(ErrNotCached, "--no-fetch specified but no cached content found for include '%s'", s.name)
		}
		logger.Debug("Skipping fetch (--no-fetch), using cached content", "name", s.name)
		return s.scanCachedContent(ctx)
	}

	scrubLegacyCredentials(ctx, s.cacheDir, s.originalURL)
	ref := s.resolvedRef()

	// A full commit SHA cannot be advertised by ls-remote — it IS the commit.
	// Use it as the authoritative SHA directly; the slow path fetches the exact
	// object and fails closed if the remote cannot serve it (#167).
	currentSHA, isSHA, err := s.resolveHeadSHA(ctx, ref)
	if err != nil {
		if errors.Is(err, errRefLookupUnavailable) {
			logger.Warn("ls-remote failed, using cached content", "name", s.name, "error", err)
			return s.scanCachedContent(ctx)
		}
		return nil, err
	}

	// Fast path: cache SHA matches — touch FetchedAt and return without acquiring the write lock.
	if isCacheHit(s.cacheDir, currentSHA) {
		meta, _ := readCacheMeta(s.cacheDir) //nolint:errcheck // best-effort; cache hit already confirmed
		if meta != nil {
			meta.FetchedAt = ambient.FromContext(ctx).Now()
			_ = writeCacheMeta(s.cacheDir, meta) //nolint:errcheck // best-effort timestamp update
		}
		return s.scanCachedContent(ctx)
	}

	// Slow path: cache is stale or missing. Serialize the refresh per cache directory.
	mu := lockForFetch(s.cacheDir)
	mu.Lock()
	defer mu.Unlock()

	// Double-check under the lock — another goroutine may have refreshed while we waited.
	if isCacheHit(s.cacheDir, currentSHA) {
		return s.scanCachedContent(ctx)
	}

	return s.refreshCache(ctx, ref, currentSHA, isSHA)
}

// resolveHeadSHA returns the authoritative commit SHA for ref. A full commit
// SHA is used verbatim (it cannot be advertised by ls-remote); any other ref
// is resolved against the remote, falling back to cached content when the
// lookup fails (never for a pinned SHA — those fail closed, #167).
func (s *GitSource) resolveHeadSHA(ctx context.Context, ref string) (currentSHA string, isSHA bool, err error) {
	if isFullSHA(ref) {
		currentSHA = ref
		isSHA = true
		return
	}
	currentSHA, err = remoteHEADSHA(ctx, s.originalURL, ref, s.accessToken)
	if err != nil {
		if meta, metaErr := readCacheMeta(s.cacheDir); metaErr == nil && meta != nil {
			return "", false, errRefLookupUnavailable
		}
		return "", false, oops.Wrapf(err, "failed to get remote HEAD for include %q", s.name)
	}
	return
}

// refreshCache replaces the on-disk cache with a fresh sparse clone of ref,
// records the fetched SHA, and returns the scanned content tree.
func (s *GitSource) refreshCache(ctx context.Context, ref, currentSHA string, isSHA bool) (*config.ContentTree, error) {
	// Invalidate any memoized scan since the on-disk tree is about to change.
	if dir := s.findAIRulezDir(); dir != "" {
		invalidateScan(dir)
	}
	if err := os.RemoveAll(s.cacheDir); err != nil {
		logger.Warn("Failed to clear include cache", "cache_dir", s.cacheDir, "error", err)
	}
	if err := os.MkdirAll(s.cacheDir, cacheDirMode); err != nil {
		return nil, oops.
			With("cache_dir", s.cacheDir).
			Wrapf(err, "failed to create cache directory")
	}

	if err := requireGit(ctx); err != nil {
		return nil, err
	}
	pathSpec := s.sparsePathSpec()
	if err := cloneFor(isSHA)(ctx, s.originalURL, ref, pathSpec, s.cacheDir, s.accessToken); err != nil {
		return nil, oops.With("repo", RedactURL(s.repoURL)).Wrapf(err, "failed to clone include %q", s.name)
	}

	hashes, _ := computeFileHashes(s.cacheDir) //nolint:errcheck // best-effort; missing hashes degrade to full refetch next run
	_ = writeCacheMeta(s.cacheDir, &CacheMeta{ //nolint:errcheck // best-effort; failing to persist meta causes a refetch next run
		RemoteHEADSHA: currentSHA,
		FetchedAt:     ambient.FromContext(ctx).Now(),
		FileHashes:    hashes,
	})

	return s.scanCachedContent(ctx)
}

// scanCachedContent locates the .ai-rulez directory in the cache and returns its content tree.
func (s *GitSource) scanCachedContent(ctx context.Context) (*config.ContentTree, error) {
	if s.okf {
		dir := s.findAIRulezDir()
		if dir == "" {
			return nil, oops.With("repo", RedactURL(s.repoURL)).With("path", s.path).Errorf("no OKF bundle found in repository")
		}
		return convertOKFBundle(ctx, dir, s.name, s.include)
	}
	// Find the .ai-rulez directory in the extracted content
	aiRulezDir := s.findAIRulezDir()
	if aiRulezDir == "" {
		return nil, oops.
			With("repo", RedactURL(s.repoURL)).
			With("ref", s.ref).
			With("path", s.path).
			Errorf("no .ai-rulez directory found in repository")
	}

	logger.Debug("Found .ai-rulez directory", "path", aiRulezDir)

	// Process-level memoization: scanning the same cached tree from many
	// consumer configs is wasted work — return the previously scanned
	// tree if we have one. Filtering still runs per consumer below.
	contentTree := cachedScan(aiRulezDir)
	if contentTree == nil {
		// Scan the .ai-rulez directory structure using the config loader's scanner which keeps
		// root content and domain content separate (avoids duplication in generated output)
		scanned, err := config.ScanContentTreeContext(ctx, aiRulezDir)
		if err != nil {
			return nil, oops.
				With("repo", RedactURL(s.repoURL)).
				Wrapf(err, "failed to scan content tree")
		}
		storeScan(aiRulezDir, scanned)
		contentTree = scanned
	} else {
		logger.Debug("Reusing cached content tree scan", "path", aiRulezDir)
	}

	// Filter content based on include list if specified.
	// filterContent returns a new tree, so the cached unfiltered tree is
	// never mutated by per-consumer filtering.
	if len(s.include) > 0 {
		contentTree = s.filterContent(contentTree)
	}

	return contentTree, nil
}

// findAIRulezDir finds the .ai-rulez directory in the cache
func (s *GitSource) findAIRulezDir() string {
	if s.okf {
		dir := s.cacheDir
		if s.path != "" && s.path != rootPath {
			dir = filepath.Join(s.cacheDir, filepath.FromSlash(strings.Trim(s.path, "/")))
		}
		if isRealDir(dir) {
			return dir
		}
		return ""
	}
	// Check for .ai-rulez in the cache directory
	// It could be at the root level or under the specified path

	aiRulezPath := filepath.Join(s.cacheDir, aiRulezDir)
	if isRealDir(aiRulezPath) {
		return aiRulezPath
	}

	// If path was specified, check if .ai-rulez is under that path
	if s.path != "" && s.path != rootPath {
		cleanPath := strings.Trim(s.path, rootPath)
		aiRulezPath := filepath.Join(s.cacheDir, cleanPath, aiRulezDir)
		if isRealDir(aiRulezPath) {
			return aiRulezPath
		}

		// Check if sub-path has flat ai-rulez structure (rules/, context/, etc. directly)
		subPathDir := filepath.Join(s.cacheDir, cleanPath)
		if s.hasAIRulezStructure(subPathDir) {
			return subPathDir
		}
	}

	// Check if cache root has flat ai-rulez structure
	if s.hasAIRulezStructure(s.cacheDir) {
		return s.cacheDir
	}

	return ""
}

// filterContent filters content based on include list
func (s *GitSource) filterContent(tree *config.ContentTree) *config.ContentTree {
	filtered := &config.ContentTree{
		Domains: make(map[string]*config.Domain),
	}

	// Helper to check if a content type should be included
	shouldInclude := func(contentType string) bool {
		for _, inc := range s.include {
			if inc == contentType {
				return true
			}
		}
		return false
	}

	// Filter root content
	if shouldInclude(rulesSubdir) {
		filtered.Rules = tree.Rules
	}
	if shouldInclude(contextSubdir) {
		filtered.Context = tree.Context
	}
	if shouldInclude("skills") {
		filtered.Skills = tree.Skills
	}
	if shouldInclude("agents") {
		filtered.Agents = tree.Agents
	}
	if shouldInclude("commands") {
		filtered.Commands = tree.Commands
	}
	if shouldInclude("checks") {
		filtered.Checks = tree.Checks
	}

	// Copy domains (domains always included if they exist)
	filtered.Domains = tree.Domains

	return filtered
}

// validateGitURL validates that the URL is a valid git repository URL
func validateGitURL(urlStr string) error {
	if urlStr == "" {
		return oops.Errorf("repository URL cannot be empty")
	}

	// Check for SSH URL format (git@host:owner/repo.git or ssh://git@host/owner/repo.git)
	if strings.HasPrefix(urlStr, "git@") || strings.HasPrefix(urlStr, "ssh://") {
		// SSH URLs are valid, they'll be normalized to HTTPS
		return nil
	}

	// Local filesystem repos using the file scheme are valid git sources used in tests and CI.
	if strings.HasPrefix(urlStr, "file://") {
		return nil
	}

	// Check for HTTP/HTTPS URLs
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return oops.
			With("url", RedactURL(urlStr)).
			Hint("Git repository URLs must use http://, https://, file://, git@, or ssh:// protocol").
			Errorf("invalid git repository URL format")
	}

	// Try to parse as URL
	if _, err := url.Parse(urlStr); err != nil {
		return oops.
			With("url", RedactURL(urlStr)).
			Errorf("invalid URL format: %s", RedactURL(err.Error()))
	}

	return nil
}

// normalizeGitURL normalizes a git repository URL
func normalizeGitURL(urlStr string) string {
	// Convert SSH URLs to HTTPS format
	// Format: git@github.com:owner/repo.git -> https://github.com/owner/repo
	if strings.HasPrefix(urlStr, "git@") {
		// Remove git@ prefix
		urlStr = strings.TrimPrefix(urlStr, "git@")

		// Replace first colon with slash (git@host:owner/repo -> host/owner/repo)
		urlStr = strings.Replace(urlStr, ":", "/", 1)

		// Add https:// prefix
		urlStr = "https://" + urlStr
	}

	// Convert ssh:// URLs to https://
	// Format: ssh://git@github.com/owner/repo.git -> https://github.com/owner/repo
	if strings.HasPrefix(urlStr, "ssh://") {
		urlStr = strings.TrimPrefix(urlStr, "ssh://")
		urlStr = strings.TrimPrefix(urlStr, "git@")
		urlStr = "https://" + urlStr
	}

	// Trim all trailing slashes
	urlStr = strings.TrimRight(urlStr, "/")
	return urlStr
}

// hasAIRulezStructure checks if a directory contains ai-rulez structure
func (s *GitSource) hasAIRulezStructure(dir string) bool {
	checkDirs := []string{rulesSubdir, contextSubdir, "skills", "agents"}
	for _, subdir := range checkDirs {
		checkPath := filepath.Join(dir, subdir)
		if isRealDir(checkPath) {
			return true
		}
	}
	return false
}
