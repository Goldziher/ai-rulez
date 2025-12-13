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
	s.registerRulesTools()
	s.registerSectionsTools()
	s.registerAgentsTools()
	s.registerOutputsTools()
	s.registerMCPServersTools()
	s.registerCommandsTools()
	s.registerSingletonTools()
	s.registerProjectTools()
	s.registerUtilityTools()
}

func (s *Server) registerRulesTools() {
	s.addTool(
		newTool("list_rules", "List all rules from the configuration", nil),
		handlers.ListRulesHandler,
	)

	s.addTool(
		newTool("get_rule", "Get a specific rule by name",
			newSchemaBuilder().
				String("name", "The name of the rule to retrieve", true),
		),
		handlers.GetRuleHandler,
	)

	s.addTool(
		newTool("add_rule", "Add a new rule to the configuration",
			newSchemaBuilder().
				String("name", "The rule name", true).
				String("content", "The rule content", true).
				String("id", "Optional unique identifier for the rule", false).
				String("priority", "Rule priority (critical, high, medium, low, minimal)", false).
				StringArray("targets", "List of output targets for this rule", false),
		),
		handlers.AddRuleHandler,
	)

	s.addTool(
		newTool("update_rule", "Update an existing rule",
			newSchemaBuilder().
				String("name", "The current name of the rule to update", true).
				String("id", "New unique identifier for the rule", false).
				String("new_name", "New name for the rule", false).
				String("content", "New content for the rule", false).
				String("priority", "New priority for the rule", false).
				StringArray("targets", "New list of output targets", false),
		),
		handlers.UpdateRuleHandler,
	)

	s.addTool(
		newTool("delete_rule", "Delete a rule from the configuration",
			newSchemaBuilder().
				String("name", "The name of the rule to delete", true),
		),
		handlers.DeleteRuleHandler,
	)
}

func (s *Server) registerSectionsTools() {
	s.addTool(
		newTool("list_sections", "List all sections from the configuration", nil),
		handlers.ListSectionsHandler,
	)

	s.addTool(
		newTool("get_section", "Get a specific section by name",
			newSchemaBuilder().
				String("name", "The name of the section to retrieve", true),
		),
		handlers.GetSectionHandler,
	)

	s.addTool(
		newTool("add_section", "Add a new section to the configuration",
			newSchemaBuilder().
				String("name", "The section name", true).
				String("content", "The section content", true).
				String("id", "Optional unique identifier for the section", false).
				String("priority", "Section priority (critical, high, medium, low, minimal)", false).
				StringArray("targets", "List of output targets for this section", false),
		),
		handlers.AddSectionHandler,
	)

	s.addTool(
		newTool("update_section", "Update an existing section",
			newSchemaBuilder().
				String("name", "The current name of the section to update", true).
				String("id", "New unique identifier for the section", false).
				String("new_name", "New name for the section", false).
				String("content", "New content for the section", false).
				String("priority", "New priority for the section", false).
				StringArray("targets", "New list of output targets", false),
		),
		handlers.UpdateSectionHandler,
	)

	s.addTool(
		newTool("delete_section", "Delete a section from the configuration",
			newSchemaBuilder().
				String("name", "The name of the section to delete", true),
		),
		handlers.DeleteSectionHandler,
	)
}

