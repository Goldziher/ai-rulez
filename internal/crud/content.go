package crud

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// AddRule creates a new rule file in the root or domain rules directory
// Returns FileResult with the created file information
func (op *OperatorImpl) AddRule(ctx context.Context, req *AddFileRequest) (*FileResult, error) {
	if req == nil {
		return nil, oops.
			Hint("AddFileRequest cannot be nil").
			Errorf("invalid request")
	}

	// Set defaults
	req.Type = ContentTypeRules

	// Validate inputs
	if err := ValidateFileName(req.Name); err != nil {
		return nil, err
	}

	if err := ValidatePriority(req.DefaultPriority()); err != nil {
		return nil, err
	}

	// Validate domain if specified
	if req.Domain != "" {
		if err := ValidateDomainName(req.Domain); err != nil {
			return nil, err
		}

		if err := op.requireDomain(req.Domain); err != nil {
			return nil, err
		}
	}

	// Check if file already exists
	if op.filesMgr.FileOrSkillExists(req.Domain, ContentTypeRules, req.Name) {
		filePath := op.filesMgr.GetFilePath(req.Domain, ContentTypeRules, req.Name)
		return nil, &FileExistsError{
			Path: filePath,
			Type: "rule",
		}
	}

	// Generate content if not provided
	content := req.Content
	if content == "" {
		content = GenerateRuleTemplate(req.Name, req.DefaultPriority(), req.Targets, "")
	} else if !strings.HasPrefix(content, "---") {
		// Add frontmatter if content provided without it
		content = GenerateFrontmatter(req.DefaultPriority(), req.Targets) + content
	}

	// Ensure trailing newline
	content = EnsureTrailingNewline(content)

	// Get file path and write
	filePath := op.filesMgr.GetFilePath(req.Domain, ContentTypeRules, req.Name)

	if err := op.filesMgr.WriteFile(filePath, content); err != nil {
		return nil, err
	}

	return &FileResult{
		Name:     req.Name,
		FullPath: filePath,
		Type:     ContentTypeRules,
		Domain:   req.Domain,
	}, nil
}

// AddContext creates a new context file in the root or domain context directory
// Returns FileResult with the created file information
func (op *OperatorImpl) AddContext(ctx context.Context, req *AddFileRequest) (*FileResult, error) {
	if req == nil {
		return nil, oops.
			Hint("AddFileRequest cannot be nil").
			Errorf("invalid request")
	}

	// Set defaults
	req.Type = ContentTypeContext

	// Validate inputs
	if err := ValidateFileName(req.Name); err != nil {
		return nil, err
	}

	if err := ValidatePriority(req.DefaultPriority()); err != nil {
		return nil, err
	}

	// Validate domain if specified
	if req.Domain != "" {
		if err := ValidateDomainName(req.Domain); err != nil {
			return nil, err
		}

		if err := op.requireDomain(req.Domain); err != nil {
			return nil, err
		}
	}

	// Check if file already exists
	if op.filesMgr.FileOrSkillExists(req.Domain, ContentTypeContext, req.Name) {
		filePath := op.filesMgr.GetFilePath(req.Domain, ContentTypeContext, req.Name)
		return nil, &FileExistsError{
			Path: filePath,
			Type: ContentTypeContext,
		}
	}

	// Generate content if not provided
	content := req.Content
	if content == "" {
		content = GenerateContextTemplate(req.Name, req.DefaultPriority(), req.Targets, "")
	} else if !strings.HasPrefix(content, "---") {
		// Add frontmatter if content provided without it
		content = GenerateFrontmatter(req.DefaultPriority(), req.Targets) + content
	}

	// Ensure trailing newline
	content = EnsureTrailingNewline(content)

	// Get file path and write
	filePath := op.filesMgr.GetFilePath(req.Domain, ContentTypeContext, req.Name)

	if err := op.filesMgr.WriteFile(filePath, content); err != nil {
		return nil, err
	}

	return &FileResult{
		Name:     req.Name,
		FullPath: filePath,
		Type:     ContentTypeContext,
		Domain:   req.Domain,
	}, nil
}

