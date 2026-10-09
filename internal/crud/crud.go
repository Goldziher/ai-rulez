package crud

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
	"github.com/samber/oops"
)

const (
	sourceTypeGit   = "git"
	sourceTypeLocal = "local"

	// PriorityDefault is the default priority when none is specified
	PriorityDefault = "medium"
)

// Operator defines the interface for CRUD operations
type Operator interface {
	// Domain operations
	AddDomain(ctx context.Context, req *AddDomainRequest) (*DomainResult, error)
	RemoveDomain(ctx context.Context, name string) error
	ListDomains(ctx context.Context) ([]DomainInfo, error)

	// Content operations
	AddRule(ctx context.Context, req *AddFileRequest) (*FileResult, error)
	AddContext(ctx context.Context, req *AddFileRequest) (*FileResult, error)
	AddSkill(ctx context.Context, req *AddFileRequest) (*FileResult, error)
	AddAgent(ctx context.Context, req *AddFileRequest) (*FileResult, error)
	AddCommand(ctx context.Context, req *AddFileRequest) (*FileResult, error)
	AddCheck(ctx context.Context, req *AddFileRequest) (*FileResult, error)
	RemoveFile(ctx context.Context, domain, ftype, name string) error
	ListFiles(ctx context.Context, domain, ftype string) ([]FileInfo, error)

	// Include operations (Phase 4)
	AddInclude(ctx context.Context, req *AddIncludeRequest) error
	RemoveInclude(ctx context.Context, name string) error
	ListIncludes(ctx context.Context) ([]IncludeInfo, error)

	// Profile operations (Phase 4)
	AddProfile(ctx context.Context, name string, domains []string) error
	RemoveProfile(ctx context.Context, name string) error
	SetDefaultProfile(ctx context.Context, name string) error
	ListProfiles(ctx context.Context) ([]ProfileInfo, error)

	// Installed skill operations
	InstallSkill(ctx context.Context, req *InstallSkillRequest) error
	UninstallSkill(ctx context.Context, name string) error
	ListInstalledSkills(ctx context.Context) ([]InstalledSkillInfo, error)

	// Read operations
	ReadFileContent(path string) (string, error)

	// Update operations (atomic overwrite)
	UpdateFile(ctx context.Context, domain, ftype, name, content, priority string, targets []string) (*FileResult, error)
}

// OperatorImpl implements the Operator interface
type OperatorImpl struct {
	baseDir    string // project directory that owns the config directory
	aiRulezDir string // config directory: .ai-rulez/ or .config/ai-rulez/
	dirName    string // slash-separated aiRulezDir below baseDir when chosen explicitly (NewOperatorAt); "" means discover
	filesMgr   *FileManager
	local      bool        // route config mutations to the config.local.* overlay (see Local)
	env        ambient.Env // environment for ~ and $VAR in local include paths; nil is the real one
	log        logger.Logger
}

// WithLogger returns a copy of the operator that reports to log instead of the
// CLI's logger.
func (op *OperatorImpl) WithLogger(log logger.Logger) *OperatorImpl {
	c := *op
	c.log = log
	return &c
}

// logger is where the operator reports what it changed: the CLI's unless WithLogger set one.
func (op *OperatorImpl) logger() logger.Logger { return logger.Or(op.log) }

// WithEnv returns a copy of the operator that reads the environment from env.
func (op *OperatorImpl) WithEnv(env ambient.Env) *OperatorImpl {
	c := *op
	c.env = env
	return &c
}

// NewOperator creates a new Operator for the given base directory. The config
// directory is resolved like config.LoadConfig: .ai-rulez/ first, then the
// project-level .config/ai-rulez/ convention.
func NewOperator(baseDir string) (*OperatorImpl, error) {
	dirName := config.ResolveConfigDirName(baseDir)
	if dirName == "" {
		dirName = ".ai-rulez"
	}
	aiRulezDir := filepath.Join(baseDir, filepath.FromSlash(dirName))

	// Verify .ai-rulez directory exists
	if _, err := os.Stat(aiRulezDir); err != nil {
		return nil, oops.
			With("path", aiRulezDir).
			Hint("Run 'ai-rulez init' to create the .ai-rulez/ directory structure.").
			Wrapf(err, ".ai-rulez directory not found")
	}

	return &OperatorImpl{
		baseDir:    baseDir,
		aiRulezDir: aiRulezDir,
		filesMgr:   NewFileManager(aiRulezDir),
	}, nil
}

