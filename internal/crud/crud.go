package crud

import (
	"context"
	"os"
	"path/filepath"

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
	aiRulezDir string
	filesMgr   *FileManager
}

// NewOperator creates a new Operator for the given base directory
func NewOperator(baseDir string) (*OperatorImpl, error) {
	aiRulezDir := filepath.Join(baseDir, ".ai-rulez")

	// Verify .ai-rulez directory exists
	if _, err := os.Stat(aiRulezDir); err != nil {
		return nil, oops.
			With("path", aiRulezDir).
			Hint("Run 'ai-rulez init' to create the .ai-rulez/ directory structure.").
			Wrapf(err, ".ai-rulez directory not found")
	}

	return &OperatorImpl{
		aiRulezDir: aiRulezDir,
		filesMgr:   NewFileManager(aiRulezDir),
	}, nil
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
	Local       bool     // Write to .ai-rulez/local/ (machine-local override, gitignored). Mutually exclusive with Domain.
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
	Name     string   // File name (without extension)
	Path     string   // Full path to file
	Type     string   // File type (rules, context, skills)
	Domain   string   // Domain name (empty if root)
	Priority string   // Priority level from metadata
	Targets  []string // Target providers from metadata
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
