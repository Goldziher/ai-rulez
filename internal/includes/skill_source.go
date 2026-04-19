package includes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

const (
	skillCachePrefix = "skills"
	skillMarkerFile  = "SKILL.md"
	referencesDir    = "references"
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
	isSSH       bool
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
		isSSH:       isSSHURL(repoURL),
	}, nil
}

// Fetch downloads the repo and extracts the skill content
func (s *SkillGitSource) Fetch(ctx context.Context) (config.ContentFile, error) {
	logger.Debug("Fetching installed skill", "name", s.name, "repo", s.repoURL, "path", s.path, "ref", s.ref)

	// Clear stale cache before downloading fresh content
	if err := os.RemoveAll(s.cacheDir); err != nil {
		logger.Warn("Failed to clear skill cache", "cache_dir", s.cacheDir, "error", err)
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return config.ContentFile{}, oops.With("cache_dir", s.cacheDir).Wrapf(err, "failed to create cache directory")
	}

	// Create a temporary GitSource to reuse the download/clone logic
	gitSource, err := NewGitSource(
		"skill-"+s.name,
		s.originalURL,
		"", // no path filter for git source - we handle path ourselves
		s.ref,
		s.cacheDir, // baseDir (not used for cache location since GitSource creates its own)
		nil,        // no include filter
		s.accessToken,
	)
	if err != nil {
		return config.ContentFile{}, oops.Wrapf(err, "failed to create git source for skill '%s'", s.name)
	}

	// Override the cache dir to use our skill-specific one
	gitSource.cacheDir = s.cacheDir

	if s.isSSH {
		if err := s.cloneViaGitForSkill(ctx); err != nil {
			return config.ContentFile{}, err
		}
	} else {
		archiveURL, err := gitSource.buildArchiveURL()
		if err != nil {
			return config.ContentFile{}, err
		}
		if err := gitSource.downloadAndExtract(ctx, archiveURL); err != nil {
			return config.ContentFile{}, err
		}
	}

	// Find the skill directory at the configured path
	skillDir := s.findSkillDir()
	if skillDir == "" {
		return config.ContentFile{}, oops.
			With("repo", s.repoURL).
			With("path", s.path).
			Errorf("skill directory not found (expected SKILL.md at %s)", s.path)
	}

	return ScanInstalledSkillDir(skillDir, s.name)
}

// cloneViaGitForSkill clones a repository via SSH and copies the skill directory
// to the cache. Unlike GitSource.cloneViaGit (which copies .ai-rulez/), this
// copies the skill path (e.g., skills/<name>/) so SKILL.md can be found.
func (s *SkillGitSource) cloneViaGitForSkill(ctx context.Context) error {
	tmpDir := filepath.Join(s.cacheDir, ".tmp-clone")

	if err := os.RemoveAll(tmpDir); err != nil {
		return oops.With("tmp_dir", tmpDir).Wrapf(err, "failed to remove existing temp directory")
	}

	ref := s.ref
	if ref == "" {
		ref = defaultRef
	}

	logger.Debug("Cloning git repository for skill", "url", s.originalURL, "ref", ref, "path", s.path)

	// nolint: gosec // G204: Command arguments are from validated config
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", ref, s.originalURL, tmpDir)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return oops.
			With("url", s.originalURL).
			With("ref", ref).
			With("output", string(output)).
			Wrapf(err, "failed to clone repository via git")
	}

	// Copy the skill path from the clone to the cache
	sourceDir := filepath.Join(tmpDir, s.path)
	if _, err := os.Stat(sourceDir); err != nil {
		if cleanupErr := os.RemoveAll(tmpDir); cleanupErr != nil {
			logger.Warn("failed to cleanup temp directory", "tmp_dir", tmpDir, "error", cleanupErr)
		}
		return oops.
			With("repo", s.originalURL).
			With("path", s.path).
			Errorf("skill directory not found in cloned repository at %s", s.path)
	}

	destDir := filepath.Join(s.cacheDir, s.path)
	if err := copyDir(sourceDir, destDir); err != nil {
		if cleanupErr := os.RemoveAll(tmpDir); cleanupErr != nil {
			logger.Warn("failed to cleanup temp directory", "tmp_dir", tmpDir, "error", cleanupErr)
		}
		return oops.With("source", sourceDir).With("dest", destDir).Wrapf(err, "failed to copy skill directory")
	}

	if err := os.RemoveAll(tmpDir); err != nil {
		logger.Warn("failed to cleanup temp directory", "tmp_dir", tmpDir, "error", err)
	}

	return nil
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

// ScanInstalledSkillDir reads a skill directory (SKILL.md + optional references/)
// and returns a single ContentFile with combined content.
func ScanInstalledSkillDir(skillDir, skillName string) (config.ContentFile, error) {
	skillPath := filepath.Join(skillDir, skillMarkerFile)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return config.ContentFile{}, oops.With("path", skillPath).Wrapf(err, "read SKILL.md")
	}

	content := string(data)

	// Parse frontmatter
	metadata, body := config.ParseFrontmatterPublic(content)

	// Read references directory if it exists
	refsDir := filepath.Join(skillDir, referencesDir)
	if info, err := os.Stat(refsDir); err == nil && info.IsDir() {
		refContent, err := readReferences(refsDir)
		if err != nil {
			logger.Warn("Failed to read skill references", "skill", skillName, "error", err)
		} else if refContent != "" {
			body = body + "\n" + refContent
		}
	}

	return config.ContentFile{
		Name:     skillName,
		Path:     skillPath,
		Content:  body,
		Metadata: metadata,
	}, nil
}

// readReferences reads all .md files from a references/ directory,
// sorted alphabetically, and concatenates them with headings.
func readReferences(refsDir string) (string, error) {
	entries, err := os.ReadDir(refsDir)
	if err != nil {
		return "", oops.With("path", refsDir).Wrapf(err, "read references directory")
	}

	// Collect .md files sorted by name
	var mdFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			mdFiles = append(mdFiles, entry.Name())
		}
	}
	sort.Strings(mdFiles)

	var sb strings.Builder
	for _, filename := range mdFiles {
		data, err := os.ReadFile(filepath.Join(refsDir, filename))
		if err != nil {
			logger.Warn("Failed to read reference file", "file", filename, "error", err)
			continue
		}

		refContent := strings.TrimSpace(string(data))
		if refContent == "" {
			continue
		}

		// Strip frontmatter from reference files
		_, refBody := config.ParseFrontmatterPublic(refContent)

		refName := strings.TrimSuffix(filename, ".md")
		sb.WriteString("\n## Reference: ")
		sb.WriteString(refName)
		sb.WriteString("\n\n")
		sb.WriteString(refBody)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
