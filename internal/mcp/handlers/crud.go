package handlers

import (
	"context"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/crud"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// workingDir extracts the working_directory parameter from a request, defaulting to "."
func workingDir(request *ToolRequest) string {
	return request.GetString("working_directory", ".")
}

// readFileContent reads and returns the content of a rule/context/skill file
func readFileContent(ctx context.Context, op *crud.OperatorImpl, domain, fileType, name string) (content string, filePath string, err error) {
	files, err := op.ListFiles(ctx, domain, fileType)
	if err != nil {
		return "", "", err
	}

	for _, f := range files {
		if f.Name == name {
			content, err := op.ReadFileContent(f.Path)
			if err != nil {
				return "", "", err
			}
			return content, normalizePath(f.Path), nil
		}
	}

	return "", "", crud.ErrFileNotFound
}

// Domain Handlers

func CreateDomainHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	description := request.GetString("description", "")

	req := &crud.AddDomainRequest{
		Name:        name,
		Description: description,
	}

	result, err := op.AddDomain(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "create_domain",
		keyName:      result.Name,
		keyPath:      result.Path,
		keyMessage:   "Domain created successfully",
	})
}

func DeleteDomainHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.RemoveDomain(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "delete_domain",
		keyName:      name,
		keyMessage:   "Domain deleted successfully",
	})
}

func ListDomainsHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	domains, err := op.ListDomains(ctx)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "list_domains",
		"domains":    domains,
		keyCount:     len(domains),
	})
}

// Read Handlers

func ReadRuleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	domain := request.GetString("domain", "")

	content, path, err := readFileContent(ctx, op, domain, "rules", name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "read_rule",
		keyName:      name,
		keyDomain:    domain,
		keyPath:      path,
		keyContent:   content,
	})
}

func ReadContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	domain := request.GetString("domain", "")

	content, path, err := readFileContent(ctx, op, domain, "context", name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "read_context",
		keyName:      name,
		keyDomain:    domain,
		keyPath:      path,
		keyContent:   content,
	})
}

func ReadSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	domain := request.GetString("domain", "")

	content, path, err := readFileContent(ctx, op, domain, "skills", name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "read_skill",
		keyName:      name,
		keyDomain:    domain,
		keyPath:      path,
		keyContent:   content,
	})
}

// Rule Handlers

func CreateRuleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")
	priority := request.GetString("priority", "medium")
	targets := request.GetStringSlice("targets", nil)

	req := &crud.AddFileRequest{
		Domain:   domain,
		Type:     "rules",
		Name:     name,
		Content:  content,
		Priority: priority,
		Targets:  targets,
	}

	result, err := op.AddRule(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "create_rule",
		keyPath:      result.FullPath,
		keyName:      result.Name,
		keyDomain:    result.Domain,
		keyMessage:   "Rule created successfully",
	})
}

func UpdateRuleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")
	priority := request.GetString("priority", "medium")
	targets := request.GetStringSlice("targets", nil)

	result, err := op.UpdateFile(ctx, domain, "rules", name, content, priority, targets)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "update_rule",
		keyPath:      result.FullPath,
		keyName:      result.Name,
		keyDomain:    result.Domain,
		keyMessage:   "Rule updated successfully",
	})
}

func DeleteRuleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	domain := request.GetString("domain", "")

	err = op.RemoveFile(ctx, domain, "rules", name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "delete_rule",
		keyName:      name,
		keyDomain:    domain,
		keyMessage:   "Rule deleted successfully",
	})
}

func ListRulesHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	domain := request.GetString("domain", "")

	files, err := op.ListFiles(ctx, domain, "rules")
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "list_rules",
		keyDomain:    domain,
		"rules":      files,
		keyCount:     len(files),
	})
}

// Context Handlers

func CreateContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")
	priority := request.GetString("priority", "medium")
	targets := request.GetStringSlice("targets", nil)

	req := &crud.AddFileRequest{
		Domain:   domain,
		Type:     "context",
		Name:     name,
		Content:  content,
		Priority: priority,
		Targets:  targets,
	}

	result, err := op.AddContext(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "create_context",
		keyPath:      result.FullPath,
		keyName:      result.Name,
		keyDomain:    result.Domain,
		keyMessage:   "Context created successfully",
	})
}

func UpdateContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")
	priority := request.GetString("priority", "medium")
	targets := request.GetStringSlice("targets", nil)

	result, err := op.UpdateFile(ctx, domain, "context", name, content, priority, targets)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "update_context",
		keyPath:      result.FullPath,
		keyName:      result.Name,
		keyDomain:    result.Domain,
		keyMessage:   "Context updated successfully",
	})
}

func DeleteContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	domain := request.GetString("domain", "")

	err = op.RemoveFile(ctx, domain, "context", name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "delete_context",
		keyName:      name,
		keyDomain:    domain,
		keyMessage:   "Context deleted successfully",
	})
}

// contextItem represents a context file with its summary
type contextItem struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Path    string `json:"path"`
}

