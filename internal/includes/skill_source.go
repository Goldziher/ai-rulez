package includes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

const (
	skillCachePrefix = "skills"
	skillMarkerFile  = "SKILL.md"
)

// getSkillCacheDir returns the cache directory for an installed skill: its name
// plus a short hash of the normalized URL, so two projects that install
// different skills under one name never share (or overwrite) a cache.
func getSkillCacheDir(host ambient.Host, skillName, repoURL string) (string, error) {
	sum := sha256.Sum256([]byte(normalizeGitURL(repoURL)))
	dir := SafeCacheName(skillName) + "-" + hex.EncodeToString(sum[:])[:cacheHashLen]
	return config.CacheDirIn(host.Env, skillCachePrefix, dir) //nolint:wrapcheck // already contextual
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
	pin         *pin             // ai-rulez.lock entry this skill must match (nil: unpinned)
	baseDir     string           // project the skill belongs to, for recording what it resolved to
	state       *resolutionState // per-load records of what this fetch resolved to
	log         logger.Logger
}

// logger is the source's log: the host's of the config it was built for, the CLI's when none.
func (s *SkillGitSource) logger() logger.Logger { return logger.Or(s.log) }

// NewSkillGitSource creates a new SkillGitSource for fetching a skill from a git repo
func NewSkillGitSource(name, repoURL, path, ref, accessToken string) (*SkillGitSource, error) {
	return NewSkillGitSourceIn(ambient.Host{}, name, repoURL, path, ref, accessToken)
}

// NewSkillGitSourceIn is NewSkillGitSource with the environment (the cache
// location) taken from host instead of the process.
func NewSkillGitSourceIn(host ambient.Host, name, repoURL, path, ref, accessToken string) (*SkillGitSource, error) {
	repoURL = stripGitPlus(repoURL)
	if err := validateGitURL(repoURL); err != nil {
		return nil, err
	}

	cacheDir, err := getSkillCacheDir(host, name, repoURL)
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
		log:         host.Log,
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
	file, err := s.fetch(ctx)
	if err != nil {
		return config.ContentFile{}, err
	}
	err = s.checkPin()
	if retryable(ctx, err) {
		// The cache may be damaged: fetch the pinned commit again before failing.
		_ = os.Remove(filepath.Join(s.cacheDir, cacheMetaFile)) //nolint:errcheck // best-effort; a stale meta only skips the retry
		if file, err = s.fetch(ctx); err != nil {
			return config.ContentFile{}, err
		}
		err = s.checkPin()
	}
	if err != nil {
		return config.ContentFile{}, err
	}
	return file, nil
}

// checkPin records what the cache holds and verifies it against the lock.
func (s *SkillGitSource) checkPin() error {
	dir := s.findSkillDir()
	if dir == "" {
		return nil
	}
	digest, err := contentlock.DigestDir(contentlock.KindInstalledSkill, dir)
	if err != nil {
		return oops.With("skill", s.name).Wrapf(err, "digest skill content")
	}
	commit := ""
	if meta, metaErr := readCacheMeta(s.cacheDir); metaErr == nil && meta != nil {
		commit = meta.RemoteHEADSHA
	}
	return s.pin.check(s.state, s.baseDir, lockfile.KindSkill, s.name, commit, digest)
}

