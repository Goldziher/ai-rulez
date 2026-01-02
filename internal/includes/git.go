package includes

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/remote"
	"github.com/Goldziher/ai-rulez/internal/scanner"
	"github.com/samber/oops"
)

const (
	gitHubHost    = "github.com"
	gitLabHost    = "gitlab.com"
	gitSuffix     = ".git"
	defaultRef    = "main"
	rootPath      = "/"
	rawGitHubURL  = "https://raw.githubusercontent.com/%s/%s/%s%s"
	rawGitLabURL  = "https://gitlab.com/%s/%s/-/raw/%s%s"
	archGitHubURL = "https://github.com/%s/%s/archive/refs/heads/%s.tar.gz"
	archGitLabURL = "https://gitlab.com/%s/%s/-/archive/%s/archive.tar.gz"
	aiRulezDir    = ".ai-rulez"
)

// GitSource represents a git repository source
type GitSource struct {
	name       string
	repoURL    string
	path       string // Path within repo to .ai-rulez/ (optional, defaults to root)
	ref        string // branch, tag, or commit (optional, defaults to main/master)
	cacheDir   string // .ai-rulez/.remote-cache/{name}/
	include    []string
	httpClient *remote.Client
}

// NewGitSource creates a new git source
func NewGitSource(name, repoURL, path, ref, baseDir string, include []string) (*GitSource, error) {
	// Validate URL format
	if err := validateGitURL(repoURL); err != nil {
		return nil, err
	}

	// Create cache directory if not exists
	cacheDir := filepath.Join(baseDir, ".remote-cache", name)

	source := &GitSource{
		name:       name,
		repoURL:    normalizeGitURL(repoURL),
		path:       path,
		ref:        ref,
		cacheDir:   cacheDir,
		include:    include,
		httpClient: remote.NewClient(nil),
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

// Fetch downloads content from git repository and returns the content tree
func (s *GitSource) Fetch(ctx context.Context) (*config.ContentTreeV3, error) {
	logger.Debug("Fetching git source", "name", s.name, "repo", s.repoURL, "ref", s.ref, "path", s.path)

	// Ensure cache directory exists
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return nil, oops.
			With("cache_dir", s.cacheDir).
			Wrapf(err, "failed to create cache directory")
	}

	// Build raw URL for the repository content
	rawURL, err := s.buildRawURL()
	if err != nil {
		return nil, err
	}

	logger.Debug("Built raw URL", "url", rawURL)

	// Download and extract archive
	if err := s.downloadAndExtract(ctx, rawURL); err != nil {
		return nil, err
	}

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

	// Create a minimal config for the scanner
	minimalConfig := &config.ConfigV3{
		Version: "3.0",
		Name:    s.name,
		BaseDir: filepath.Dir(aiRulezDir),
		Profiles: map[string][]string{
			"default": {}, // Empty default profile
		},
	}

	// Scan the .ai-rulez directory structure
	scnr := scanner.NewScanner(filepath.Dir(aiRulezDir), minimalConfig)
	contentTree, err := scnr.ScanProfile("") // Scan all content, no profile filter
	if err != nil {
		return nil, oops.
			With("repo", s.repoURL).
			Wrapf(err, "failed to scan content tree")
	}

	// Filter content based on include list if specified
	if len(s.include) > 0 {
		contentTree = s.filterContent(contentTree)
	}

	return contentTree, nil
}

// buildRawURL converts repository URL to raw content URL
func (s *GitSource) buildRawURL() (string, error) {
	// Normalize the URL
	repoURL := s.repoURL

	// Remove .git suffix if present
	repoURL = strings.TrimSuffix(repoURL, gitSuffix)

	// Determine ref (default to main if not specified)
	ref := s.ref
	if ref == "" {
		ref = defaultRef
	}

	// Determine the path within the repo
	path := s.path
	if path == "" {
		path = rootPath
	}
	// Ensure path starts with / for URL construction
	if !strings.HasPrefix(path, rootPath) {
		path = rootPath + path
	}

	// GitHub raw URL format
	if strings.Contains(repoURL, gitHubHost) {
		// Extract owner/repo from URL
		// Format: https://github.com/owner/repo
		parts := strings.Split(strings.TrimSuffix(repoURL, rootPath), rootPath)
		if len(parts) < 2 {
			return "", oops.
				With("url", s.repoURL).
				Errorf("invalid GitHub URL format")
		}
		owner := parts[len(parts)-2]
		repo := parts[len(parts)-1]

		return fmt.Sprintf(rawGitHubURL, owner, repo, ref, path), nil
	}

	// GitLab raw URL format
	if strings.Contains(repoURL, gitLabHost) {
		// Extract owner/repo from URL
		// Format: https://gitlab.com/owner/repo
		parts := strings.Split(strings.TrimSuffix(repoURL, rootPath), rootPath)
		if len(parts) < 2 {
			return "", oops.
				With("url", s.repoURL).
				Errorf("invalid GitLab URL format")
		}
		owner := parts[len(parts)-2]
		repo := parts[len(parts)-1]

		// GitLab uses /-/raw/ for raw content access
		return fmt.Sprintf(rawGitLabURL, owner, repo, ref, path), nil
	}

	// Generic git server support
	// Assume raw URL format similar to GitHub
	parts := strings.Split(strings.TrimSuffix(repoURL, "/"), "/")
	if len(parts) < 2 {
		return "", oops.
			With("url", s.repoURL).
			Errorf("invalid git URL format")
	}
	owner := parts[len(parts)-2]
	repo := parts[len(parts)-1]

	return fmt.Sprintf("%s/%s/%s/%s%s", repoURL, owner, repo, ref, path), nil
}

// downloadAndExtract downloads the repository archive and extracts it to cache
func (s *GitSource) downloadAndExtract(ctx context.Context, rawURL string) error {
	// For now, we'll fetch the directory listing or tarball
	// GitHub provides /archive/refs/heads/{ref}.tar.gz or .zip
	// GitLab provides /archive/ref/archive.tar.gz or archive.zip

	// Modify URL to get the archive download
	archiveURL, err := s.buildArchiveURL()
	if err != nil {
		return err
	}

	logger.Debug("Downloading archive", "url", archiveURL)

	// Fetch the archive
	body, err := s.httpClient.Fetch(ctx, archiveURL)
	if err != nil {
		return oops.
			With("url", archiveURL).
			With("repo", s.repoURL).
			Wrapf(err, "failed to download archive")
	}

	// Detect archive format and extract
	switch {
	case strings.HasSuffix(archiveURL, ".tar.gz"):
		if err := s.extractTarGz(body); err != nil {
			return err
		}
	case strings.HasSuffix(archiveURL, ".zip"):
		if err := s.extractZip(body); err != nil {
			return err
		}
	default:
		return oops.
			With("url", archiveURL).
			Errorf("unsupported archive format")
	}

	return nil
}

// buildArchiveURL constructs the URL to download a repository archive
func (s *GitSource) buildArchiveURL() (string, error) {
	repoURL := s.repoURL
	repoURL = strings.TrimSuffix(repoURL, gitSuffix)

	ref := s.ref
	if ref == "" {
		ref = defaultRef
	}

	// GitHub archive format
	if strings.Contains(repoURL, gitHubHost) {
		parts := strings.Split(strings.TrimSuffix(repoURL, rootPath), rootPath)
		if len(parts) < 2 {
			return "", oops.
				With("url", s.repoURL).
				Errorf("invalid GitHub URL format")
		}
		owner := parts[len(parts)-2]
		repo := parts[len(parts)-1]

		return fmt.Sprintf(archGitHubURL, owner, repo, ref), nil
	}

	// GitLab archive format
	if strings.Contains(repoURL, gitLabHost) {
		parts := strings.Split(strings.TrimSuffix(repoURL, rootPath), rootPath)
		if len(parts) < 2 {
			return "", oops.
				With("url", s.repoURL).
				Errorf("invalid GitLab URL format")
		}
		owner := parts[len(parts)-2]
		repo := parts[len(parts)-1]

		return fmt.Sprintf(archGitLabURL, owner, repo, ref), nil
	}

	return "", oops.
		With("url", s.repoURL).
		Errorf("unsupported git host")
}

// extractTarGz extracts a tar.gz archive to the cache directory
func (s *GitSource) extractTarGz(body []byte) error {
	// gosec: G110 - decompression bomb check is acceptable here as we control the source
	gr, err := gzip.NewReader(strings.NewReader(string(body))) // nolint: gosec
	if err != nil {
		return oops.Wrapf(err, "failed to create gzip reader")
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return oops.Wrapf(err, "failed to read tar header")
		}

		// Extract file path (strip top-level directory)
		parts := strings.SplitN(header.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		relPath := parts[1]

		targetPath := filepath.Join(s.cacheDir, relPath)

		// Check if this is a directory by inspecting Typeflag
		isDir := header.Typeflag == tar.TypeDir
		if isDir {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return oops.
					With("path", targetPath).
					Wrapf(err, "failed to create directory")
			}
		} else {
			// Create parent directory
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return oops.
					With("path", filepath.Dir(targetPath)).
					Wrapf(err, "failed to create parent directory")
			}

			// Extract file
			file, err := os.Create(targetPath)
			if err != nil {
				return oops.
					With("path", targetPath).
					Wrapf(err, "failed to create file")
			}
			if _, err := io.Copy(file, tr); err != nil { // nolint: gosec
				if err := file.Close(); err != nil {
					// Log but continue with the original error
					logger.Warn("failed to close file", "path", targetPath, "error", err)
				}
				return oops.
					With("path", targetPath).
					Wrapf(err, "failed to extract file")
			}
			if err := file.Close(); err != nil {
				return oops.
					With("path", targetPath).
					Wrapf(err, "failed to close file")
			}

			// Set permissions
			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil { // nolint: gosec
				return oops.
					With("path", targetPath).
					Wrapf(err, "failed to set file permissions")
			}
		}
	}

	return nil
}