// extractSummary extracts the summary field from YAML frontmatter
func extractSummary(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	// Parse frontmatter
	contentStr := string(content)
	if !strings.HasPrefix(contentStr, "---") {
		return ""
	}

	// Find the closing --- delimiter
	endIdx := strings.Index(contentStr[3:], "---")
	if endIdx == -1 {
		return ""
	}

	frontmatterStr := contentStr[3 : endIdx+3]

	// Parse YAML
	var metadata map[string]interface{}
	err = yaml.Unmarshal([]byte(frontmatterStr), &metadata)
	if err != nil {
		return ""
	}

	// Extract summary
	if summary, ok := metadata["summary"]; ok {
		if summaryStr, ok := summary.(string); ok {
			return summaryStr
		}
	}

	return ""
}

// ListContextsHandler lists all context files with their names and summaries
func ListContextsHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	domain := request.GetString("domain", "")

	files, err := op.ListFiles(ctx, domain, "context")
	if err != nil {
		return ToolError(err)
	}

	// Build context items with summaries
	var items []contextItem
	for _, file := range files {
		// Normalize path to use forward slashes
		normalizedPath := normalizePath(file.Path)

		summary := extractSummary(file.Path)
		items = append(items, contextItem{
			Name:    file.Name,
			Summary: summary,
			Path:    normalizedPath,
		})
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "list_contexts",
		keyDomain:    domain,
		"contexts":   items,
		keyCount:     len(items),
	})
}

// normalizePath converts file paths to use forward slashes
func normalizePath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// Skill Handlers

func CreateSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")
	priority := request.GetString("priority", "medium")
	targets := request.GetStringSlice("targets", nil)

	req := &crud.AddFileRequest{
		Domain:   domain,
		Type:     "skills",
		Name:     name,
		Content:  content,
		Priority: priority,
		Targets:  targets,
	}

	result, err := op.AddSkill(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "create_skill",
		keyPath:      result.FullPath,
		keyName:      result.Name,
		keyDomain:    result.Domain,
		keyMessage:   "Skill created successfully",
	})
}

func UpdateSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")
	priority := request.GetString("priority", "medium")
	targets := request.GetStringSlice("targets", nil)

	result, err := op.UpdateFile(ctx, domain, "skills", name, content, priority, targets)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "update_skill",
		keyPath:      result.FullPath,
		keyName:      result.Name,
		keyDomain:    result.Domain,
		keyMessage:   "Skill updated successfully",
	})
}

func DeleteSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	domain := request.GetString("domain", "")

	err = op.RemoveFile(ctx, domain, "skills", name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "delete_skill",
		keyName:      name,
		keyDomain:    domain,
		keyMessage:   "Skill deleted successfully",
	})
}

func ListSkillsHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	domain := request.GetString("domain", "")

	files, err := op.ListFiles(ctx, domain, "skills")
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "list_skills",
		keyDomain:    domain,
		"skills":     files,
		keyCount:     len(files),
	})
}

// Include Handlers

func AddIncludeHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	source := request.GetString("source", "")
	path := request.GetString("path", "")
	ref := request.GetString("ref", "")
	include := request.GetStringSlice("include", nil)
	mergeStrategy := request.GetString("merge_strategy", "default")
	installTo := request.GetString("install_to", "")

	req := &crud.AddIncludeRequest{
		Name:          name,
		Source:        source,
		Path:          path,
		Ref:           ref,
		Include:       include,
		MergeStrategy: mergeStrategy,
		InstallTo:     installTo,
	}

	err = op.AddInclude(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "add_include",
		keyName:      name,
		keySource:    source,
		keyMessage:   "Include added successfully",
	})
}

func RemoveIncludeHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.RemoveInclude(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "remove_include",
		keyName:      name,
		keyMessage:   "Include removed successfully",
	})
}

func ListIncludesHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	includes, err := op.ListIncludes(ctx)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "list_includes",
		"includes":   includes,
		keyCount:     len(includes),
	})
}

// Installed Skill Handlers

func InstallSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	source := request.GetString("source", "")
	path := request.GetString("path", "")
	ref := request.GetString("ref", "")

	req := &crud.InstallSkillRequest{
		Name:   name,
		Source: source,
		Path:   path,
		Ref:    ref,
	}

	err = op.InstallSkill(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "install_skill",
		keyName:      name,
		keySource:    source,
		keyMessage:   "Skill installed successfully",
	})
}

func UninstallSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.UninstallSkill(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "uninstall_skill",
		keyName:      name,
		keyMessage:   "Skill uninstalled successfully",
	})
}

func ListInstalledSkillsHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	skills, err := op.ListInstalledSkills(ctx)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:         true,
		keyOperation:       "list_installed_skills",
		"installed_skills": skills,
		keyCount:           len(skills),
	})
}

// Profile Handlers

func AddProfileHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	domains := request.GetStringSlice("domains", nil)

	err = op.AddProfile(ctx, name, domains)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "add_profile",
		keyName:      name,
		"domains":    domains,
		keyMessage:   "Profile added successfully",
	})
}

func RemoveProfileHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.RemoveProfile(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "remove_profile",
		keyName:      name,
		keyMessage:   "Profile removed successfully",
	})
}

func SetDefaultProfileHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.SetDefaultProfile(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "set_default_profile",
		keyName:      name,
		keyMessage:   "Default profile set successfully",
	})
}

func ListProfilesHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(workingDir(request))
	if err != nil {
		return ToolError(err)
	}

	profiles, err := op.ListProfiles(ctx)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		keySuccess:   true,
		keyOperation: "list_profiles",
		"profiles":   profiles,
		keyCount:     len(profiles),
	})
}