// AddSkill creates a new skill directory with a SKILL.md file
// Returns FileResult with the created skill information
func (op *OperatorImpl) AddSkill(ctx context.Context, req *AddFileRequest) (*FileResult, error) {
	if req == nil {
		return nil, oops.
			Hint("AddFileRequest cannot be nil").
			Errorf("invalid request")
	}

	// Set defaults
	req.Type = ContentTypeSkills

	// Validate inputs - skill names follow domain name rules
	if err := ValidateDomainName(req.Name); err != nil {
		return nil, oops.
			With("field", "skill_name").
			With("value", req.Name).
			Hint("Skill names follow the same rules as domain names: alphanumeric + underscore/hyphen, 1-50 chars.").
			Wrapf(err, "invalid skill name")
	}

	if err := ValidatePriority(req.DefaultPriority()); err != nil {
		return nil, err
	}

	// Validate domain if specified
	if req.Domain != "" {
		if err := ValidateDomainName(req.Domain); err != nil {
			return nil, err
		}

		if err := op.requireDomain(req.Domain); err != nil {
			return nil, err
		}
	}

	// Check if skill already exists
	if op.filesMgr.FileOrSkillExists(req.Domain, ContentTypeSkills, req.Name) {
		skillPath := op.filesMgr.GetFilePath(req.Domain, ContentTypeSkills, req.Name)
		return nil, &FileExistsError{
			Path: skillPath,
			Type: "skill",
		}
	}

	// Generate content if not provided
	content := req.Content
	description := config.SkillDescriptionOrFallback(req.Description, req.Name)
	if content == "" {
		content = GenerateSkillTemplate(req.Name, description, req.DefaultPriority(), req.Targets, "")
	} else if !strings.HasPrefix(content, "---") {
		// Add frontmatter if content provided without it
		content = GenerateSkillTemplate(req.Name, description, req.DefaultPriority(), req.Targets, content)
	}

	// Ensure trailing newline
	content = EnsureTrailingNewline(content)

	// Create skill directory
	skillDir := filepath.Join(op.filesMgr.GetSkillsPath(req.Domain), req.Name)
	if err := op.filesMgr.CreateDirectory(skillDir); err != nil {
		return nil, err
	}

	// Write SKILL.md file
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := op.filesMgr.WriteFile(skillFile, content); err != nil {
		return nil, err
	}

	return &FileResult{
		Name:     req.Name,
		FullPath: skillFile,
		Type:     ContentTypeSkills,
		Domain:   req.Domain,
	}, nil
}

// RemoveFile deletes a file or skill directory
func (op *OperatorImpl) RemoveFile(ctx context.Context, domain, ftype, name string) error {
	// Validate inputs
	if err := ValidateFileName(name); err != nil {
		return err
	}

	if err := ValidateFileType(ftype); err != nil {
		return err
	}

	// Validate domain if specified
	if domain != "" {
		if err := ValidateDomainName(domain); err != nil {
			return err
		}

		if !op.filesMgr.DomainExists(domain) {
			return &DomainNotFoundError{
				Name: domain,
				Path: op.filesMgr.GetDomainPath(domain),
			}
		}
	}

	// Check if file/skill exists
	if !op.filesMgr.FileOrSkillExists(domain, ftype, name) {
		filePath := op.filesMgr.GetFilePath(domain, ftype, name)
		return oops.
			With("path", filePath).
			With("type", ftype).
			With("name", name).
			Hint("The file/skill does not exist.").
			Errorf("file not found")
	}

	// Delete file or skill directory
	if ftype == ContentTypeSkills {
		// Delete skill directory
		skillDir := filepath.Join(op.filesMgr.GetSkillsPath(domain), name)
		if err := op.filesMgr.DeleteDirectory(skillDir); err != nil {
			return err
		}
	} else {
		// Delete file
		filePath := op.filesMgr.GetFilePath(domain, ftype, name)
		if err := op.filesMgr.DeleteFile(filePath); err != nil {
			return err
		}
	}

	return nil
}

// ListFiles returns information about all files of a specific type
func (op *OperatorImpl) ListFiles(ctx context.Context, domain, ftype string) ([]FileInfo, error) {
	// Validate inputs
	if err := ValidateFileType(ftype); err != nil {
		return nil, err
	}

	// Validate domain if specified
	if domain != "" {
		if err := ValidateDomainName(domain); err != nil {
			return nil, err
		}

		if !op.filesMgr.DomainExists(domain) {
			return nil, &DomainNotFoundError{
				Name: domain,
				Path: op.filesMgr.GetDomainPath(domain),
			}
		}
	}

	return op.listFilesInDirectory(domain, ftype)
}

