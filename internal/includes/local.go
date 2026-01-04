package includes

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/scanner"
	"github.com/samber/oops"
)

// LocalSource represents a local file system source
type LocalSource struct {
	name    string
	path    string   // Absolute or relative path
	baseDir string   // Base directory for resolving relative paths
	include []string // Content types to include
}

// NewLocalSource creates a new local source
func NewLocalSource(name, path, baseDir string, include []string) *LocalSource {
	return &LocalSource{
		name:    name,
		path:    path,
		baseDir: baseDir,
		include: include,
	}
}

// GetType returns the source type
func (s *LocalSource) GetType() SourceType {
	return SourceTypeLocal
}

// GetName returns the source name
func (s *LocalSource) GetName() string {
	return s.name
}

// Fetch loads content from the local file system
func (s *LocalSource) Fetch(ctx context.Context) (*config.ContentTreeV3, error) {
	// Resolve path (handle relative paths)
	resolvedPath, err := s.resolvePath()
	if err != nil {
		return nil, oops.Wrapf(err, "failed to resolve path")
	}

	logger.Debug("Loading local source", "name", s.name, "path", resolvedPath)

	// Validate path exists and is readable
	if err := s.validatePath(resolvedPath); err != nil {
		return nil, oops.Wrapf(err, "invalid path")
	}

	// Check if this is an .ai-rulez directory or contains one
	aiRulezPath := s.findAIRulezDir(resolvedPath)

	var baseDir string
	var needsCleanup bool
	var tempDir string

	if aiRulezPath != "" {
		// Found .ai-rulez directory - use its parent as base
		baseDir = filepath.Dir(aiRulezPath)
	} else {
		// No .ai-rulez directory - assume bare structure (agents/, rules/, etc. directly in resolvedPath)
		// This supports shared repositories and git includes that don't use .ai-rulez/ wrapping
		// Create a temporary directory with .ai-rulez symlink to work with the scanner
		logger.Debug("No .ai-rulez directory found, using bare structure", "path", resolvedPath)

		var err error
		tempDir, err = os.MkdirTemp("", "ai-rulez-include-*")
		if err != nil {
			return nil, oops.Wrapf(err, "create temp directory for bare structure")
		}
		needsCleanup = true

		// Create symlink .ai-rulez -> resolvedPath
		symlinkPath := filepath.Join(tempDir, ".ai-rulez")
		if err := os.Symlink(resolvedPath, symlinkPath); err != nil {
			if cleanupErr := os.RemoveAll(tempDir); cleanupErr != nil {
				logger.Warn("Failed to cleanup temp directory", "error", cleanupErr)
			}
			return nil, oops.Wrapf(err, "create symlink for bare structure")
		}

		baseDir = tempDir
	}

	// Cleanup temp directory if needed
	if needsCleanup {
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				logger.Warn("Failed to cleanup temp directory", "error", err)
			}
		}()
	}

	// Create a minimal config for the scanner
	minimalConfig := &config.ConfigV3{
		Version: "3.0",
		Name:    s.name,
		BaseDir: baseDir,
		Profiles: map[string][]string{
			"default": {}, // Empty default profile
		},
	}

	// Scan the directory structure
	scnr := scanner.NewScanner(baseDir, minimalConfig)
	contentTree, err := scnr.ScanProfile("") // Scan all content, no profile filter
	if err != nil {
		return nil, oops.Wrapf(err, "failed to scan content tree")
	}

	// Filter content based on include list if specified
	if len(s.include) > 0 {
		contentTree = s.filterContent(contentTree)
	}

	return contentTree, nil
}

// resolvePath resolves relative paths to absolute paths
func (s *LocalSource) resolvePath() (string, error) {
	path := s.path

	// If already absolute, return as-is
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	// Resolve relative to baseDir
	absPath := filepath.Join(s.baseDir, path)
	return filepath.Clean(absPath), nil
}

// validatePath checks if the path exists and is accessible
func (s *LocalSource) validatePath(path string) error {
	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return oops.Errorf("path does not exist: %s", path)
		}
		return oops.Wrapf(err, "failed to access path")
	}

	// Check if it's a directory
	if !info.IsDir() {
		return oops.Errorf("path is not a directory: %s", path)
	}

	return nil
}

// findAIRulezDir finds the .ai-rulez directory in the given path
// Returns the path to the .ai-rulez directory, or empty string if not found
func (s *LocalSource) findAIRulezDir(path string) string {
	// Check if path itself is a .ai-rulez directory
	if filepath.Base(path) == ".ai-rulez" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Check if path contains a .ai-rulez subdirectory
	aiRulezPath := filepath.Join(path, ".ai-rulez")
	if info, err := os.Stat(aiRulezPath); err == nil && info.IsDir() {
		return aiRulezPath
	}

	return ""
}

// filterContent filters content based on include list
func (s *LocalSource) filterContent(tree *config.ContentTreeV3) *config.ContentTreeV3 {
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