func (s *SkillGitSource) fetch(ctx context.Context) (config.ContentFile, error) {
	mu := lockForFetch(s.cacheDir)
	mu.Lock()
	defer mu.Unlock()

	s.logger().Debug("Fetching installed skill", "name", s.name, "repo", RedactURL(s.repoURL), "path", s.path, "ref", s.ref)

	if config.OfflineIncludes(ctx) {
		skillDir := s.findSkillDir()
		if skillDir == "" {
			return config.ContentFile{}, oops.
				With("repo", RedactURL(s.repoURL)).
				With("cache_dir", s.cacheDir).
				Wrapf(ErrNotCached, "%s: no cached skill found for '%s'", offlineReason(ctx), s.name)
		}
		return ScanInstalledSkillDir(logger.WithContext(ctx, s.log), skillDir, s.name)
	}

	// A full commit SHA is already the resolved commit — ls-remote never advertises
	// one. Use it verbatim so a pin that the remote cannot serve fails closed at
	// clone time instead of silently degrading to cached content (#167).
	ref := s.resolvedRef()
	isSHA := isFullSHA(ref)
	currentSHA := ref
	if !isSHA {
		var err error
		currentSHA, err = remoteHEADSHA(ctx, s.originalURL, ref, s.accessToken)
		if err != nil {
			if skillDir := s.findSkillDir(); skillDir != "" {
				s.logger().Warn("ls-remote failed, using cached skill", "name", s.name, "error", err)
				return ScanInstalledSkillDir(logger.WithContext(ctx, s.log), skillDir, s.name)
			}
			return config.ContentFile{}, oops.With("repo", RedactURL(s.repoURL)).Wrapf(err, "failed to get remote HEAD for skill %q", s.name)
		}
	}

	if isCacheHit(s.cacheDir, currentSHA) {
		if skillDir := s.findSkillDir(); skillDir != "" {
			return ScanInstalledSkillDir(logger.WithContext(ctx, s.log), skillDir, s.name)
		}
		// Meta is valid but files are gone — fall through to re-fetch.
	}

	if err := requireGit(ctx); err != nil {
		return config.ContentFile{}, err
	}

	// Clone into a private temp directory and swap it in, so a fetch never
	// deletes a directory another project may be reading.
	if err := os.MkdirAll(filepath.Dir(s.cacheDir), cacheDirMode); err != nil {
		return config.ContentFile{}, oops.With("cache_dir", s.cacheDir).Wrapf(err, "failed to create cache directory")
	}
	tmpDir, err := os.MkdirTemp(filepath.Dir(s.cacheDir), filepath.Base(s.cacheDir)+".tmp-*")
	if err != nil {
		return config.ContentFile{}, oops.With("cache_dir", s.cacheDir).Wrapf(err, "failed to create cache directory")
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort; a no-op after a successful swap

	if err := cloneFor(isSHA)(ctx, s.originalURL, ref, s.sparsePathSpec(), tmpDir, s.accessToken); err != nil {
		return config.ContentFile{}, oops.
			With("repo", RedactURL(s.repoURL)).
			With("path", s.path).
			Wrapf(err, "failed to clone skill %q", s.name)
	}

	hashes, _ := computeFileHashes(tmpDir) //nolint:errcheck // best-effort; missing hashes degrade to full refetch next run
	_ = writeCacheMeta(tmpDir, &CacheMeta{ //nolint:errcheck // best-effort; failing to persist meta causes a refetch next run
		RemoteHEADSHA: currentSHA,
		FetchedAt:     ambient.FromContext(ctx).Now(),
		FileHashes:    hashes,
	})
	if err := swapDir(tmpDir, s.cacheDir); err != nil {
		return config.ContentFile{}, oops.With("cache_dir", s.cacheDir).Wrapf(err, "failed to install skill cache")
	}

	skillDir := s.findSkillDir()
	if skillDir == "" {
		if link := symlinkedMarker(filepath.Join(s.cacheDir, s.path)); link != "" {
			return config.ContentFile{}, oops.
				With("repo", RedactURL(s.repoURL)).
				With("path", link).
				Errorf("skill %q: %s is a symlink, and symlinks are not followed", s.name, link)
		}
		return config.ContentFile{}, oops.
			With("repo", RedactURL(s.repoURL)).
			With("path", s.path).
			Errorf("skill directory not found (expected SKILL.md at %s)", s.path)
	}
	return ScanInstalledSkillDir(logger.WithContext(ctx, s.log), skillDir, s.name)
}

// swapDir replaces final with tmp. The old directory is renamed aside first so
// a concurrent reader sees either the old tree or the new one, never a partial.
func swapDir(tmp, final string) error {
	trash := ""
	if _, err := os.Lstat(final); err == nil {
		trash = final + ".old-" + filepath.Base(tmp)
		if err := os.Rename(final, trash); err != nil {
			return err //nolint:wrapcheck // wrapped by caller
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		if trash != "" {
			_ = os.Rename(trash, final) //nolint:errcheck // restore best-effort
		}
		return err //nolint:wrapcheck // wrapped by caller
	}
	if trash != "" {
		_ = os.RemoveAll(trash) //nolint:errcheck // best-effort cleanup
	}
	return nil
}

// findSkillDir locates the skill directory in the cached repo content
func (s *SkillGitSource) findSkillDir() string {
	// Check at the configured path
	if s.path != "" {
		skillDir := filepath.Join(s.cacheDir, s.path)
		if hasSkillMarkerUnder(s.logger(), s.cacheDir, skillDir) {
			return skillDir
		}

		// Also check under .ai-rulez/ path in case the clone extracted there
		skillDir = filepath.Join(s.cacheDir, aiRulezDir, s.path)
		if hasSkillMarkerUnder(s.logger(), s.cacheDir, skillDir) {
			return skillDir
		}
	}

	// Fallback: check cache root
	if hasSkillMarker(s.cacheDir) {
		return s.cacheDir
	}

	return ""
}

// hasSkillMarker reports whether dir holds a regular (non-symlink) SKILL.md.
// A symlinked SKILL.md could point at any local file, so it is never a skill.
func hasSkillMarker(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, skillMarkerFile))
	return err == nil && info.Mode().IsRegular()
}

