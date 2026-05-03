package includes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

const (
	skillCachePrefix = "skills"
	skillMarkerFile  = "SKILL.md"
)

// getSkillCacheDir returns the cache directory for a given installed skill.
// Uses ~/.cache/ai-rulez/ (XDG convention) for consistent cross-platform behavior.
func getSkillCacheDir(skillName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.TempDir()
	}
	cacheDir := filepath.Join(homeDir, ".cache", "ai-rulez", skillCachePrefix, skillName)
	return cacheDir, nil
}

// SkillGitSource fetches a skill from a git repository.
// Unlike GitSource (which expects .ai-rulez/ structure), this looks for
// a skill directory (SKILL.md + optional references/) at the configured path.
type SkillGitSource struct {
	name        string
	repoURL     string
	originalURL string
	path        string // path within repo to skill directory (e.g., "skills/kreuzberg")
	ref         string
	cacheDir    string
	accessToken string
}

// NewSkillGitSource creates a new SkillGitSource for fetching a skill from a git repo
func NewSkillGitSource(name, repoURL, path, ref, accessToken string) (*SkillGitSource, error) {
	if err := validateGitURL(repoURL); err != nil {
		return nil, err
	}

	cacheDir, err := getSkillCacheDir(name)
	if err != nil {
		return nil, oops.With("skill_name", name).Wrapf(err, "failed to determine cache directory")
	}

	return &SkillGitSource{
		name:        name,
		repoURL:     normalizeGitURL(repoURL),
		originalURL: repoURL,
		path:        path,
		ref:         ref,
		cacheDir:    cacheDir,
		accessToken: accessToken,
	}, nil
}

// resolvedRef returns s.ref or "HEAD" when empty.
func (s *SkillGitSource) resolvedRef() string {
	if s.ref != "" {
		return s.ref
	}
	return refHead
}

// sparsePathSpec returns the path spec to pass to sparseClone.
func (s *SkillGitSource) sparsePathSpec() string {
	return strings.TrimPrefix(s.path, "/") + "/"
}

// Fetch downloads the repo and extracts the skill content.
//
// Safe for concurrent invocation: serialized per cache directory so
// parallel callers targeting the same skill share a single download.
// Unlike GitSource.Fetch, the lock is acquired first (before ls-remote)
// since skills always check freshness under the lock.
func (s *SkillGitSource) Fetch(ctx context.Context) (config.ContentFile, error) {
	mu := lockForFetch(s.cacheDir)
	mu.Lock()
	defer mu.Unlock()

	logger.Debug("Fetching installed skill", "name", s.name, "repo", s.repoURL, "path", s.path, "ref", s.ref)

	if SkipFetch {
		skillDir := s.findSkillDir()
		if skillDir == "" {
			return config.ContentFile{}, oops.
				With("repo", s.repoURL).
				With("cache_dir", s.cacheDir).
				Errorf("--no-fetch specified but no cached skill found for '%s'", s.name)
		}
		return ScanInstalledSkillDir(skillDir, s.name)
	}

	currentSHA, err := remoteHEADSHA(ctx, s.originalURL, s.resolvedRef(), s.accessToken)
	if err != nil {
		if skillDir := s.findSkillDir(); skillDir != "" {
			logger.Warn("ls-remote failed, using cached skill", "name", s.name, "error", err)
			return ScanInstalledSkillDir(skillDir, s.name)
		}
		return config.ContentFile{}, oops.With("repo", s.repoURL).Wrapf(err, "failed to get remote HEAD for skill %q", s.name)
	}

	if isCacheHit(s.cacheDir, currentSHA) {
		if skillDir := s.findSkillDir(); skillDir != "" {
			return ScanInstalledSkillDir(skillDir, s.name)
		}
		// Meta is valid but files are gone — fall through to re-fetch.
	}

	if err := os.RemoveAll(s.cacheDir); err != nil {
		logger.Warn("Failed to clear skill cache", "cache_dir", s.cacheDir, "error", err)
	}
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return config.ContentFile{}, oops.With("cache_dir", s.cacheDir).Wrapf(err, "failed to create cache directory")
	}

	if err := requireGit(ctx); err != nil {
		return config.ContentFile{}, err
	}
	if err := sparseClone(ctx, s.originalURL, s.resolvedRef(), s.sparsePathSpec(), s.cacheDir, s.accessToken); err != nil {
		return config.ContentFile{}, oops.
			With("repo", s.repoURL).
			With("path", s.path).
			Wrapf(err, "failed to clone skill %q", s.name)
	}

	hashes, _ := computeFileHashes(s.cacheDir) //nolint:errcheck // best-effort; missing hashes degrade to full refetch next run
	_ = writeCacheMeta(s.cacheDir, &CacheMeta{ //nolint:errcheck // best-effort; failing to persist meta causes a refetch next run
		RemoteHEADSHA: currentSHA,
		FetchedAt:     time.Now(),
		FileHashes:    hashes,
	})

	skillDir := s.findSkillDir()
	if skillDir == "" {
		return config.ContentFile{}, oops.
			With("repo", s.repoURL).
			With("path", s.path).
			Errorf("skill directory not found (expected SKILL.md at %s)", s.path)
	}
	return ScanInstalledSkillDir(skillDir, s.name)
}

// findSkillDir locates the skill directory in the cached repo content
func (s *SkillGitSource) findSkillDir() string {
	// Check at the configured path
	if s.path != "" {
		skillDir := filepath.Join(s.cacheDir, s.path)
		if hasSkillMarker(skillDir) {
			return skillDir
		}

		// Also check under .ai-rulez/ path in case the clone extracted there
		skillDir = filepath.Join(s.cacheDir, aiRulezDir, s.path)
		if hasSkillMarker(skillDir) {
			return skillDir
		}
	}

	// Fallback: check cache root
	if hasSkillMarker(s.cacheDir) {
		return s.cacheDir
	}

	return ""
}

// hasSkillMarker checks if a directory contains SKILL.md
func hasSkillMarker(dir string) bool {
	skillPath := filepath.Join(dir, skillMarkerFile)
	_, err := os.Stat(skillPath)
	return err == nil
}

// ScanInstalledSkillDir reads a skill directory (SKILL.md plus optional
// references/, scripts/, and assets/ subdirectories) and returns a single
// ContentFile. SKILL.md becomes the body; the supporting subdirectories are
// loaded into Resources so generators can preserve the canonical Agent Skills
// layout in their output rather than concatenating everything inline.
func ScanInstalledSkillDir(skillDir, skillName string) (config.ContentFile, error) {
	skillPath := filepath.Join(skillDir, skillMarkerFile)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return config.ContentFile{}, oops.With("path", skillPath).Wrapf(err, "read SKILL.md")
	}

	metadata, body := config.ParseFrontmatterPublic(string(data))

	resources, err := config.LoadSkillResources(skillDir)
	if err != nil {
		// Resources are optional — log but don't fail the skill load.
		logger.Warn("Failed to read skill resources", "skill", skillName, "error", err)
	}

	return config.ContentFile{
		Name:      skillName,
		Path:      skillPath,
		Content:   body,
		Metadata:  metadata,
		Resources: resources,
	}, nil
}
