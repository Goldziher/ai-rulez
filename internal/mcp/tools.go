package mcp

import (
	"github.com/Goldziher/ai-rulez/internal/mcp/handlers"
	"github.com/mark3labs/mcp-go/mcp"
)

// registerTools registers all MCP tools with their handlers
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

// registerRulesTools registers rule-related tools
func (s *Server) registerRulesTools() {
	// list_rules - consistent naming
	s.mcpServer.AddTool(
		mcp.NewTool("list_rules",
			mcp.WithDescription("List all rules from the configuration"),
		),
		handlers.ListRulesHandler,
	)

	// New: Get a single rule by name
	s.mcpServer.AddTool(
		mcp.NewTool("get_rule",
			mcp.WithDescription("Get a specific rule by name"),
			mcp.WithString("name",
				mcp.Description("The name of the rule to retrieve"),
				mcp.Required(),
			),
		),
		handlers.GetRuleHandler,
	)

	// Enhanced: Add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("add_rule",
			mcp.WithDescription("Add a new rule to the configuration"),
			mcp.WithString("name", mcp.Description("The rule name"), mcp.Required()),
			mcp.WithString("content", mcp.Description("The rule content"), mcp.Required()),
			mcp.WithString("id", mcp.Description("Optional unique identifier for the rule")),
			mcp.WithString("priority", mcp.Description("Rule priority (critical, high, medium, low, minimal)")),
			mcp.WithArray("targets", mcp.Description("List of output targets for this rule")),
		),
		handlers.AddRuleHandler,
	)

	// Enhanced: Use name identifier and add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("update_rule",
			mcp.WithDescription("Update an existing rule"),
			mcp.WithString("name", mcp.Description("The current name of the rule to update"), mcp.Required()),
			mcp.WithString("id", mcp.Description("New unique identifier for the rule")),
			mcp.WithString("new_name", mcp.Description("New name for the rule")),
			mcp.WithString("content", mcp.Description("New content for the rule")),
			mcp.WithString("priority", mcp.Description("New priority for the rule")),
			mcp.WithArray("targets", mcp.Description("New list of output targets")),
		),
		handlers.UpdateRuleHandler,
	)

	// Enhanced: Use name identifier
	s.mcpServer.AddTool(
		mcp.NewTool("delete_rule",
			mcp.WithDescription("Delete a rule from the configuration"),
			mcp.WithString("name",
				mcp.Description("The name of the rule to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteRuleHandler,
	)
}

// registerSectionsTools registers section-related tools
func (s *Server) registerSectionsTools() {
	// list_sections - consistent naming
	s.mcpServer.AddTool(
		mcp.NewTool("list_sections",
			mcp.WithDescription("List all sections from the configuration"),
		),
		handlers.ListSectionsHandler,
	)

	// New: Get a single section by name
	s.mcpServer.AddTool(
		mcp.NewTool("get_section",
			mcp.WithDescription("Get a specific section by name"),
			mcp.WithString("name",
				mcp.Description("The name of the section to retrieve"),
				mcp.Required(),
			),
		),
		handlers.GetSectionHandler,
	)

	// Enhanced: Add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("add_section",
			mcp.WithDescription("Add a new section to the configuration"),
			mcp.WithString("name", mcp.Description("The section name"), mcp.Required()),
			mcp.WithString("content", mcp.Description("The section content"), mcp.Required()),
			mcp.WithString("id", mcp.Description("Optional unique identifier for the section")),
			mcp.WithString("priority", mcp.Description("Section priority (critical, high, medium, low, minimal)")),
			mcp.WithArray("targets", mcp.Description("List of output targets for this section")),
		),
		handlers.AddSectionHandler,
	)

	// Enhanced: Use name identifier and add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("update_section",
			mcp.WithDescription("Update an existing section"),
			mcp.WithString("name", mcp.Description("The current name of the section to update"), mcp.Required()),
			mcp.WithString("id", mcp.Description("New unique identifier for the section")),
			mcp.WithString("new_name", mcp.Description("New name for the section")),
			mcp.WithString("content", mcp.Description("New content for the section")),
			mcp.WithString("priority", mcp.Description("New priority for the section")),
			mcp.WithArray("targets", mcp.Description("New list of output targets")),
		),
		handlers.UpdateSectionHandler,
	)

	// Enhanced: Use name identifier
	s.mcpServer.AddTool(
		mcp.NewTool("delete_section",
			mcp.WithDescription("Delete a section from the configuration"),
			mcp.WithString("name",
				mcp.Description("The name of the section to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteSectionHandler,
	)
}

// registerAgentsTools registers agent-related tools
func (s *Server) registerAgentsTools() {
	// list_agents - consistent naming
	s.mcpServer.AddTool(
		mcp.NewTool("list_agents",
			mcp.WithDescription("List all agents from the configuration"),
		),
		handlers.ListAgentsHandler,
	)

	// New: Get a single agent by name
	s.mcpServer.AddTool(
		mcp.NewTool("get_agent",
			mcp.WithDescription("Get a specific agent by name"),
			mcp.WithString("name",
				mcp.Description("The name of the agent to retrieve"),
				mcp.Required(),
			),
		),
		handlers.GetAgentHandler,
	)

	// Enhanced: Add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("add_agent",
			mcp.WithDescription("Add a new agent to the configuration"),
			mcp.WithString("name", mcp.Description("The agent name"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Agent description"), mcp.Required()),
			mcp.WithString("id", mcp.Description("Optional unique identifier for the agent")),
			mcp.WithString("priority", mcp.Description("Agent priority (critical, high, medium, low, minimal)")),
			mcp.WithArray("tools", mcp.Description("List of tools available to the agent")),
			mcp.WithString("system_prompt", mcp.Description("System prompt for the agent")),
			mcp.WithString("template_type", mcp.Description("Template type: builtin, file, or inline")),
			mcp.WithString("template_value", mcp.Description("Template value (name, path, or content)")),
			mcp.WithArray("targets", mcp.Description("List of output targets for this agent")),
		),
		handlers.AddAgentHandler,
	)

	// Enhanced: Use name identifier and add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("update_agent",
			mcp.WithDescription("Update an existing agent"),
			mcp.WithString("name", mcp.Description("The current name of the agent to update"), mcp.Required()),
			mcp.WithString("id", mcp.Description("New unique identifier for the agent")),
			mcp.WithString("new_name", mcp.Description("New name for the agent")),
			mcp.WithString("description", mcp.Description("New description for the agent")),
			mcp.WithString("priority", mcp.Description("New priority for the agent")),
			mcp.WithArray("tools", mcp.Description("New list of tools")),
			mcp.WithString("system_prompt", mcp.Description("New system prompt for the agent")),
			mcp.WithString("template_type", mcp.Description("New template type for the agent")),
			mcp.WithString("template_value", mcp.Description("New template value for the agent")),
			mcp.WithArray("targets", mcp.Description("New list of output targets")),
		),
		handlers.UpdateAgentHandler,
	)

	// Enhanced: Use name identifier
	s.mcpServer.AddTool(
		mcp.NewTool("delete_agent",
			mcp.WithDescription("Delete an agent from the configuration"),
			mcp.WithString("name",
				mcp.Description("The name of the agent to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteAgentHandler,
	)
}

// registerOutputsTools registers output-related tools
func (s *Server) registerOutputsTools() {
	// list_outputs - consistent naming
	s.mcpServer.AddTool(
		mcp.NewTool("list_outputs",
			mcp.WithDescription("List all outputs from the configuration"),
		),
		handlers.ListOutputsHandler,
	)

	// New: Get a single output by path
	s.mcpServer.AddTool(
		mcp.NewTool("get_output",
			mcp.WithDescription("Get a specific output by path"),
			mcp.WithString("path",
				mcp.Description("The path of the output to retrieve"),
				mcp.Required(),
			),
		),
		handlers.GetOutputHandler,
	)

	// Enhanced: Add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("add_output",
			mcp.WithDescription("Add a new output to the configuration"),
			mcp.WithString("path", mcp.Description("The output path"), mcp.Required()),
			mcp.WithString("type", mcp.Description("The type of content to output (rule, agent)")),
			mcp.WithString("naming_scheme", mcp.Description("Naming scheme for directory outputs")),
			mcp.WithString("template_type", mcp.Description("Template type: builtin, file, or inline")),
			mcp.WithString("template_value", mcp.Description("Template value (name, path, or content)")),
		),
		handlers.AddOutputHandler,
	)

	// Enhanced: Use path identifier and add all schema fields
	s.mcpServer.AddTool(
		mcp.NewTool("update_output",
			mcp.WithDescription("Update an existing output"),
			mcp.WithString("path", mcp.Description("The current path of the output to update"), mcp.Required()),
			mcp.WithString("new_path", mcp.Description("New path for the output")),
			mcp.WithString("type", mcp.Description("New type for the output")),
			mcp.WithString("naming_scheme", mcp.Description("New naming scheme for the output")),
			mcp.WithString("template_type", mcp.Description("New template type for the output")),
			mcp.WithString("template_value", mcp.Description("New template value for the output")),
		),
		handlers.UpdateOutputHandler,
	)

	// Enhanced: Use path identifier
	s.mcpServer.AddTool(
		mcp.NewTool("delete_output",
			mcp.WithDescription("Delete an output from the configuration"),
			mcp.WithString("path",
				mcp.Description("The path of the output to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteOutputHandler,
	)
}

// registerMCPServersTools registers mcp_server-related tools
func (s *Server) registerMCPServersTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_mcp_servers", mcp.WithDescription("List all MCP servers")),
		handlers.ListMCPServersHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("get_mcp_server",
			mcp.WithDescription("Get a specific MCP server by name"),
			mcp.WithString("name", mcp.Description("The name of the server to retrieve"), mcp.Required()),
		),
		handlers.GetMCPServerHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("add_mcp_server",
			mcp.WithDescription("Add a new MCP server"),
			mcp.WithString("id", mcp.Description("Optional unique identifier for the server")),
			mcp.WithString("name", mcp.Description("The server name"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description")),
			mcp.WithString("command", mcp.Description("Command to start the server")),
			mcp.WithArray("args", mcp.Description("Command line arguments")),
			mcp.WithObject("env", mcp.Description("Environment variables")),
			mcp.WithString("transport", mcp.Description("Transport protocol (stdio, sse, http)")),
			mcp.WithString("url", mcp.Description("URL for remote servers")),
			mcp.WithBoolean("enabled", mcp.Description("Whether the server is enabled")),
			mcp.WithArray("targets", mcp.Description("List of output targets")),
		),
		handlers.AddMCPServerHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("update_mcp_server",
			mcp.WithDescription("Update an existing MCP server"),
			mcp.WithString("name", mcp.Description("The name of the server to update"), mcp.Required()),
			mcp.WithString("id", mcp.Description("New unique identifier for the server")),
			mcp.WithString("new_name", mcp.Description("New name for the server")),
			mcp.WithString("description", mcp.Description("New description")),
			mcp.WithString("command", mcp.Description("New command")),
			mcp.WithArray("args", mcp.Description("New list of arguments")),
			mcp.WithObject("env", mcp.Description("New set of environment variables")),
			mcp.WithString("transport", mcp.Description("New transport protocol")),
			mcp.WithString("url", mcp.Description("New URL")),
			mcp.WithBoolean("enabled", mcp.Description("New enabled state")),
			mcp.WithArray("targets", mcp.Description("New list of targets")),
		),
		handlers.UpdateMCPServerHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("delete_mcp_server",
			mcp.WithDescription("Delete an MCP server"),
			mcp.WithString("name", mcp.Description("The name of the server to delete"), mcp.Required()),
		),
		handlers.DeleteMCPServerHandler,
	)
}

// registerCommandsTools registers command-related tools
func (s *Server) registerCommandsTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_commands", mcp.WithDescription("List all custom commands")),
		handlers.ListCommandsHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("get_command",
			mcp.WithDescription("Get a specific command by name"),
			mcp.WithString("name", mcp.Description("The name of the command to retrieve"), mcp.Required()),
		),
		handlers.GetCommandHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("add_command",
			mcp.WithDescription("Add a new custom command"),
			mcp.WithString("id", mcp.Description("Optional unique identifier for the command")),
			mcp.WithString("name", mcp.Description("The command name"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Command description"), mcp.Required()),
			mcp.WithArray("aliases", mcp.Description("Alternative names for the command")),
			mcp.WithString("usage", mcp.Description("Usage example or pattern")),
			mcp.WithString("system_prompt", mcp.Description("System prompt for the command")),
			mcp.WithString("shortcut", mcp.Description("Keyboard shortcut")),
			mcp.WithBoolean("enabled", mcp.Description("Whether the command is enabled")),
			mcp.WithArray("targets", mcp.Description("List of output targets")),
		),
		handlers.AddCommandHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("update_command",
			mcp.WithDescription("Update an existing custom command"),
			mcp.WithString("name", mcp.Description("The name of the command to update"), mcp.Required()),
			mcp.WithString("id", mcp.Description("New unique identifier for the command")),
			mcp.WithString("new_name", mcp.Description("New name for the command")),
			mcp.WithString("description", mcp.Description("New description")),
			mcp.WithArray("aliases", mcp.Description("New list of aliases")),
			mcp.WithString("usage", mcp.Description("New usage example")),
			mcp.WithString("system_prompt", mcp.Description("New system prompt")),
			mcp.WithString("shortcut", mcp.Description("New keyboard shortcut")),
			mcp.WithBoolean("enabled", mcp.Description("New enabled state")),
			mcp.WithArray("targets", mcp.Description("New list of targets")),
		),
		handlers.UpdateCommandHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("delete_command",
			mcp.WithDescription("Delete a custom command"),
			mcp.WithString("name", mcp.Description("The name of the command to delete"), mcp.Required()),
		),
		handlers.DeleteCommandHandler,
	)
}

// registerSingletonTools registers tools for managing top-level singleton properties
func (s *Server) registerSingletonTools() {
	// Metadata
	s.mcpServer.AddTool(mcp.NewTool("get_metadata", mcp.WithDescription("Get project metadata")),
		handlers.GetMetadataHandler)
	s.mcpServer.AddTool(
		mcp.NewTool("set_metadata",
			mcp.WithDescription("Set project metadata"),
			mcp.WithString("name", mcp.Description("New project name")),
			mcp.WithString("version", mcp.Description("New project version")),
			mcp.WithString("description", mcp.Description("New project description")),
		),
		handlers.SetMetadataHandler,
	)

	// Extends
	s.mcpServer.AddTool(mcp.NewTool("get_extends", mcp.WithDescription("Get the extends property")),
		handlers.GetExtendsHandler)
	s.mcpServer.AddTool(
		mcp.NewTool("set_extends",
			mcp.WithDescription("Set the extends property"),
			mcp.WithString("path_or_url", mcp.Description("Path or URL to a base configuration"), mcp.Required()),
		),
		handlers.SetExtendsHandler,
	)
	s.mcpServer.AddTool(mcp.NewTool("delete_extends", mcp.WithDescription("Delete the extends property")),
		handlers.DeleteExtendsHandler)

	// Includes
	s.mcpServer.AddTool(mcp.NewTool("list_includes", mcp.WithDescription("List all include paths")),
		handlers.ListIncludesHandler)
	s.mcpServer.AddTool(
		mcp.NewTool("add_include",
			mcp.WithDescription("Add an include path"),
			mcp.WithString("path_or_url", mcp.Description("Path or URL to an ai_rules file to include"), mcp.Required()),
		),
		handlers.AddIncludeHandler,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("delete_include",
			mcp.WithDescription("Delete an include path"),
			mcp.WithString("path_or_url", mcp.Description("The path or URL to remove from the includes list"), mcp.Required()),
		),
		handlers.DeleteIncludeHandler,
	)
}

// registerProjectTools registers project-related tools
func (s *Server) registerProjectTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("generate_outputs",
			mcp.WithDescription("Generate output files from the current configuration, respecting includes and extends"),
			mcp.WithString("config_file", mcp.Description("Path to the root configuration file (optional)")),
		),
		handlers.GenerateOutputsHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("validate_config",
			mcp.WithDescription("Validate the configuration file, including all includes"),
			mcp.WithString("config_file", mcp.Description("Path to the root configuration file to validate (optional)")),
		),
		handlers.ValidateConfigHandler,
	)

	// Overhauled init_project to match CLI capabilities
	s.mcpServer.AddTool(
		mcp.NewTool("init_project",
			mcp.WithDescription("Initialize a new ai-rulez project in the current directory"),
			mcp.WithString("project_name", mcp.Description("The name for the new project")),
			mcp.WithArray("providers", mcp.Description("A list of providers to enable (e.g., ['claude', 'cursor'])")),
			mcp.WithBoolean("with_agents", mcp.Description("Include sample agent configurations")),
			mcp.WithBoolean("all_providers", mcp.Description("Enable all supported providers")),
			mcp.WithBoolean("popular_providers", mcp.Description("Enable a curated list of popular providers")),
		),
		handlers.InitProjectHandler,
	)
}

// registerUtilityTools registers utility tools
func (s *Server) registerUtilityTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_version",
			mcp.WithDescription("Get the ai-rulez version"),
		),
		handlers.GetVersionHandler(s.version),
	)
}