// extractMetadata reads a file and extracts priority and targets from YAML frontmatter.
// Returns empty defaults if the file cannot be read or has no valid frontmatter.
func extractMetadata(filePath string) (priority string, targets []string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", nil
	}

	contentStr := string(content)
	if !strings.HasPrefix(contentStr, "---") {
		return "", nil
	}

	endIdx := strings.Index(contentStr[3:], "---")
	if endIdx == -1 {
		return "", nil
	}

	frontmatterStr := contentStr[3 : endIdx+3]

	var metadata struct {
		Priority string   `yaml:"priority"`
		Targets  []string `yaml:"targets"`
	}

	if err := yaml.Unmarshal([]byte(frontmatterStr), &metadata); err != nil {
		return "", nil
	}

	return metadata.Priority, metadata.Targets
}

// listFilesInDirectory is a helper that lists files in a specific directory
func (op *OperatorImpl) listFilesInDirectory(domainName, fileType string) ([]FileInfo, error) {
	var dirPath string

	switch fileType {
	case ContentTypeRules:
		dirPath = op.filesMgr.GetRulesPath(domainName)
	case ContentTypeContext:
		dirPath = op.filesMgr.GetContextPath(domainName)
	case ContentTypeSkills:
		dirPath = op.filesMgr.GetSkillsPath(domainName)
	case ContentTypeAgents:
		dirPath = op.filesMgr.GetAgentsPath(domainName)
	case ContentTypeCommands:
		dirPath = op.filesMgr.GetCommandsPath(domainName)
	case ContentTypeChecks:
		dirPath = op.filesMgr.GetChecksPath(domainName)
	default:
		return nil, oops.
			With("type", fileType).
			Hint("Valid types: rules, context, skills, agents, commands, checks.").
			Errorf("invalid file type: %s", fileType)
	}

	// If directory doesn't exist, return empty list
	if !op.filesMgr.PathExists(dirPath) {
		return []FileInfo{}, nil
	}

	var fileInfos []FileInfo

	if fileType == ContentTypeSkills {
		// For skills, list subdirectories (each skill is a directory with SKILL.md)
		subdirs, err := op.filesMgr.ListSubdirectories(dirPath)
		if err != nil {
			return nil, err
		}

		sort.Strings(subdirs)

		for _, skillName := range subdirs {
			skillPath := filepath.Join(dirPath, skillName, "SKILL.md")

			// Only include if SKILL.md exists
			if op.filesMgr.PathExists(skillPath) {
				priority, targets := extractMetadata(skillPath)
				fileInfos = append(fileInfos, FileInfo{
					Name:     skillName,
					Path:     skillPath,
					Type:     fileType,
					Domain:   domainName,
					Priority: priority,
					Targets:  targets,
				})
			}
		}
	} else {
		// For rules and context, list .md files
		files, err := op.filesMgr.ListMarkdownFiles(dirPath)
		if err != nil {
			return nil, err
		}

		sort.Strings(files)

		for _, fileName := range files {
			filePath := filepath.Join(dirPath, fileName)
			name := strings.TrimSuffix(fileName, ".md")

			priority, targets := extractMetadata(filePath)
			fileInfos = append(fileInfos, FileInfo{
				Name:     name,
				Path:     filePath,
				Type:     fileType,
				Domain:   domainName,
				Priority: priority,
				Targets:  targets,
			})
		}
	}

	return fileInfos, nil
}

// ReadFileContent reads and returns the content of a file by its path
func (op *OperatorImpl) ReadFileContent(path string) (string, error) {
	return op.filesMgr.ReadFile(path)
}

