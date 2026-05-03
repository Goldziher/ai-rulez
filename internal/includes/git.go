package includes

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

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

// getIncludeCacheDir returns the cache directory for a given include source.
// Uses ~/.cache/ai-rulez/ (XDG convention) for consistent cross-platform behavior.
func getIncludeCacheDir(sourceName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.TempDir()
	}

	cacheDir := filepath.Join(homeDir, ".cache", "ai-rulez", "includes", sourceName)
	return cacheDir, nil
}

// GitSource represents a git repository source
type GitSource struct {
	name        string
	repoURL     string // Normalized HTTPS URL
	originalURL string // Original URL (may be SSH format)
	path        string // Path within repo to .ai-rulez/ (optional, defaults to root)
	ref         string // branch, tag, or commit (optional, defaults to main/master)
	cacheDir    string // ~/.cache/ai-rulez/includes/{name}/
	include     []string
	accessToken string
}

// NewGitSource creates a new git source
func NewGitSource(name, repoURL, path, ref, baseDir string, include []string, accessToken string) (*GitSource, error) {
	// Validate URL format
	if err := validateGitURL(repoURL); err != nil {
		return nil, err
	}

	// Create cache directory in system cache location
	cacheDir, err := getIncludeCacheDir(name)
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
	}

	return source, nil
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
	logger.Debug("Fetching git source", "name", s.name, "repo", s.repoURL, "ref", s.ref, "path", s.path, "has_token", s.accessToken != "")

	if SkipFetch {
		if s.findAIRulezDir() == "" {
			return nil, oops.
				With("repo", s.repoURL).
				With("cache_dir", s.cacheDir).
				Errorf("--no-fetch specified but no cached content found for include '%s'", s.name)
		}
		logger.Debug("Skipping fetch (--no-fetch), using cached content", "name", s.name)
		return s.scanCachedContent()
	}

	currentSHA, err := remoteHEADSHA(ctx, s.originalURL, s.resolvedRef(), s.accessToken)
	if err != nil {
		meta, metaErr := readCacheMeta(s.cacheDir)
		if metaErr == nil && meta != nil {
			logger.Warn("ls-remote failed, using cached content", "name", s.name, "error", err)
			return s.scanCachedContent()
		}
		return nil, oops.Wrapf(err, "failed to get remote HEAD for include %q", s.name)
	}

	// Fast path: cache SHA matches — touch FetchedAt and return without acquiring the write lock.
	if isCacheHit(s.cacheDir, currentSHA) {
		meta, _ := readCacheMeta(s.cacheDir) //nolint:errcheck // best-effort; cache hit already confirmed
		if meta != nil {
			meta.FetchedAt = time.Now()
			_ = writeCacheMeta(s.cacheDir, meta) //nolint:errcheck // best-effort timestamp update
		}
		return s.scanCachedContent()
	}

	// Slow path: cache is stale or missing. Serialize the refresh per cache directory.
	mu := lockForFetch(s.cacheDir)
	mu.Lock()
	defer mu.Unlock()

	// Double-check under the lock — another goroutine may have refreshed while we waited.
	if isCacheHit(s.cacheDir, currentSHA) {
		return s.scanCachedContent()
	}

	// Invalidate any memoized scan since the on-disk tree is about to change.
	if dir := s.findAIRulezDir(); dir != "" {
		invalidateScan(dir)
	}
	if err := os.RemoveAll(s.cacheDir); err != nil {
		logger.Warn("Failed to clear include cache", "cache_dir", s.cacheDir, "error", err)
	}
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return nil, oops.
			With("cache_dir", s.cacheDir).
			Wrapf(err, "failed to create cache directory")
	}

	if err := requireGit(ctx); err != nil {
		return nil, err
	}
	pathSpec := s.sparsePathSpec()
	if err := sparseClone(ctx, s.originalURL, s.resolvedRef(), pathSpec, s.cacheDir, s.accessToken); err != nil {
		return nil, oops.With("repo", s.repoURL).Wrapf(err, "failed to clone include %q", s.name)
	}

	hashes, _ := computeFileHashes(s.cacheDir) //nolint:errcheck // best-effort; missing hashes degrade to full refetch next run
	_ = writeCacheMeta(s.cacheDir, &CacheMeta{ //nolint:errcheck // best-effort; failing to persist meta causes a refetch next run
		RemoteHEADSHA: currentSHA,
		FetchedAt:     time.Now(),
		FileHashes:    hashes,
	})

	return s.scanCachedContent()
}

// scanCachedContent locates the .ai-rulez directory in the cache and returns its content tree.
func (s *GitSource) scanCachedContent() (*config.ContentTree, error) {
	// Find the .ai-rulez directory in the extracted content
	aiRulezDir := s.findAIRulezDir()
	if aiRulezDir == "" {
		return nil, oops.
			With("repo", s.repoURL).
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
		scanned, err := config.ScanContentTree(aiRulezDir)
		if err != nil {
			return nil, oops.
				With("repo", s.repoURL).
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
	// Check for .ai-rulez in the cache directory
	// It could be at the root level or under the specified path

	aiRulezPath := filepath.Join(s.cacheDir, aiRulezDir)
	if info, err := os.Stat(aiRulezPath); err == nil && info.IsDir() {
		return aiRulezPath
	}

	// If path was specified, check if .ai-rulez is under that path
	if s.path != "" && s.path != rootPath {
		cleanPath := strings.Trim(s.path, rootPath)
		aiRulezPath := filepath.Join(s.cacheDir, cleanPath, aiRulezDir)
		if info, err := os.Stat(aiRulezPath); err == nil && info.IsDir() {
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
			With("url", urlStr).
			Hint("Git repository URLs must use http://, https://, file://, git@, or ssh:// protocol").
			Errorf("invalid git repository URL format")
	}

	// Try to parse as URL
	if _, err := url.Parse(urlStr); err != nil {
		return oops.
			With("url", urlStr).
			Wrapf(err, "invalid URL format")
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
		if info, err := os.Stat(checkPath); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