func (s *Server) registerAgentsTools() {
	s.addTool(
		newTool("list_agents", "List all agents from the configuration", nil),
		handlers.ListAgentsHandler,
	)

	s.addTool(
		newTool("get_agent", "Get a specific agent by name",
			newSchemaBuilder().
				String("name", "The name of the agent to retrieve", true),
		),
		handlers.GetAgentHandler,
	)

	s.addTool(
		newTool("add_agent", "Add a new agent to the configuration",
			newSchemaBuilder().
				String("name", "The agent name", true).
				String("description", "Agent description", true).
				String("id", "Optional unique identifier for the agent", false).
				String("priority", "Agent priority (critical, high, medium, low, minimal)", false).
				StringArray("tools", "List of tools available to the agent", false).
				String("model", "Optional model identifier for this agent", false).
				String("system_prompt", "System prompt for the agent", false).
				String("template_type", "Template type: builtin, file, or inline", false).
				String("template_value", "Template value (name, path, or content)", false).
				StringArray("targets", "List of output targets for this agent", false),
		),
		handlers.AddAgentHandler,
	)

	s.addTool(
		newTool("update_agent", "Update an existing agent",
			newSchemaBuilder().
				String("name", "The current name of the agent to update", true).
				String("id", "New unique identifier for the agent", false).
				String("new_name", "New name for the agent", false).
				String("description", "New description for the agent", false).
				String("priority", "New priority for the agent", false).
				StringArray("tools", "New list of tools", false).
				String("model", "New model identifier for this agent", false).
				String("system_prompt", "New system prompt for the agent", false).
				String("template_type", "New template type for the agent", false).
				String("template_value", "New template value for the agent", false).
				StringArray("targets", "New list of output targets", false),
		),
		handlers.UpdateAgentHandler,
	)

	s.addTool(
		newTool("delete_agent", "Delete an agent from the configuration",
			newSchemaBuilder().
				String("name", "The name of the agent to delete", true),
		),
		handlers.DeleteAgentHandler,
	)
}

func (s *Server) registerOutputsTools() {
	s.addTool(
		newTool("list_outputs", "List all outputs from the configuration", nil),
		handlers.ListOutputsHandler,
	)

	s.addTool(
		newTool("get_output", "Get a specific output by path",
			newSchemaBuilder().
				String("path", "The path of the output to retrieve", true),
		),
		handlers.GetOutputHandler,
	)

	s.addTool(
		newTool("add_output", "Add a new output to the configuration",
			newSchemaBuilder().
				String("path", "The output path", true).
				String("type", "The type of content to output (rule, agent)", false).
				String("naming_scheme", "Naming scheme for directory outputs", false).
				String("template_type", "Template type: builtin, file, or inline", false).
				String("template_value", "Template value (name, path, or content)", false),
		),
		handlers.AddOutputHandler,
	)

	s.addTool(
		newTool("update_output", "Update an existing output",
			newSchemaBuilder().
				String("path", "The current path of the output to update", true).
				String("new_path", "New path for the output", false).
				String("type", "New type for the output", false).
				String("naming_scheme", "New naming scheme for the output", false).
				String("template_type", "New template type for the output", false).
				String("template_value", "New template value for the output", false),
		),
		handlers.UpdateOutputHandler,
	)

	s.addTool(
		newTool("delete_output", "Delete an output from the configuration",
			newSchemaBuilder().
				String("path", "The path of the output to delete", true),
		),
		handlers.DeleteOutputHandler,
	)
}

func (s *Server) registerMCPServersTools() {
	s.addTool(
		newTool("list_mcp_servers", "List all MCP servers", nil),
		handlers.ListMCPServersHandler,
	)

	s.addTool(
		newTool("get_mcp_server", "Get a specific MCP server by name",
			newSchemaBuilder().
				String("name", "The name of the server to retrieve", true),
		),
		handlers.GetMCPServerHandler,
	)

	s.addTool(
		newTool("add_mcp_server", "Add a new MCP server",
			newSchemaBuilder().
				String("id", "Optional unique identifier for the server", false).
				String("name", "The server name", true).
				String("description", "Optional description", false).
				String("command", "Command to start the server", false).
				StringArray("args", "Command line arguments", false).
				Object("env", "Environment variables", false).
				String("transport", "Transport protocol (stdio, sse, http)", false).
				String("url", "URL for remote servers", false).
				Boolean("enabled", "Whether the server is enabled", false).
				StringArray("targets", "List of output targets", false),
		),
		handlers.AddMCPServerHandler,
	)

	s.addTool(
		newTool("update_mcp_server", "Update an existing MCP server",
			newSchemaBuilder().
				String("name", "The name of the server to update", true).
				String("id", "New unique identifier for the server", false).
				String("new_name", "New name for the server", false).
				String("description", "New description", false).
				String("command", "New command", false).
				StringArray("args", "New list of arguments", false).
				Object("env", "New set of environment variables", false).
				String("transport", "New transport protocol", false).
				String("url", "New URL", false).
				Boolean("enabled", "New enabled state", false).
				StringArray("targets", "New list of targets", false),
		),
		handlers.UpdateMCPServerHandler,
	)

	s.addTool(
		newTool("delete_mcp_server", "Delete an MCP server",
			newSchemaBuilder().
				String("name", "The name of the server to delete", true),
		),
		handlers.DeleteMCPServerHandler,
	)
}