// symlinkedMarker returns the path of dir's SKILL.md when it is a symlink, which
// is never followed, so the error can name it instead of claiming no SKILL.md
// exists. It is "" when SKILL.md is absent or a regular file.
func symlinkedMarker(dir string) string {
	p := filepath.Join(dir, skillMarkerFile)
	if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return p
	}
	return ""
}

// hasSkillMarkerUnder is hasSkillMarker for a directory below root, additionally
// requiring that no component between root and dir is a symlink.
func hasSkillMarkerUnder(log logger.Logger, root, dir string) bool {
	return !pathHasSymlink(log, root, dir) && hasSkillMarker(dir)
}

// pathHasSymlink reports whether any component of dir below root is a symlink.
func pathHasSymlink(log logger.Logger, root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return false
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		if info, err := os.Lstat(cur); err == nil && info.Mode()&os.ModeSymlink != 0 {
			log.Warn("Skipping symlinked path in installed skill; symlinks are not followed", "path", cur)
			return true
		}
	}
	return false
}

// ScanInstalledSkillDir reads a skill directory (SKILL.md plus optional
// references/, scripts/, and assets/ subdirectories) and returns a single
// ContentFile. SKILL.md becomes the body; the supporting subdirectories are
// loaded into Resources so generators can preserve the canonical Agent Skills
// layout in their output rather than concatenating everything inline.
func ScanInstalledSkillDir(ctx context.Context, skillDir, skillName string) (config.ContentFile, error) {
	log := logger.FromContext(ctx)
	skillPath := filepath.Join(skillDir, skillMarkerFile)
	// Lstat before reading: a symlinked SKILL.md would be read through to any
	// local file and rendered into the outputs.
	info, err := os.Lstat(skillPath)
	if err != nil {
		return config.ContentFile{}, oops.With("path", skillPath).Wrapf(err, "read SKILL.md")
	}
	if !info.Mode().IsRegular() {
		log.Warn("Refusing SKILL.md that is not a regular file; symlinks are not followed", "path", skillPath)
		return config.ContentFile{}, oops.With("path", skillPath).Errorf("SKILL.md at %s is a symlink or not a regular file", skillPath)
	}
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return config.ContentFile{}, oops.With("path", skillPath).Wrapf(err, "read SKILL.md")
	}

	metadata, body, malformed := config.ParseFrontmatterChecked(string(data))
	if malformed {
		log.Warn("Ignoring malformed YAML frontmatter in an installed skill", "skill", skillName, "path", skillPath)
	}

	resources, err := config.LoadSkillResourcesContext(ctx, skillDir)
	if err != nil {
		// Resources are optional — log but don't fail the skill load.
		log.Warn("Failed to read skill resources", "skill", skillName, "error", err)
	}

	// A skill may ship verifier declarations in verifiers/. They are read like an
	// include's, as data, and can never be trusted to run commands.
	shipped, err := config.ScanVerifierFiles(skillDir)
	if err != nil {
		log.Warn("Failed to read skill verifiers", "skill", skillName, "error", err)
	}
	for i := range shipped {
		shipped[i].Include, shipped[i].Skill = skillName, true
	}

	return config.ContentFile{
		Name:      skillName,
		Path:      skillPath,
		Content:   body,
		Metadata:  metadata,
		Resources: resources,
		Verifiers: shipped,
	}, nil
}