// UpdateFile atomically updates an existing file's content.
// Unlike the delete-then-create pattern, this uses WriteFile (temp+rename)
// so the old content is never lost if the write fails.
func (op *OperatorImpl) UpdateFile(_ context.Context, domain, ftype, name, content, priority string, targets []string) (*FileResult, error) {
	if err := ValidateFileName(name); err != nil {
		return nil, err
	}

	if priority == "" {
		priority = PriorityDefault
	}
	if err := ValidatePriority(priority); err != nil {
		return nil, err
	}

	if domain != "" {
		if err := ValidateDomainName(domain); err != nil {
			return nil, err
		}
		if !op.filesMgr.DomainExists(domain) {
			return nil, &DomainNotFoundError{
				Name: domain,
				Path: op.filesMgr.GetDomainPath(domain),
			}
		}
	}

	// Verify file exists
	if !op.filesMgr.FileOrSkillExists(domain, ftype, name) {
		return nil, ErrFileNotFound
	}

	// Build content with frontmatter
	if content != "" && !strings.HasPrefix(content, "---") {
		content = GenerateFrontmatter(priority, targets) + content
	}
	content = EnsureTrailingNewline(content)

	// Get path and write atomically (temp+rename)
	var filePath string
	if ftype == ContentTypeSkills {
		filePath = filepath.Join(op.filesMgr.GetSkillsPath(domain), name, "SKILL.md")
	} else {
		filePath = op.filesMgr.GetFilePath(domain, ftype, name)
	}

	if err := op.filesMgr.WriteFileOverwrite(filePath, content); err != nil {
		return nil, err
	}

	return &FileResult{
		Name:     name,
		FullPath: filePath,
		Type:     ftype,
		Domain:   domain,
	}, nil
}

// requireDomain checks that a domain exists. A local operator creates it on
// demand: machine-local domains are private scratch space with no other way to
// be declared.
func (op *OperatorImpl) requireDomain(name string) error {
	if op.filesMgr.DomainExists(name) {
		return nil
	}
	if op.local {
		return op.filesMgr.CreateDomainStructure(name)
	}
	return &DomainNotFoundError{Name: name, Path: op.filesMgr.GetDomainPath(name)}
}

// AddAgent creates a new agent file in the root or domain agents directory.
func (op *OperatorImpl) AddAgent(_ context.Context, req *AddFileRequest) (*FileResult, error) {
	return op.addFlatItem(req, ContentTypeAgents, func(r *AddFileRequest) string {
		return GenerateAgentTemplate(r.Name, r.Description)
	})
}

// AddCommand creates a new command file in the root or domain commands directory.
func (op *OperatorImpl) AddCommand(_ context.Context, req *AddFileRequest) (*FileResult, error) {
	return op.addFlatItem(req, ContentTypeCommands, func(r *AddFileRequest) string {
		return GenerateCommandTemplate(r.Name, r.Description)
	})
}

// AddCheck creates a new check file in the root or domain checks directory.
func (op *OperatorImpl) AddCheck(_ context.Context, req *AddFileRequest) (*FileResult, error) {
	if req != nil {
		if err := ValidateCheckName(req.Name); err != nil {
			return nil, err
		}
	}
	return op.addFlatItem(req, ContentTypeChecks, func(r *AddFileRequest) string {
		return GenerateCheckTemplate(r.Name, r.Description)
	})
}

// addFlatItem writes a flat markdown content file (agent or command). Content
// supplied without frontmatter is written as given, since agent and command
// frontmatter carries no priority or targets of its own to generate.
func (op *OperatorImpl) addFlatItem(req *AddFileRequest, ftype string, template func(*AddFileRequest) string) (*FileResult, error) {
	if req == nil {
		return nil, oops.Hint("AddFileRequest cannot be nil").Errorf("invalid request")
	}
	req.Type = ftype
	if err := ValidateFileName(req.Name); err != nil {
		return nil, err
	}
	if req.Domain != "" {
		if err := ValidateDomainName(req.Domain); err != nil {
			return nil, err
		}
		if err := op.requireDomain(req.Domain); err != nil {
			return nil, err
		}
	}

	filePath := op.filesMgr.GetFilePath(req.Domain, ftype, req.Name)
	if op.filesMgr.PathExists(filePath) {
		return nil, &FileExistsError{Path: filePath, Type: ftype}
	}
	content := req.Content
	if content == "" {
		content = template(req)
	}
	if err := op.filesMgr.WriteFile(filePath, EnsureTrailingNewline(content)); err != nil {
		return nil, err
	}
	return &FileResult{Name: req.Name, FullPath: filePath, Type: ftype, Domain: req.Domain}, nil
}