// extractZip extracts a zip archive to the cache directory
func (s *GitSource) extractZip(body []byte) error {
	reader := strings.NewReader(string(body))
	zr, err := zip.NewReader(reader, int64(len(body)))
	if err != nil {
		return oops.Wrapf(err, "failed to create zip reader")
	}

	for _, file := range zr.File {
		// Extract file path (strip top-level directory)
		parts := strings.SplitN(file.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		relPath := parts[1]
		targetPath := filepath.Join(s.cacheDir, relPath)

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return oops.
					With("path", targetPath).
					Wrapf(err, "failed to create directory")
			}
		} else {
			if err := s.extractZipFile(file, targetPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// extractZipFile extracts a single file from a zip archive
func (s *GitSource) extractZipFile(file *zip.File, targetPath string) error {
	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return oops.
			With("path", filepath.Dir(targetPath)).
			Wrapf(err, "failed to create parent directory")
	}

	// Extract file
	rc, err := file.Open()
	if err != nil {
		return oops.
			With("path", file.Name).
			Wrapf(err, "failed to open file in archive")
	}

	targetFile, err := os.Create(targetPath)
	if err != nil {
		if err := rc.Close(); err != nil {
			logger.Warn("failed to close archive reader", "path", file.Name, "error", err)
		}
		return oops.
			With("path", targetPath).
			Wrapf(err, "failed to create file")
	}

	if _, err := io.Copy(targetFile, rc); err != nil { // nolint: gosec
		if err := targetFile.Close(); err != nil {
			logger.Warn("failed to close file", "path", targetPath, "error", err)
		}
		if err := rc.Close(); err != nil {
			logger.Warn("failed to close archive reader", "path", file.Name, "error", err)
		}
		return oops.
			With("path", targetPath).
			Wrapf(err, "failed to extract file")
	}

	if err := targetFile.Close(); err != nil {
		if err := rc.Close(); err != nil {
			logger.Warn("failed to close archive reader", "path", file.Name, "error", err)
		}
		return oops.
			With("path", targetPath).
			Wrapf(err, "failed to close file")
	}
	if err := rc.Close(); err != nil {
		return oops.
			With("path", file.Name).
			Wrapf(err, "failed to close archive reader")
	}

	// Set permissions
	if err := os.Chmod(targetPath, file.FileInfo().Mode()); err != nil {
		return oops.
			With("path", targetPath).
			Wrapf(err, "failed to set file permissions")
	}

	return nil
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
	}

	return ""
}

// filterContent filters content based on include list
func (s *GitSource) filterContent(tree *config.ContentTreeV3) *config.ContentTreeV3 {
	filtered := &config.ContentTreeV3{
		Domains: make(map[string]*config.DomainV3),
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
	if shouldInclude("rules") {
		filtered.Rules = tree.Rules
	}
	if shouldInclude("context") {
		filtered.Context = tree.Context
	}
	if shouldInclude("skills") {
		filtered.Skills = tree.Skills
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

	// Check for HTTP/HTTPS URLs
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return oops.
			With("url", urlStr).
			Hint("Git repository URLs must use http://, https://, git@, or ssh:// protocol").
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