// NewOperatorAt creates an Operator for exactly the config directory configDir
// (any name, absolute or relative), the one a global --config-dir or -C names.
// Nothing is discovered: a project with several config directories is
// disambiguated by the caller. The project directory is the one that owns it,
// skipping the generic .config/ wrapper.
func NewOperatorAt(configDir string) (*OperatorImpl, error) {
	abs, err := filepath.Abs(configDir)
	if err != nil {
		return nil, oops.With("path", configDir).Wrapf(err, "resolve config directory")
	}
	if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = oops.Errorf("not a directory")
		}
		return nil, oops.
			With("path", abs).
			Hint("Check --config-dir / -C, or run 'ai-rulez init' to create the directory structure.").
			Wrapf(statErr, "config directory not found")
	}
	base := config.ProjectBaseDir(abs)
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return nil, oops.With("path", abs).Wrapf(err, "resolve config directory")
	}
	return &OperatorImpl{
		baseDir:    base,
		aiRulezDir: abs,
		dirName:    filepath.ToSlash(rel),
		filesMgr:   NewFileManager(abs),
	}, nil
}

// load reads the project's configuration from the operator's config directory:
// the explicit one when the operator was opened with NewOperatorAt, else the
// discovered one under baseDir.
func (op *OperatorImpl) load(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
	if op.dirName != "" {
		return project.LoadDir(ctx, op.baseDir, op.dirName, opts...) //nolint:wrapcheck // already contextual
	}
	return project.Load(ctx, op.baseDir, opts...) //nolint:wrapcheck // already contextual
}

// Request and Response Types

// AddDomainRequest represents a request to create a new domain
type AddDomainRequest struct {
	Name        string // Domain name (required)
	Description string // Domain description (optional)
}

// DomainResult represents the result of a domain operation
type DomainResult struct {
	Name        string // Domain name
	Path        string // Full path to domain directory
	Description string // Domain description
	Created     bool   // Whether the domain was created
}

// DomainInfo represents information about a domain
type DomainInfo struct {
	Name        string // Domain name
	Path        string // Full path to domain directory
	Description string // Domain description (from .description file if present)
}

// AddFileRequest represents a request to add a rule, context, or skill file
type AddFileRequest struct {
	Domain      string   // Domain name (optional, uses root if empty)
	Type        string   // File type: rules, context, or skills
	Name        string   // File name (without .md extension)
	Description string   // Skill description (used for skills)
	Content     string   // File content (optional, uses template if empty)
	Priority    string   // Priority level: critical, high, medium, low
	Targets     []string // Target providers: claude, cursor, etc.
}

// FileResult represents the result of a file operation
type FileResult struct {
	Name     string // File name (without extension)
	FullPath string // Full path to file
	Type     string // File type (rules, context, skills)
	Domain   string // Domain name (empty if root)
}

// FileInfo represents information about a file
type FileInfo struct {
	Name     string   `json:"name"`               // File name (without extension)
	Path     string   `json:"path"`               // Full path to file
	Type     string   `json:"type"`               // File type (rules, context, skills)
	Domain   string   `json:"domain,omitempty"`   // Domain name (empty if root)
	Priority string   `json:"priority,omitempty"` // Priority level from metadata
	Targets  []string `json:"targets,omitempty"`  // Target providers from metadata
}

// DefaultPriority returns the default priority if not specified
func (req *AddFileRequest) DefaultPriority() string {
	if req.Priority == "" {
		return PriorityDefault
	}
	return req.Priority
}

// GetDomain returns the domain name, defaulting to empty for root
func (req *AddFileRequest) GetDomain() string {
	if req.Domain == "" {
		return ""
	}
	return req.Domain
}

// IsRootContent returns true if this is root-level content (no domain)
func (req *AddFileRequest) IsRootContent() bool {
	return req.Domain == ""
}

// Phase 4 Request Types - Include & Profile Operations

// AddIncludeRequest represents a request to add an include source
type AddIncludeRequest struct {
	Name          string   // Include name (required)
	Source        string   // Git URL or local path (required)
	Path          string   // Path within git repo (git only, optional)
	Ref           string   // Branch/tag/commit (git only, optional)
	Include       []string // Types to include: rules, context, skills, mcp
	MergeStrategy string   // Merge strategy: default, override, append
	InstallTo     string   // Installation path (optional)
}

// IncludeInfo represents information about an include source
type IncludeInfo struct {
	Name   string
	Source string
	Type   string // "git" or "local"
}

// ProfileInfo represents information about a profile
type ProfileInfo struct {
	Name      string
	Domains   []string
	IsDefault bool
}

// InstallSkillRequest represents a request to install a named skill
type InstallSkillRequest struct {
	Name   string // Skill name (required)
	Source string // Git URL or local path (required)
	Path   string // Path within repo to skill directory (optional, defaults to skills/<name>)
	Ref    string // Git ref (optional)
}

// InstalledSkillInfo represents information about an installed skill
type InstalledSkillInfo struct {
	Name   string
	Source string
	Path   string
	Ref    string
	Type   string // "git" or "local"
}
