package includes

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// LocalSource represents a local file system source
type LocalSource struct {
	name    string
	path    string   // Absolute or relative path
	baseDir string   // Base directory for resolving relative paths
	include []string // Content types to include
	// v reads the project the include belongs to; the zero View reads the real
	// directory the include names.
	v workspace.View
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

// In reads the include through v (normally the loaded project's workspace)
// instead of the real file system.
func (s *LocalSource) In(v workspace.View) *LocalSource {
	s.v = v
	return s
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
func (s *LocalSource) Fetch(ctx context.Context) (*config.ContentTree, error) {
	// Resolve path (handle relative paths)
	resolvedPath, err := s.resolvePath()
	if err != nil {
		return nil, oops.Wrapf(err, "failed to resolve path")
	}

	logger.FromContext(ctx).Debug("Loading local source", "name", s.name, "path", resolvedPath)

	v := s.v.For(resolvedPath)

	// Validate path exists and is readable
	if err := s.validatePath(v, resolvedPath); err != nil {
		return nil, oops.Wrapf(err, "invalid path")
	}

	// Check if this is an .ai-rulez directory or contains one
	aiRulezPath := s.findAIRulezDir(v, logger.FromContext(ctx), resolvedPath)

	// Without an .ai-rulez directory the path is a bare structure (rules/,
	// agents/, ... directly in it) and is scanned in place.
	scanDir := aiRulezPath
	if scanDir == "" {
		logger.FromContext(ctx).Debug("No .ai-rulez directory found, using bare structure", "path", resolvedPath)
		scanDir = resolvedPath
	}

	// Scan the directory structure using the config loader's scanner which keeps
	// root content and domain content separate (avoids duplication in generated output)
	contentTree, err := config.ScanContentTreeIn(ctx, v, scanDir)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to scan content tree")
	}
	if contentTree.ImportedVerifiers, err = config.ScanVerifierFilesIn(v, scanDir); err != nil {
		return nil, oops.Wrapf(err, "failed to scan verifiers")
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
func (s *LocalSource) validatePath(v workspace.View, path string) error {
	// Check if path exists
	info, err := v.Stat(path)
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
func (s *LocalSource) findAIRulezDir(v workspace.View, log logger.Logger, path string) string {
	// Check if path itself is a .ai-rulez directory
	if filepath.Base(path) == aiRulezDir && isRealDirIn(v, log, path) {
		return path
	}

	// Check if path contains a .ai-rulez subdirectory
	aiRulezPath := filepath.Join(path, aiRulezDir)
	if isRealDirIn(v, log, aiRulezPath) {
		return aiRulezPath
	}

	return ""
}

// filterContent filters content based on include list
func (s *LocalSource) filterContent(tree *config.ContentTree) *config.ContentTree {
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
	if shouldInclude("checks") {
		filtered.Checks = tree.Checks
	}

	// Copy domains (domains always included if they exist)
	filtered.Domains = tree.Domains

	return filtered
}

// checkInsideProject refuses a local include path that resolves outside the
// project. A committed config can name any path (`/home/victim/.config`,
// `../victim`), and its files would be written into generated outputs; only the
// machine-local overlay (config.local.*) or the user scope may leave the project.
// Both paths are compared after symlinks are resolved, so a link inside the
// project does not smuggle an outside directory in.
func checkInsideProject(cfg *config.Config, baseDir, field, name, path string) error {
	if cfg != nil && (cfg.UserScope || overlaySetsField(cfg, "includes", name, field)) {
		return nil
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, abs)
	}
	v := viewFor(cfg, baseDir)
	abs = realPath(v, abs)
	project := realPath(v, baseDir)
	rel, err := filepath.Rel(project, abs)
	if err != nil || safefs.RelEscapes(rel) || filepath.IsAbs(rel) {
		return oops.With("name", name).With("path", abs).With("project", project).
			Hint("Put the path in config.local.toml (machine-local, not committed), declare the include in your user config, or copy the content into the project").
			Wrapf(config.ErrIncludeOutsideProject, "include %q: local path %s is outside the project %s; a local include in the project config must stay inside the project", name, abs, project)
	}
	return nil
}

// CheckLocalInsideProject is the check the resolver applies to a local include
// the project config declares: its source must resolve inside the project at
// baseDir. "include add" runs it before it writes the include, so the project
// cannot be left unloadable by an entry the loader would refuse.
func CheckLocalInsideProject(baseDir, name, source string) error {
	return checkInsideProject(nil, baseDir, "source", name, source)
}

// viewFor is the view the project of cfg is read through; without a config it is
// the real directory dir.
func viewFor(cfg *config.Config, dir string) workspace.View {
	if cfg != nil {
		if v := cfg.View(); v.W != nil {
			return v
		}
	}
	return workspace.OSView(dir)
}

// realPath returns p with the symlinks that can be resolved inside v's workspace
// resolved: a path that does not exist (yet) keeps its longest existing prefix
// resolved. A link that leaves the workspace comes back as its (outside) target,
// so the caller's containment check refuses it.
func realPath(v workspace.View, p string) string {
	p = filepath.Clean(p)
	rest := ""
	for cur := p; ; {
		resolved, err := v.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(resolved, rest)
		}
		var outside *workspace.OutsideError
		if errors.As(err, &outside) {
			return filepath.Join(outside.Target, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
