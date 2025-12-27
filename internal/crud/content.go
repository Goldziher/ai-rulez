package crud

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
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

		if !op.filesMgr.DomainExists(req.Domain) {
			return nil, &DomainNotFoundError{
				Name: req.Domain,
				Path: op.filesMgr.GetDomainPath(req.Domain),
			}
		}
	}

	// Check if file already exists
	if op.filesMgr.FileOrSkillExists(req.Domain, "rules", req.Name) {
		filePath := op.filesMgr.GetFilePath(req.Domain, "rules", req.Name)
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
	filePath := op.filesMgr.GetFilePath(req.Domain, "rules", req.Name)

	if err := op.filesMgr.WriteFile(filePath, content); err != nil {
		return nil, err
	}

	return &FileResult{
		Name:     req.Name,
		FullPath: filePath,
		Type:     "rules",
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

		if !op.filesMgr.DomainExists(req.Domain) {
			return nil, &DomainNotFoundError{
				Name: req.Domain,
				Path: op.filesMgr.GetDomainPath(req.Domain),
			}
		}
	}

	// Check if file already exists
	if op.filesMgr.FileOrSkillExists(req.Domain, "context", req.Name) {
		filePath := op.filesMgr.GetFilePath(req.Domain, "context", req.Name)
		return nil, &FileExistsError{
			Path: filePath,
			Type: "context",
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
	filePath := op.filesMgr.GetFilePath(req.Domain, "context", req.Name)

	if err := op.filesMgr.WriteFile(filePath, content); err != nil {
		return nil, err
	}

	return &FileResult{
		Name:     req.Name,
		FullPath: filePath,
		Type:     "context",
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

		if !op.filesMgr.DomainExists(req.Domain) {
			return nil, &DomainNotFoundError{
				Name: req.Domain,
				Path: op.filesMgr.GetDomainPath(req.Domain),
			}
		}
	}

	// Check if skill already exists
	if op.filesMgr.FileOrSkillExists(req.Domain, "skills", req.Name) {
		skillPath := op.filesMgr.GetFilePath(req.Domain, "skills", req.Name)
		return nil, &FileExistsError{
			Path: skillPath,
			Type: "skill",
		}
	}

	// Generate content if not provided
	content := req.Content
	if content == "" {
		content = GenerateSkillTemplate(req.Name, "", req.DefaultPriority(), req.Targets, "")
	} else if !strings.HasPrefix(content, "---") {
		// Add frontmatter if content provided without it
		content = GenerateFrontmatter(req.DefaultPriority(), req.Targets) + content
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
		Type:     "skills",
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
	default:
		return nil, oops.
			With("type", fileType).
			Hint("Valid types: rules, context, skills.").
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
				fileInfos = append(fileInfos, FileInfo{
					Name:   skillName,
					Path:   skillPath,
					Type:   fileType,
					Domain: domainName,
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

			fileInfos = append(fileInfos, FileInfo{
				Name:   name,
				Path:   filePath,
				Type:   fileType,
				Domain: domainName,
			})
		}
	}

	return fileInfos, nil
}
