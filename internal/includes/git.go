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
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
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

// getIncludeCacheDir isolates repository, sparse path and the ref the cache is
// keyed by so one project cannot overwrite another project's pinned cache. ~keep
func getIncludeCacheDir(host ambient.Host, sourceName, repoURL, path, ref string) (string, error) {
	sum := sha256.Sum256([]byte(normalizeGitURL(repoURL) + "\x00" + path + "\x00" + ref))
	dir := SafeCacheName(sourceName) + "-" + hex.EncodeToString(sum[:])[:cacheHashLen]
	return config.CacheDirIn(host.Env, "includes", dir) //nolint:wrapcheck // already contextual
}

// pinnedIncludeCacheDir is the cache directory of an include the lock pins to
// commit: one tree per commit, so a checkout pinning another commit can neither
// read nor overwrite it (#524).
//
// A cache written before the commit was part of the identity — by an older
// ai-rulez, or by the `lock` run that wrote the pin — lives in the ref-keyed
// directory. It is still used while it holds exactly the pinned commit, so
// already-populated caches are not discarded; a directory holding another
// commit is never read through it.
func pinnedIncludeCacheDir(host ambient.Host, sourceName, repoURL, path, ref, commit string) (string, error) {
	if commit == "" {
		return getIncludeCacheDir(host, sourceName, repoURL, path, ref) //nolint:wrapcheck // already contextual
	}
	pinned, err := getIncludeCacheDir(host, sourceName, repoURL, path, commit)
	if err != nil || isCacheDir(pinned) {
		return pinned, err //nolint:wrapcheck // already contextual
	}
	legacy, err := getIncludeCacheDir(host, sourceName, repoURL, path, ref)
	if err != nil {
		return "", err //nolint:wrapcheck // already contextual
	}
	if cachedCommit(legacy) == commit {
		return legacy, nil
	}
	return pinned, nil
}

