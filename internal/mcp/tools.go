package mcp

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/mcp/handlers"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// JSON-schema literals used by tool schema builders.
const (
	jsonTypeString = "string"
	jsonKeyType    = "type"
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
		jsonKeyType: jsonTypeString,
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
		jsonKeyType: "array",
		"items":     map[string]any{jsonKeyType: jsonTypeString},
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
		jsonKeyType: "boolean",
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
		jsonKeyType: "number",
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
		jsonKeyType:            "object",
		"additionalProperties": map[string]any{jsonKeyType: jsonTypeString},
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
		jsonKeyType: jsonTypeString,
		"enum":      enumValues,
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
		jsonKeyType:  "object",
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

// additiveAnnotations marks a writing tool that performs only additive,
// non-destructive updates (create/add operations). Without this, clients
// assume the MCP default DestructiveHint=true and may warn on a harmless create.
func additiveAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
	}
}

// idempotentAnnotations marks a non-destructive writing tool whose repeated
// calls with the same arguments have no additional effect (update/set/generate).
func idempotentAnnotations() *sdkmcp.ToolAnnotations {
	return &sdkmcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		IdempotentHint:  true,
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
		if err == nil && (res == nil || !res.IsError) {
			s.emitToolTelemetry(ctx, tool.Name, wrapper)
		}
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
		newAnnotatedTool("generate_outputs", "Generate output files from the current configuration, respecting includes and extends",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file (optional)", false).
				String("config_dir", "Configuration directory name (default: .ai-rulez)", false).
				Boolean("dry_run", "Preview changes without writing files", false).
				Boolean("recursive", "Generate for all subdirectories containing .ai-rulez/", false).
				Boolean("no_local", "Ignore the machine-local config.local.* overlay and local/ content (the teammate view)", false).
				WorkingDirectory(),
			idempotentAnnotations(),
		),
		handlers.GenerateOutputsHandler,
	)

	s.addTool(
		newAnnotatedTool("clean_outputs", "Remove the files produced by generate (the inverse of generate_outputs): generated assistant files, the generated manifest, and the ai-rulez managed .gitignore block. The .ai-rulez/ source tree is never touched.",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file (optional)", false).
				String("config_dir", "Configuration directory name (default: .ai-rulez)", false).
				Boolean("dry_run", "Preview what would be removed without deleting", false).
				Boolean("keep_gitignore", "Leave the ai-rulez managed block in .gitignore", false).
				Boolean("keep_manifest", "Leave the generated manifest in place", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.CleanOutputsHandler,
	)

	s.addTool(
		newAnnotatedTool("validate_config", "Validate the configuration file, including all includes",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file to validate (optional)", false).
				String("config_dir", "Configuration directory name (default: .ai-rulez)", false).
				Boolean("no_local", "Validate without the machine-local config.local.* overlay", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ValidateConfigHandler,
	)

	s.addTool(
		newAnnotatedTool("doctor", "Run read-only diagnostics: config validity, preset names, generated-output drift, gitignore coverage, shared settings documents, MCP env placeholders, hook scripts, lock file, and tool binaries. Returns findings by severity (error, warning, info).",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file (optional)", false).
				String("config_dir", "Configuration directory name (default: .ai-rulez)", false).
				String("profile", "Profile to render for the drift and gitignore checks", false).
				Boolean("strict", "Treat warnings as failures in the ok field", false).
				Boolean("no_local", "Ignore the machine-local config.local.* overlay and local/ content", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.DoctorHandler,
	)

	s.addTool(
		newAnnotatedTool("init_project", "Initialize a new ai-rulez project in the current directory",
			newSchemaBuilder().
				String("project_name", "The name for the new project", false).
				StringArray("providers", "A list of providers to enable (e.g., ['claude', 'cursor'])", false).
				Boolean("with_agents", "Include sample agent configurations", false).
				Boolean("all_providers", "Enable all supported providers", false).
				Boolean("popular_providers", "Enable a curated list of popular providers", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.InitProjectHandler,
	)
}

func (s *Server) registerUtilityTools() {
	s.addTool(
		newAnnotatedTool("get_version", "Get the ai-rulez version", nil, readOnlyAnnotations()),
		handlers.GetVersionHandler(s.version),
	)

	s.addTool(
		newAnnotatedTool("show_builtin", "Show the full content of a builtin domain (rules, context, skills)",
			newSchemaBuilder().
				String("name", "Builtin domain name (e.g., security, rust, typescript)", true),
			readOnlyAnnotations(),
		),
		handlers.ShowBuiltinHandler,
	)
}

var priorityValues = []string{"critical", "high", "medium", "low", "minimal"}

func (s *Server) registerCRUDTools() {
	// Domain tools
	s.addTool(
		newAnnotatedTool("create_domain", "Create a new domain with subdirectories for rules, context, and skills",
			newSchemaBuilder().
				String("name", "Domain name (alphanumeric and underscores, 1-50 characters)", true).
				String("description", "Optional domain description", false).
				WorkingDirectory(),
			additiveAnnotations(),
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
		newAnnotatedTool("create_rule", "Create a new rule file with optional YAML frontmatter",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("content", "Markdown content with optional YAML frontmatter", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.CreateRuleHandler,
	)

	s.addTool(
		newAnnotatedTool("read_rule", "Read the content of a rule file",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadRuleHandler,
	)

	s.addTool(
		newAnnotatedTool("update_rule", "Update an existing rule file atomically",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			idempotentAnnotations(),
		),
		handlers.UpdateRuleHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_rule", "Delete a rule file",
			newSchemaBuilder().
				String("name", "Rule filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteRuleHandler,
	)

	s.addTool(
		newAnnotatedTool("list_rules", "List all rules in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root rules if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListRulesHandler,
	)

	// Check tools (code-review guidelines)
	s.addTool(
		newAnnotatedTool("create_check", "Create a new code-review check file (.ai-rulez/checks/<name>.md)",
			newSchemaBuilder().
				String("name", "Check filename without .md extension (letters, digits, '.', '_', '-')", true).
				String("content", "Markdown body, or a full file with YAML frontmatter", false).
				String("description", "Short summary of the check", false).
				Enum("severity", "Severity level", []string{"low", "medium", "high", "critical"}, false).
				StringArray("tools", "Tool names the check may use", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				StringArray("targets", "Target providers (e.g., cursor, kilo)", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.CreateCheckHandler,
	)

	s.addTool(
		newAnnotatedTool("read_check", "Read the content of a check file",
			newSchemaBuilder().
				String("name", "Check filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadCheckHandler,
	)

	s.addTool(
		newAnnotatedTool("update_check", "Update an existing check file atomically: give content, a field (description, severity, tools, targets), or both",
			newSchemaBuilder().
				String("name", "Check filename without .md extension", true).
				String("content", "New markdown body (the existing frontmatter is kept), or a full file with YAML frontmatter. Optional when a field below is given", false).
				String("description", "Short summary of the check; given fields are set on the existing frontmatter", false).
				Enum("severity", "Severity level", []string{"low", "medium", "high", "critical"}, false).
				StringArray("tools", "Tool names the check may use", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				StringArray("targets", "Target presets or path globs (e.g., cursor, kilo, src/**)", false).
				WorkingDirectory(),
			idempotentAnnotations(),
		),
		handlers.UpdateCheckHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_check", "Delete a check file",
			newSchemaBuilder().
				String("name", "Check filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteCheckHandler,
	)

	s.addTool(
		newAnnotatedTool("list_checks", "List all checks in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root checks if not specified)", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListChecksHandler,
	)

	// Context tools
	s.addTool(
		newAnnotatedTool("create_context", "Create a new context file with optional YAML frontmatter",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("content", "Markdown content with optional YAML frontmatter", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.CreateContextHandler,
	)

	s.addTool(
		newAnnotatedTool("read_context", "Read the content of a context file",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadContextHandler,
	)

	s.addTool(
		newAnnotatedTool("update_context", "Update an existing context file atomically",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			idempotentAnnotations(),
		),
		handlers.UpdateContextHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_context", "Delete a context file",
			newSchemaBuilder().
				String("name", "Context filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteContextHandler,
	)

	s.addTool(
		newAnnotatedTool("list_context", "List all context files in the root or a specific domain with summaries",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root context if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListContextsHandler,
	)

	// Skill tools
	s.addTool(
		newAnnotatedTool("create_skill", "Create a new skill file with optional YAML frontmatter",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("content", "Markdown content with optional YAML frontmatter", false).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.CreateSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("read_skill", "Read the content of a skill file",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ReadSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("update_skill", "Update an existing skill file atomically",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("content", "New markdown content", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Enum("priority", "Priority level", priorityValues, false).
				StringArray("targets", "Target providers (e.g., claude, cursor)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			idempotentAnnotations(),
		),
		handlers.UpdateSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("delete_skill", "Delete a skill file",
			newSchemaBuilder().
				String("name", "Skill filename without .md extension", true).
				String("domain", "Domain name (optional, uses root if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.DeleteSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("list_skills", "List all skill files in the root or a specific domain",
			newSchemaBuilder().
				String("domain", "Domain name (optional, lists root skills if not specified)", false).
				Boolean("local", "Work on the machine-local tree (.ai-rulez/local/, gitignored) instead of the shared content", false).
				WorkingDirectory(),
			readOnlyAnnotations(),
		),
		handlers.ListSkillsHandler,
	)

	// Include tools
	s.addTool(
		newAnnotatedTool("add_include", "Add a new include source (git URL or local path) to the configuration",
			newSchemaBuilder().
				String("name", "Include name (unique identifier)", true).
				String("source", "Git URL or local filesystem path", true).
				String("path", "Path within git repository (git sources only)", false).
				String("ref", "Git reference: branch, tag, or commit hash (git sources only)", false).
				StringArray("include", "Content types to include: rules, context, skills, agents, commands", false).
				Enum("merge_strategy", "Merge strategy", []string{"local-override", "include-override", "error"}, false).
				String("install_to", "Installation target path (optional)", false).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.AddIncludeHandler,
	)

	s.addTool(
		newAnnotatedTool("remove_include", "Remove an include source from the configuration",
			newSchemaBuilder().
				String("name", "Include name to remove", true).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
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
		newAnnotatedTool("install_skill", "Install a named skill from a git repository or local path",
			newSchemaBuilder().
				String("name", "Skill name (unique identifier)", true).
				String("source", "Git URL or local filesystem path", true).
				String("path", "Path within repo to skill directory (defaults to skills/<name>)", false).
				String("ref", "Git reference: branch, tag, or commit hash", false).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.InstallSkillHandler,
	)

	s.addTool(
		newAnnotatedTool("uninstall_skill", "Remove an installed skill from the configuration",
			newSchemaBuilder().
				String("name", "Skill name to remove", true).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
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
		newAnnotatedTool("update_config", "Update specific fields in the project configuration",
			newSchemaBuilder().
				String("name", "Project name", false).
				String("description", "Project description", false).
				StringArray("builtins", "List of builtin names to enable (replaces current builtins setting)", false).
				Boolean("gitignore", "Whether to update .gitignore when generating outputs", false).
				Boolean("agents_md", "Render always-on rules and context once into a shared AGENTS.md and skills once into .agents/skills for tools that read them", false).
				String("default_effort", "Default reasoning effort for Claude Code subagents (low, medium, high, xhigh, max, inherit). Empty string clears the default.", false).
				Object("default_effort_by_preset", "Per-preset reasoning effort override (e.g. {\"codex\": \"high\", \"claude\": \"xhigh\"}). Each value must be one of low, medium, high, xhigh, max, inherit. Pass {} to clear.", false).
				String("rules_mode", "Default rules output mode: split (one file per rule, default) or inline (rules embedded in the root file). Empty string clears it.", false).
				Object("rules_mode_by_preset", "Per-preset rules mode override (e.g. {\"claude\": \"split\", \"cursor\": \"inline\"}). Each value must be split or inline. Pass {} to clear.", false).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
				WorkingDirectory(),
			idempotentAnnotations(),
		),
		handlers.UpdateConfigHandler,
	)

	// Profile tools
	s.addTool(
		newAnnotatedTool("add_profile", "Create a new profile with a set of domains",
			newSchemaBuilder().
				String("name", "Profile name (unique identifier)", true).
				StringArray("domains", "List of domain names to include in the profile. A builtin pack is referenced as 'builtin:<name>' and is scoped to this profile.", true).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
				WorkingDirectory(),
			additiveAnnotations(),
		),
		handlers.AddProfileHandler,
	)

	s.addTool(
		newAnnotatedTool("remove_profile", "Remove a profile from the configuration",
			newSchemaBuilder().
				String("name", "Profile name to remove", true).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
				WorkingDirectory(),
			destructiveAnnotations(),
		),
		handlers.RemoveProfileHandler,
	)

	s.addTool(
		newAnnotatedTool("set_default_profile", "Set a profile as the default",
			newSchemaBuilder().
				String("name", "Profile name to set as default", true).
				Boolean("local", "Apply to the machine-local config.local.* overlay instead of the shared config", false).
				WorkingDirectory(),
			idempotentAnnotations(),
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
