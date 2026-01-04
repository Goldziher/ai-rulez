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
				String("config_file", "Path to the root configuration file (optional)", false),
		),
		handlers.GenerateOutputsHandler,
	)

	s.addTool(
		newTool("validate_config", "Validate the configuration file, including all includes",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file to validate (optional)", false),
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
				Boolean("popular_providers", "Enable a curated list of popular providers", false),
		),
		handlers.InitProjectHandler,
	)
}

func (s *Server) registerUtilityTools() {
	s.addTool(
		newTool("get_version", "Get the ai-rulez version", nil),
		handlers.GetVersionHandler(s.version),
	)
}

func (s *Server) registerCRUDTools() {
	// Domain tools
	s.addTool(
		newTool("create_domain", "Create a new domain with subdirectories for rules, context, and skills",
			newSchemaBuilder().
				String("name", "Domain name (alphanumeric and underscores, 1-50 characters)", true).
				String("description", "Optional domain description", false),
		),
		handlers.CreateDomainHandler,
	)

	s.addTool(
		newTool("delete_domain", "Delete a domain and all its contents",
			newSchemaBuilder().
				String("name", "Domain name to delete", true),
		),
		handlers.DeleteDomainHandler,
	)

	s.addTool(
		newTool("list_domains", "List all domains in the .ai-rulez directory", nil),
		handlers.ListDomainsHandler,
	)

	// Rule tools
	s.addTool(
		newTool("create_rule", "Create a new rule file with optional YAML frontmatter",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("content", "Markdown content with optional YAML frontmatter", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				String("priority", "Priority level: critical, high, medium, or low", false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false),
		),
		handlers.CreateRuleHandler,
	)

	s.addTool(
		newTool("update_rule", "Update an existing rule file",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				String("priority", "Priority level: critical, high, medium, or low", false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false),
		),
		handlers.UpdateRuleHandler,
	)

	s.addTool(
		newTool("delete_rule", "Delete a rule file",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false),
		),
		handlers.DeleteRuleHandler,
	)

	s.addTool(
		newTool("list_rules", "List all rules in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root rules if not specified)", false),
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
				String("priority", "Priority level: critical, high, medium, or low", false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false),
		),
		handlers.CreateContextHandler,
	)

	s.addTool(
		newTool("update_context", "Update an existing context file",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				String("priority", "Priority level: critical, high, medium, or low", false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false),
		),
		handlers.UpdateContextHandler,
	)

	s.addTool(
		newTool("delete_context", "Delete a context file",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false),
		),
		handlers.DeleteContextHandler,
	)

	s.addTool(
		newTool("list_context", "List all context files in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root context if not specified)", false),
		),
		handlers.ListContextHandler,
	)

	s.addTool(
		newTool("list_contexts", "List all context files with their names and summaries",
			newSchemaBuilder().
				String("domain", "Domain name (optional)", false),
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
				String("priority", "Priority level: critical, high, medium, or low", false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false),
		),
		handlers.CreateSkillHandler,
	)

	s.addTool(
		newTool("update_skill", "Update an existing skill file",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				String("priority", "Priority level: critical, high, medium, or low", false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false),
		),
		handlers.UpdateSkillHandler,
	)

	s.addTool(
		newTool("delete_skill", "Delete a skill file",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false),
		),
		handlers.DeleteSkillHandler,
	)

	s.addTool(
		newTool("list_skills", "List all skill files in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root skills if not specified)", false),
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
				String("merge_strategy", "Merge strategy: default, override, or append", false).
				String("install_to", "Installation target path (optional)", false),
		),
		handlers.AddIncludeHandler,
	)

	s.addTool(
		newTool("remove_include", "Remove an include source from the configuration",
			newSchemaBuilder().
				String("name", "Include name to remove", true),
		),
		handlers.RemoveIncludeHandler,
	)

	s.addTool(
		newTool("list_includes", "List all include sources in the configuration", nil),
		handlers.ListIncludesHandler,
	)

	// Profile tools
	s.addTool(
		newTool("add_profile", "Create a new profile with a set of domains",
			newSchemaBuilder().
				String("name", "Profile name (unique identifier)", true).
				StringArray("domains", "List of domain names to include in the profile", true),
		),
		handlers.AddProfileHandler,
	)

	s.addTool(
		newTool("remove_profile", "Remove a profile from the configuration",
			newSchemaBuilder().
				String("name", "Profile name to remove", true),
		),
		handlers.RemoveProfileHandler,
	)

	s.addTool(
		newTool("set_default_profile", "Set a profile as the default",
			newSchemaBuilder().
				String("name", "Profile name to set as default", true),
		),
		handlers.SetDefaultProfileHandler,
	)

	s.addTool(
		newTool("list_profiles", "List all profiles in the configuration", nil),
		handlers.ListProfilesHandler,
	)
}