func (s *Server) registerCommandsTools() {
	s.addTool(
		newTool("list_commands", "List all custom commands", nil),
		handlers.ListCommandsHandler,
	)

	s.addTool(
		newTool("get_command", "Get a specific command by name",
			newSchemaBuilder().
				String("name", "The name of the command to retrieve", true),
		),
		handlers.GetCommandHandler,
	)

	s.addTool(
		newTool("add_command", "Add a new custom command",
			newSchemaBuilder().
				String("id", "Optional unique identifier for the command", false).
				String("name", "The command name", true).
				String("description", "Command description", true).
				StringArray("aliases", "Alternative names for the command", false).
				String("usage", "Usage example or pattern", false).
				String("system_prompt", "System prompt for the command", false).
				String("shortcut", "Keyboard shortcut", false).
				Boolean("enabled", "Whether the command is enabled", false).
				StringArray("targets", "List of output targets", false),
		),
		handlers.AddCommandHandler,
	)

	s.addTool(
		newTool("update_command", "Update an existing custom command",
			newSchemaBuilder().
				String("name", "The name of the command to update", true).
				String("id", "New unique identifier for the command", false).
				String("new_name", "New name for the command", false).
				String("description", "New description", false).
				StringArray("aliases", "New list of aliases", false).
				String("usage", "New usage example", false).
				String("system_prompt", "New system prompt", false).
				String("shortcut", "New keyboard shortcut", false).
				Boolean("enabled", "New enabled state", false).
				StringArray("targets", "New list of targets", false),
		),
		handlers.UpdateCommandHandler,
	)

	s.addTool(
		newTool("delete_command", "Delete a custom command",
			newSchemaBuilder().
				String("name", "The name of the command to delete", true),
		),
		handlers.DeleteCommandHandler,
	)
}

func (s *Server) registerSingletonTools() {
	s.addTool(
		newTool("get_metadata", "Get project metadata", nil),
		handlers.GetMetadataHandler,
	)

	s.addTool(
		newTool("set_metadata", "Set project metadata",
			newSchemaBuilder().
				String("name", "New project name", false).
				String("version", "New project version", false).
				String("description", "New project description", false),
		),
		handlers.SetMetadataHandler,
	)

	s.addTool(
		newTool("get_extends", "Get the extends property", nil),
		handlers.GetExtendsHandler,
	)

	s.addTool(
		newTool("set_extends", "Set the extends property",
			newSchemaBuilder().
				String("path_or_url", "Path or URL to a base configuration", true),
		),
		handlers.SetExtendsHandler,
	)

	s.addTool(
		newTool("delete_extends", "Delete the extends property", nil),
		handlers.DeleteExtendsHandler,
	)

	s.addTool(
		newTool("list_includes", "List all include paths", nil),
		handlers.ListIncludesHandler,
	)

	s.addTool(
		newTool("add_include", "Add an include path",
			newSchemaBuilder().
				String("path_or_url", "Path or URL to an ai_rules file to include", true),
		),
		handlers.AddIncludeHandler,
	)

	s.addTool(
		newTool("delete_include", "Delete an include path",
			newSchemaBuilder().
				String("path_or_url", "The path or URL to remove from the includes list", true),
		),
		handlers.DeleteIncludeHandler,
	)
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
