package crud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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
	if err := ValidateNewFileName(req.Name); err != nil {
		return nil, err
	}

	if err := ValidatePriority(req.DefaultPriority()); err != nil {
		return nil, err
	}
	if err := ValidateTargets(req.Targets); err != nil {
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

	if err := op.writeConcept(ctx, filePath, ContentTypeRules, req.Domain, req.Name, content); err != nil {
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
	if err := ValidateNewFileName(req.Name); err != nil {
		return nil, err
	}

	if err := ValidatePriority(req.DefaultPriority()); err != nil {
		return nil, err
	}
	if err := ValidateTargets(req.Targets); err != nil {
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

	if err := op.writeConcept(ctx, filePath, ContentTypeContext, req.Domain, req.Name, content); err != nil {
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
	if err := ValidateTargets(req.Targets); err != nil {
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
	description := strings.TrimSpace(req.Description)
	if description == "" {
		// A scaffold must pass `validate` as written: AR802 wants at least
		// 20 characters, which the bare name never reaches.
		description = fmt.Sprintf("Describe what the %s skill does and when an assistant should use it", req.Name)
	}
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
	if err := op.writeConcept(ctx, skillFile, ContentTypeSkills, req.Domain, req.Name, content); err != nil {
		return nil, err
	}

	return &FileResult{
		Name:     req.Name,
		FullPath: skillFile,
		Type:     ContentTypeSkills,
		Domain:   req.Domain,
	}, nil
}

// ContentPath is what RemoveFile deletes for the item: the file, or the whole
// directory for a skill. It does not check that the item exists.
func (op *OperatorImpl) ContentPath(domain, ftype, name string) string {
	if ftype == ContentTypeSkills {
		return filepath.Join(op.filesMgr.GetSkillsPath(domain), name)
	}
	return op.filesMgr.GetFilePath(domain, ftype, name)
}

// RemoveFile deletes a file or skill directory
func (op *OperatorImpl) RemoveFile(ctx context.Context, domain, ftype, name string) error {
	if err := op.RequireContent(ctx, domain, ftype, name); err != nil {
		return err
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

	return op.refreshIndexes(ctx)
}

// RequireContent reports why the item cannot be read, changed or removed (a bad name, an unknown
// domain, nothing of that name) without touching anything. A caller asks it
// before it asks the user to confirm a removal or reads the item.
func (op *OperatorImpl) RequireContent(ctx context.Context, domain, ftype, name string) error {
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

	if err := op.filesMgr.Confined(op.filesMgr.GetFilePath(domain, ftype, name)); err != nil {
		return err
	}

	// Check if file/skill exists
	if !op.filesMgr.FileOrSkillExists(domain, ftype, name) {
		filePath := op.filesMgr.GetFilePath(domain, ftype, name)
		hint := "Check the name; nothing of that name exists."
		if files, err := op.ListFiles(ctx, domain, ftype); err == nil {
			names := make([]string, 0, len(files))
			for i := range files {
				names = append(names, files[i].Name)
			}
			if len(names) > 0 {
				hint = fmt.Sprintf("Existing %s: %s", ftype, strings.Join(names, ", "))
			} else {
				hint = fmt.Sprintf("There are no %s yet.", ftype)
			}
		}
		return oops.
			With("path", filePath).
			With("type", ftype).
			With("name", name).
			Hint(hint).
			Errorf("%s %q not found: no such file or skill at %s", ftype, name, filePath)
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
	dirPath, err := op.contentDirPath(domainName, fileType)
	if err != nil {
		return nil, err
	}

	// If directory doesn't exist, return empty list
	if !op.filesMgr.PathExists(dirPath) {
		return []FileInfo{}, nil
	}
	if err := op.filesMgr.Confined(dirPath); err != nil {
		return nil, err
	}

	if fileType == ContentTypeSkills {
		// For skills, list subdirectories (each skill is a directory with SKILL.md)
		return op.listSkillDirectories(domainName, fileType, dirPath)
	}
	// For rules and context, list .md files
	return op.listMarkdownContent(domainName, fileType, dirPath)
}

// contentDirPath maps a content type to the directory that holds it.
func (op *OperatorImpl) contentDirPath(domainName, fileType string) (string, error) {
	switch fileType {
	case ContentTypeRules:
		return op.filesMgr.GetRulesPath(domainName), nil
	case ContentTypeContext:
		return op.filesMgr.GetContextPath(domainName), nil
	case ContentTypeSkills:
		return op.filesMgr.GetSkillsPath(domainName), nil
	case ContentTypeAgents:
		return op.filesMgr.GetAgentsPath(domainName), nil
	case ContentTypeCommands:
		return op.filesMgr.GetCommandsPath(domainName), nil
	case ContentTypeChecks:
		return op.filesMgr.GetChecksPath(domainName), nil
	default:
		return "", oops.
			With("type", fileType).
			Hint("Valid types: rules, context, skills, agents, commands, checks.").
			Errorf("invalid file type: %s", fileType)
	}
}

// listSkillDirectories lists the skill subdirectories of dirPath, one FileInfo
// per directory that holds a SKILL.md.
func (op *OperatorImpl) listSkillDirectories(domainName, fileType, dirPath string) ([]FileInfo, error) {
	subdirs, err := op.filesMgr.ListSubdirectories(dirPath)
	if err != nil {
		return nil, err
	}

	sort.Strings(subdirs)

	var fileInfos []FileInfo
	for _, skillName := range subdirs {
		skillPath := filepath.Join(dirPath, skillName, "SKILL.md")

		// Only include if SKILL.md exists and is not a link
		if op.filesMgr.PathExists(skillPath) && op.filesMgr.Confined(skillPath) == nil {
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
	return fileInfos, nil
}

// listMarkdownContent lists the .md files of dirPath as content files.
func (op *OperatorImpl) listMarkdownContent(domainName, fileType, dirPath string) ([]FileInfo, error) {
	files, err := op.filesMgr.ListMarkdownFiles(dirPath)
	if err != nil {
		return nil, err
	}

	sort.Strings(files)

	var fileInfos []FileInfo
	for _, fileName := range files {
		filePath := filepath.Join(dirPath, fileName)
		if op.filesMgr.isGeneratedListing(filePath) {
			continue // the generated index.md or log.md is not content
		}
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
	return fileInfos, nil
}

// ReadFileContent reads and returns the content of a file by its path
func (op *OperatorImpl) ReadFileContent(path string) (string, error) {
	return op.filesMgr.ReadFile(path)
}

// UpdateFile atomically updates an existing file's content.
// Unlike the delete-then-create pattern, this uses WriteFile (temp+rename)
// so the old content is never lost if the write fails.
func (op *OperatorImpl) UpdateFile(ctx context.Context, domain, ftype, name, content, priority string, targets []string) (*FileResult, error) {
	if err := ValidateFileName(name); err != nil {
		return nil, err
	}

	if priority != "" {
		if err := ValidatePriority(priority); err != nil {
			return nil, err
		}
	}
	if err := ValidateTargets(targets); err != nil {
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

	// Get path and write atomically (temp+rename)
	var filePath string
	if ftype == ContentTypeSkills {
		filePath = filepath.Join(op.filesMgr.GetSkillsPath(domain), name, "SKILL.md")
	} else {
		filePath = op.filesMgr.GetFilePath(domain, ftype, name)
	}

	previous, err := op.filesMgr.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Bare content keeps the frontmatter the file had (priority, targets,
	// description, anything else); only the flags that were given change it.
	if content != "" && !strings.HasPrefix(content, "---") {
		if content, err = withKeptFrontmatter(previous, content, priority, targets); err != nil {
			return nil, err
		}
	}
	content = EnsureTrailingNewline(content)

	if err := op.overwriteConcept(ctx, filePath, ftype, domain, name, content, previous); err != nil {
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
	if err := op.filesMgr.Confined(op.filesMgr.GetDomainPath(name)); err != nil {
		return err
	}
	if op.filesMgr.DomainExists(name) {
		return nil
	}
	if op.local {
		return op.filesMgr.CreateDomainStructure(name)
	}
	return &DomainNotFoundError{Name: name, Path: op.filesMgr.GetDomainPath(name)}
}

// AddAgent creates a new agent file in the root or domain agents directory.
func (op *OperatorImpl) AddAgent(ctx context.Context, req *AddFileRequest) (*FileResult, error) {
	return op.addFlatItem(ctx, req, ContentTypeAgents, func(r *AddFileRequest) string {
		return GenerateAgentTemplate(r.Name, r.Description)
	})
}

// AddCommand creates a new command file in the root or domain commands directory.
func (op *OperatorImpl) AddCommand(ctx context.Context, req *AddFileRequest) (*FileResult, error) {
	return op.addFlatItem(ctx, req, ContentTypeCommands, func(r *AddFileRequest) string {
		return GenerateCommandTemplate(r.Name, r.Description)
	})
}

// AddCheck creates a new check file in the root or domain checks directory.
func (op *OperatorImpl) AddCheck(ctx context.Context, req *AddFileRequest) (*FileResult, error) {
	if req != nil {
		if err := ValidateCheckName(req.Name); err != nil {
			return nil, err
		}
	}
	return op.addFlatItem(ctx, req, ContentTypeChecks, func(r *AddFileRequest) string {
		return GenerateCheckTemplate(r.Name, r.Description)
	})
}

// addFlatItem writes a flat markdown content file (agent or command). Content
// supplied without frontmatter is written as given, since agent and command
// frontmatter carries no priority or targets of its own to generate.
func (op *OperatorImpl) addFlatItem(ctx context.Context, req *AddFileRequest, ftype string, template func(*AddFileRequest) string) (*FileResult, error) {
	if req == nil {
		return nil, oops.Hint("AddFileRequest cannot be nil").Errorf("invalid request")
	}
	req.Type = ftype
	if err := ValidateNewFileName(req.Name); err != nil {
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
	if err := op.writeConcept(ctx, filePath, ftype, req.Domain, req.Name, EnsureTrailingNewline(content)); err != nil {
		return nil, err
	}
	return &FileResult{Name: req.Name, FullPath: filePath, Type: ftype, Domain: req.Domain}, nil
}

// writeConcept stores a new content file as an OKF concept and refreshes the indexes.
func (op *OperatorImpl) writeConcept(ctx context.Context, path, ftype, domain, name, content string) error {
	concept, err := op.concept(ftype, domain, name, content, "")
	if err != nil {
		return err
	}
	if err := op.filesMgr.WriteFile(path, concept); err != nil {
		return err
	}
	return op.refreshIndexes(ctx)
}

// overwriteConcept replaces an existing content file with an OKF concept, keeping
// the type and title of previous, and refreshes the indexes.
func (op *OperatorImpl) overwriteConcept(ctx context.Context, path, ftype, domain, name, content, previous string) error {
	concept, err := op.concept(ftype, domain, name, content, previous)
	if err != nil {
		return err
	}
	if err := op.filesMgr.WriteFileOverwrite(path, concept); err != nil {
		return err
	}
	return op.refreshIndexes(ctx)
}

// withKeptFrontmatter puts body under the frontmatter of previous, with priority
// and targets set when given. A previous without frontmatter gets the generated
// default.
func withKeptFrontmatter(previous, body, priority string, targets []string) (string, error) {
	fmText, _, has := splitFrontmatter(config.NativeContent(previous))
	if !has {
		if priority == "" {
			priority = PriorityDefault
		}
		return GenerateFrontmatter(priority, targets) + body, nil
	}
	mapping, err := frontmatterMapping(fmText)
	if err != nil {
		return "", err
	}
	if priority != "" {
		if err := setFrontmatterKey(mapping, "priority", priority); err != nil {
			return "", err
		}
	}
	if len(targets) > 0 {
		if err := setFrontmatterKey(mapping, "targets", NormalizeTargets(targets)); err != nil {
			return "", err
		}
	}
	head, err := renderFrontmatter(mapping)
	if err != nil {
		return "", err
	}
	return head + body, nil
}