// isCacheDir reports whether dir is an existing directory.
func isCacheDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// SafeCacheName keeps a source name usable as one path segment.
func SafeCacheName(name string) string {
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

// cacheHashLen fixes the cache-name suffix recognized by CacheDirMatches. ~keep
const cacheHashLen = 12

// CacheDirMatches reports whether seg is the cache directory name of the
// include called name: SafeCacheName(name), a dash and a lower-case hex hash of
// exactly cacheHashLen characters. A prefix match is not enough: the include
// "shared" must not claim the directory of "shared-extra".
func CacheDirMatches(seg, name string) bool {
	hash, ok := strings.CutPrefix(seg, SafeCacheName(name)+"-")
	if !ok || len(hash) != cacheHashLen {
		return false
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// GitSource represents a git repository source
type GitSource struct {
	name        string
	repoURL     string // Normalized HTTPS URL
	originalURL string // Original URL (may be SSH format)
	path        string // Path within repo to .ai-rulez/ (optional, defaults to root)
	ref         string // Fetch ref: the locked commit when pinned, else the configured ref. ~keep
	cacheDir    string // ~/.cache/ai-rulez/includes/{name}-{source hash}/
	include     []string
	accessToken string
	pin         *pin              // ai-rulez.lock entry this source must match (nil: unpinned)
	baseDir     string            // project the include belongs to, for recording what it resolved to
	okf         bool              // the repository holds an OKF bundle (at path) instead of an .ai-rulez directory
	okfScan     okfbridge.Scanner // security scan for the OKF conversion (nil skips it)
	state       *resolutionState  // per-load records of what this fetch resolved to
	log         logger.Logger
}

// logger is the source's log: the host's of the config it was built for, the CLI's when none.
func (s *GitSource) logger() logger.Logger { return logger.Or(s.log) }

// NewGitSource creates a new git source
func NewGitSource(name, repoURL, path, ref, baseDir string, include []string, accessToken string) (*GitSource, error) {
	return NewGitSourceIn(ambient.Host{}, name, repoURL, path, ref, baseDir, include, accessToken)
}

// NewGitSourceIn is NewGitSource with the environment (the cache location) taken
// from host instead of the process.
func NewGitSourceIn(host ambient.Host, name, repoURL, path, ref, baseDir string, include []string, accessToken string) (*GitSource, error) {
	return newGitSource(host, name, repoURL, path, ref, "", baseDir, include, accessToken)
}

// newGitSource builds a git source. commit is the commit the lock pins ("" when
// the source is not pinned): it keys the cache, so two checkouts pinning
// different commits of one include never share a directory (#524).
func newGitSource(host ambient.Host, name, repoURL, path, ref, commit, baseDir string, include []string, accessToken string) (*GitSource, error) {
	repoURL = stripGitPlus(repoURL)
	// Validate URL format
	if err := validateGitURL(repoURL); err != nil {
		return nil, err
	}

	// Create cache directory in system cache location
	cacheDir, err := pinnedIncludeCacheDir(host, name, repoURL, path, ref, commit)
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
		log:         host.Log,
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
	return newOKFGitSource(host, name, repoURL, path, ref, "", baseDir, include, accessToken)
}

// newOKFGitSource builds an OKF git source, commit being the commit the lock pins
// (see [newGitSource]).
func newOKFGitSource(host ambient.Host, name, repoURL, path, ref, commit, baseDir string, include []string, accessToken string) (*GitSource, error) {
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return nil, oops.With("include", name).Errorf("invalid OKF bundle path %q", path)
		}
	}
	s, err := newGitSource(host, name, repoURL, path, ref, commit, baseDir, include, accessToken)
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
// The whole fetch, including scanning and digesting, runs under a per-cacheDir
// mutex so refreshes of one source cannot replace files mid-read. Independent
// source identities do not wait for each other. A sibling advisory lock also
// serializes separate CLI processes sharing the same cache. ~keep
func (s *GitSource) Fetch(ctx context.Context) (*config.ContentTree, error) {
	mu := lockForFetch(s.cacheDir)
	mu.Lock()
	defer mu.Unlock()

	release, err := lockFetchCache(ctx, s.cacheDir)
	if err != nil {
		return nil, err
	}
	defer release()

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
	return s.pin.check(s.state, s.baseDir, lockfile.KindInclude, s.name, commit, digest)
}

func (s *GitSource) fetch(ctx context.Context) (*config.ContentTree, error) {
	if s.okf {
		ctx = withHardenedGit(ctx)
	}
	s.logger().Debug("Fetching git source", "name", s.name, "repo", RedactURL(s.repoURL), "ref", s.ref, "path", s.path, "has_token", s.accessToken != "")

	if config.OfflineIncludes(ctx) {
		if s.findAIRulezDir() == "" {
			return nil, oops.
				With("repo", RedactURL(s.repoURL)).
				With("cache_dir", s.cacheDir).
				Wrapf(ErrNotCached, "%s: no cached content found for include '%s'", offlineReason(ctx), s.name)
		}
		s.logger().Debug("Skipping fetch (--no-fetch), using cached content", "name", s.name)
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
			s.logger().Warn("ls-remote failed, using cached content", "name", s.name, "error", err)
			return s.scanCachedContent(ctx)
		}
		return nil, err
	}

	// The cache is current. Nothing was fetched, so nothing is written:
	// a read-only load must not touch the cache metadata (FetchedAt records the
	// last real fetch, and nothing reads it as a last-used time).
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
		s.logger().Warn("Failed to clear include cache", "cache_dir", s.cacheDir, "error", err)
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
		return convertOKFBundle(ctx, dir, s.name, s.include, s.okfScan)
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

	s.logger().Debug("Found .ai-rulez directory", "path", aiRulezDir)

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
		if scanned.ImportedVerifiers, err = config.ScanVerifierFiles(aiRulezDir); err != nil {
			return nil, oops.Wrapf(err, "failed to scan verifiers")
		}
		storeScan(aiRulezDir, scanned)
		contentTree = scanned
	} else {
		s.logger().Debug("Reusing cached content tree scan", "path", aiRulezDir)
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
		if isRealDir(s.logger(), dir) {
			return dir
		}
		return ""
	}
	// Check for .ai-rulez in the cache directory
	// It could be at the root level or under the specified path

	aiRulezPath := filepath.Join(s.cacheDir, aiRulezDir)
	if isRealDir(s.logger(), aiRulezPath) {
		return aiRulezPath
	}

	// If path was specified, check if .ai-rulez is under that path
	if s.path != "" && s.path != rootPath {
		cleanPath := strings.Trim(s.path, rootPath)
		aiRulezPath := filepath.Join(s.cacheDir, cleanPath, aiRulezDir)
		if isRealDir(s.logger(), aiRulezPath) {
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

	// A credential in the URL would be stored in clear in config.toml and passed
	// on git's command line (and into the clone's .git/config). Refuse it and
	// point at the token that is kept out of both.
	if HasCredentials(urlStr) {
		return oops.
			With("url", RedactURL(urlStr)).
			Hint("Remove the credential from the URL and authenticate with the git credential helper or the AI_RULEZ_GIT_TOKEN environment variable (ai-rulez sends it as a scoped git header, never stored)").
			Errorf("the source URL embeds a credential")
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
		if isRealDir(s.logger(), checkPath) {
			return true
		}
	}
	return false
}
