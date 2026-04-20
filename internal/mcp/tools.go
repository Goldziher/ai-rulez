package mcp

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/mcp/handlers"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type handlerFunc func(context.Context, *handlers.ToolRequest) (*sdkmcp.CallToolResult, error)

type toolSchemaBuilder struct {
	properties map[string]any
	required   []string
}

func newSchemaBuilder() *toolSchemaBuilder {
	return &toolSchemaBuilder{
		properties: map[string]any{},
	}
}

func (b *toolSchemaBuilder) String(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type": "string",
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) StringArray(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Boolean(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type": "boolean",
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Number(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type": "number",
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Object(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "string"},
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Enum(name, description string, values []string, required bool) *toolSchemaBuilder {
	enumValues := make([]any, len(values))
	for i, v := range values {
		enumValues[i] = v
	}
	prop := map[string]any{
		"type": "string",
		"enum": enumValues,
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

// WorkingDirectory adds the standard working_directory parameter to all CRUD tools
func (b *toolSchemaBuilder) WorkingDirectory() *toolSchemaBuilder {
	return b.String("working_directory", "Working directory path (defaults to current directory, useful for polyrepo setups)", false)
}

func (b *toolSchemaBuilder) Build() map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": b.properties,
	}
	if len(b.required) > 0 {
		schema["required"] = b.required
	}
	return schema
}

// boolPtr returns a pointer to a bool value, used for optional annotation hints.
func boolPtr(b bool) *bool {
	return &b
}

// readOnlyAnnotations returns tool annotations marking a tool as read-only and non-destructive.
func readOnlyAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: boolPtr(false),
	}
}

// destructiveAnnotations returns tool annotations marking a tool as destructive.
func destructiveAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: boolPtr(true),
	}
}

func newTool(name, description string, builder *toolSchemaBuilder) *sdkmcp.Tool {
	var schema map[string]any
	if builder != nil {
		schema = builder.Build()
	} else {
		schema = newSchemaBuilder().Build()
	}
	return &sdkmcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: schema,
	}
}

func newAnnotatedTool(name, description string, builder *toolSchemaBuilder, annotations *sdkmcp.ToolAnnotations) *sdkmcp.Tool {
	tool := newTool(name, description, builder)
	tool.Annotations = annotations
	return tool
}

func (s *Server) addTool(tool *sdkmcp.Tool, handler handlerFunc) {
	if tool.InputSchema == nil {
		tool.InputSchema = newSchemaBuilder().Build()
	}

	sdkmcp.AddTool(s.mcpServer, tool, func(ctx context.Context, req *sdkmcp.CallToolRequest, input map[string]any) (*sdkmcp.CallToolResult, any, error) {
		wrapper := handlers.NewToolRequest(req, input)
		res, err := handler(ctx, wrapper)
		return res, nil, err
	})
}

func (s *Server) registerTools() {
	s.registerProjectTools()
	s.registerUtilityTools()
	s.registerCRUDTools()
}

