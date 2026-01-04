package handlers

import (
	"context"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/crud"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// Domain Handlers

func CreateDomainHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "create_domain",
		"name":      result.Name,
		"path":      result.Path,
		"message":   "Domain created successfully",
	})
}

func DeleteDomainHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.RemoveDomain(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "delete_domain",
		"name":      name,
		"message":   "Domain deleted successfully",
	})
}

func ListDomainsHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	domains, err := op.ListDomains(ctx)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "list_domains",
		"domains":   domains,
		"count":     len(domains),
	})
}

// Rule Handlers

func CreateRuleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "create_rule",
		"path":      result.FullPath,
		"name":      result.Name,
		"domain":    result.Domain,
		"message":   "Rule created successfully",
	})
}

func UpdateRuleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")

	// First remove the existing rule
	err = op.RemoveFile(ctx, domain, "rules", name)
	if err != nil {
		return ToolError(err)
	}

	// Then create it with new content
	req := &crud.AddFileRequest{
		Domain:   domain,
		Type:     "rules",
		Name:     name,
		Content:  content,
		Priority: request.GetString("priority", "medium"),
		Targets:  request.GetStringSlice("targets", nil),
	}

	result, err := op.AddRule(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "update_rule",
		"path":      result.FullPath,
		"name":      result.Name,
		"domain":    result.Domain,
		"message":   "Rule updated successfully",
	})
}

func DeleteRuleHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "delete_rule",
		"name":      name,
		"domain":    domain,
		"message":   "Rule deleted successfully",
	})
}

func ListRulesHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	domain := request.GetString("domain", "")

	files, err := op.ListFiles(ctx, domain, "rules")
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "list_rules",
		"domain":    domain,
		"rules":     files,
		"count":     len(files),
	})
}

// Context Handlers

func CreateContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "create_context",
		"path":      result.FullPath,
		"name":      result.Name,
		"domain":    result.Domain,
		"message":   "Context created successfully",
	})
}

func UpdateContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")

	// First remove the existing context
	err = op.RemoveFile(ctx, domain, "context", name)
	if err != nil {
		return ToolError(err)
	}

	// Then create it with new content
	req := &crud.AddFileRequest{
		Domain:   domain,
		Type:     "context",
		Name:     name,
		Content:  content,
		Priority: request.GetString("priority", "medium"),
		Targets:  request.GetStringSlice("targets", nil),
	}

	result, err := op.AddContext(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "update_context",
		"path":      result.FullPath,
		"name":      result.Name,
		"domain":    result.Domain,
		"message":   "Context updated successfully",
	})
}

func DeleteContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "delete_context",
		"name":      name,
		"domain":    domain,
		"message":   "Context deleted successfully",
	})
}

func ListContextHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	domain := request.GetString("domain", "")

	files, err := op.ListFiles(ctx, domain, "context")
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "list_context",
		"domain":    domain,
		"context":   files,
		"count":     len(files),
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
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "list_contexts",
		"domain":    domain,
		"contexts":  items,
		"count":     len(items),
	})
}

// normalizePath converts file paths to use forward slashes
func normalizePath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// Skill Handlers

func CreateSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "create_skill",
		"path":      result.FullPath,
		"name":      result.Name,
		"domain":    result.Domain,
		"message":   "Skill created successfully",
	})
}

func UpdateSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")
	content := request.GetString("content", "")
	domain := request.GetString("domain", "")

	// First remove the existing skill
	err = op.RemoveFile(ctx, domain, "skills", name)
	if err != nil {
		return ToolError(err)
	}

	// Then create it with new content
	req := &crud.AddFileRequest{
		Domain:   domain,
		Type:     "skills",
		Name:     name,
		Content:  content,
		Priority: request.GetString("priority", "medium"),
		Targets:  request.GetStringSlice("targets", nil),
	}

	result, err := op.AddSkill(ctx, req)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "update_skill",
		"path":      result.FullPath,
		"name":      result.Name,
		"domain":    result.Domain,
		"message":   "Skill updated successfully",
	})
}

func DeleteSkillHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "delete_skill",
		"name":      name,
		"domain":    domain,
		"message":   "Skill deleted successfully",
	})
}

func ListSkillsHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	domain := request.GetString("domain", "")

	files, err := op.ListFiles(ctx, domain, "skills")
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "list_skills",
		"domain":    domain,
		"skills":    files,
		"count":     len(files),
	})
}

// Include Handlers

func AddIncludeHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "add_include",
		"name":      name,
		"source":    source,
		"message":   "Include added successfully",
	})
}

func RemoveIncludeHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.RemoveInclude(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "remove_include",
		"name":      name,
		"message":   "Include removed successfully",
	})
}

func ListIncludesHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	includes, err := op.ListIncludes(ctx)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "list_includes",
		"includes":  includes,
		"count":     len(includes),
	})
}

// Profile Handlers

func AddProfileHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
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
		"success":   true,
		"operation": "add_profile",
		"name":      name,
		"domains":   domains,
		"message":   "Profile added successfully",
	})
}

func RemoveProfileHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.RemoveProfile(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "remove_profile",
		"name":      name,
		"message":   "Profile removed successfully",
	})
}

func SetDefaultProfileHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	name := request.GetString("name", "")

	err = op.SetDefaultProfile(ctx, name)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "set_default_profile",
		"name":      name,
		"message":   "Default profile set successfully",
	})
}

func ListProfilesHandler(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return ToolError(err)
	}

	profiles, err := op.ListProfiles(ctx)
	if err != nil {
		return ToolError(err)
	}

	return ToolSuccess(map[string]interface{}{
		"success":   true,
		"operation": "list_profiles",
		"profiles":  profiles,
		"count":     len(profiles),
	})
}