func (s *Server) registerProjectTools() {
	s.addTool(
		newTool("generate_outputs", "Generate output files from the current configuration, respecting includes and extends",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file (optional)", false).
				Boolean("dry_run", "Preview changes without writing files", false).
				Boolean("recursive", "Generate for all subdirectories containing .ai-rulez/", false).
				WorkingDirectory(),
		),
		handlers.GenerateOutputsHandler,
	)

	s.addTool(
		newAnnotatedTool("validate_config", "Validate the configuration file, including all includes",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file to validate (optional)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ValidateConfigHandler,
	)

	s.addTool(
		newTool("init_project", "Initialize a new ai-rulez project in the current directory",
			newSchemaBuilder().
				String("project_name", "The name for the new project", false).
				StringArray("providers", "A list of providers to enable (e.g., ['claude', 'cursor'])", false).
				Boolean("with_agents", "Include sample agent configurations", false).
				Boolean("all_providers", "Enable all supported providers", false).
				Boolean("popular_providers", "Enable a curated list of popular providers", false).
				WorkingDirectory(),
		),
		handlers.InitProjectHandler,
	)
}

func (s *Server) registerUtilityTools() {
	s.addTool(
		newAnnotatedTool("get_version", "Get the ai-rulez version", nil, readOnlyAnnotations()),
		handlers.GetVersionHandler(s.version),
	)
}

var priorityValues = []string{"critical", "high", "medium", "low"}

func (s *Server) registerCRUDTools() {
	// Domain tools
	s.addTool(
		newTool("create_domain", "Create a new domain with subdirectories for rules, context, and skills",
			newSchemaBuilder().
				String("name", "Domain name (alphanumeric and underscores, 1-50 characters)", true).
				String("description", "Optional domain description", false).
				WorkingDirectory(),
		),
		handlers.CreateDomainHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_domain", "Delete a domain and all its contents",
			newSchemaBuilder().
				String("name", "Domain name to delete", true).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteDomainHandler,
	)

	s.addTool(
		newAnnotatedTool("list_domains", "List all domains in the .ai-rulez directory",
			newSchemaBuilder().WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListDomainsHandler,
	)

	// Rule tools
	s.addTool(
		newTool("create_rule", "Create a new rule file with optional YAML frontmatter",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("content", "Markdown content with optional YAML frontmatter", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				WorkingDirectory(),
		),
		handlers.CreateRuleHandler,
	)

	s.addTool(
		newAnnotatedTool("read_rule", "Read the content of a rule file",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadRuleHandler,
	)

	s.addTool(
		newTool("update_rule", "Update an existing rule file atomically",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				WorkingDirectory(),
		),
		handlers.UpdateRuleHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_rule", "Delete a rule file",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteRuleHandler,
	)

	s.addTool(
		newAnnotatedTool("list_rules", "List all rules in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root rules if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListRulesHandler,
	)

	// Context tools
	s.addTool(
		newTool("create_context", "Create a new context file with optional YAML frontmatter",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("content", "Markdown content with optional YAML frontmatter", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				WorkingDirectory(),
		),
		handlers.CreateContextHandler,
	)

	s.addTool(
		newAnnotatedTool("read_context", "Read the content of a context file",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadContextHandler,
	)

	s.addTool(
		newTool("update_context", "Update an existing context file atomically",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				WorkingDirectory(),
		),
		handlers.UpdateContextHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_context", "Delete a context file",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteContextHandler,
	)

	s.addTool(
		newAnnotatedTool("list_context", "List all context files in the root or a specific domain with summaries",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root context if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListContextsHandler,
	)

	// Skill tools
	s.addTool(
		newTool("create_skill", "Create a new skill file with optional YAML frontmatter",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("content", "Markdown content with optional YAML frontmatter", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				WorkingDirectory(),
		),
		handlers.CreateSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("read_skill", "Read the content of a skill file",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadSkillHandler,
	)

	s.addTool(
		newTool("update_skill", "Update an existing skill file atomically",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				WorkingDirectory(),
		),
		handlers.UpdateSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_skill", "Delete a skill file",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("list_skills", "List all skill files in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root skills if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListSkillsHandler,
	)

	// Include tools
	s.addTool(
		newTool("add_include", "Add a new include source (git URL or local path) to the configuration",
			newSchemaBuilder().
				String("name", "Include name (unique identifier)", true).
				String("source", "Git URL or local filesystem path", true).
				String("path", "Path within git repository (git sources only)", false).
				String("ref", "Git reference: branch, tag, or commit hash (git sources only)", false).
				StringArray("include", "Content types to include: rules, context, skills, mcp", false).
				Enum("merge_strategy", "Merge strategy", []string{"default", "override", "append"}, false).
				String("install_to", "Installation target path (optional)", false).
				WorkingDirectory(),
		),
		handlers.AddIncludeHandler,
	)

	s.addTool(
		newAnnotatedTool("remove_include", "Remove an include source from the configuration",
			newSchemaBuilder().
				String("name", "Include name to remove", true).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.RemoveIncludeHandler,
	)

	s.addTool(
		newAnnotatedTool("list_includes", "List all include sources in the configuration",
			newSchemaBuilder().WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListIncludesHandler,
	)

	// Installed skill tools
	s.addTool(
		newTool("install_skill", "Install a named skill from a git repository or local path",
			newSchemaBuilder().
				String("name", "Skill name (unique identifier)", true).
				String("source", "Git URL or local filesystem path", true).
				String("path", "Path within repo to skill directory (defaults to skills/<name>)", false).
				String("ref", "Git reference: branch, tag, or commit hash", false).
				WorkingDirectory(),
		),
		handlers.InstallSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("uninstall_skill", "Remove an installed skill from the configuration",
			newSchemaBuilder().
				String("name", "Skill name to remove", true).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.UninstallSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("list_installed_skills", "List all installed skills",
			newSchemaBuilder().WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListInstalledSkillsHandler,
	)

	// Config tools
	s.addTool(
		newAnnotatedTool("read_config", "Read the current project configuration and return its fields as structured JSON",
			newSchemaBuilder().
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadConfigHandler,
	)

	s.addTool(
		newTool("update_config", "Update specific fields in the project configuration",
			newSchemaBuilder().
				String("name", "Project name", false).
				String("description", "Project description", false).
				StringArray("builtins", "List of builtin names to enable (replaces current builtins setting)", false).
				Boolean("gitignore", "Whether to update .gitignore when generating outputs", false).
				WorkingDirectory(),
		),
		handlers.UpdateConfigHandler,
	)

	// Profile tools
	s.addTool(
		newTool("add_profile", "Create a new profile with a set of domains",
			newSchemaBuilder().
				String("name", "Profile name (unique identifier)", true).
				StringArray("domains", "List of domain names to include in the profile", true).
				WorkingDirectory(),
		),
		handlers.AddProfileHandler,
	)

	s.addTool(
		newAnnotatedTool("remove_profile", "Remove a profile from the configuration",
			newSchemaBuilder().
				String("name", "Profile name to remove", true).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.RemoveProfileHandler,
	)

	s.addTool(
		newTool("set_default_profile", "Set a profile as the default",
			newSchemaBuilder().
				String("name", "Profile name to set as default", true).
				WorkingDirectory(),
		),
		handlers.SetDefaultProfileHandler,
	)

	s.addTool(
		newAnnotatedTool("list_profiles", "List all profiles in the configuration",
			newSchemaBuilder().WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListProfilesHandler,
	)
}
