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
	s.registerProjectTools()
	s.registerUtilityTools()
}

// registerRulesTools registers rule-related tools
func (s *Server) registerRulesTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_rules",
			mcp.WithDescription("Get all rules from the configuration"),
			mcp.WithString("filter",
				mcp.Description("Optional filter to search rules by content"),
			),
		),
		handlers.GetRulesHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("add_rule",
			mcp.WithDescription("Add a new rule to the configuration"),
			mcp.WithString("name",
				mcp.Description("The rule name"),
				mcp.Required(),
			),
			mcp.WithString("content",
				mcp.Description("The rule content"),
				mcp.Required(),
			),
			mcp.WithString("section",
				mcp.Description("The section to add the rule to (optional)"),
			),
			mcp.WithNumber("priority",
				mcp.Description("Rule priority (1-10, optional)"),
			),
		),
		handlers.AddRuleHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("update_rule",
			mcp.WithDescription("Update an existing rule"),
			mcp.WithString("name",
				mcp.Description("The name of the rule to update"),
				mcp.Required(),
			),
			mcp.WithString("new_name",
				mcp.Description("New name for the rule (optional)"),
			),
			mcp.WithString("content",
				mcp.Description("The new rule content (optional)"),
			),
			mcp.WithNumber("priority",
				mcp.Description("New priority (optional)"),
			),
		),
		handlers.UpdateRuleHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("delete_rule",
			mcp.WithDescription("Delete a rule from the configuration"),
			mcp.WithNumber("index",
				mcp.Description("The index of the rule to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteRuleHandler,
	)
}

// registerSectionsTools registers section-related tools
func (s *Server) registerSectionsTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_sections",
			mcp.WithDescription("Get all sections from the configuration"),
			mcp.WithString("name",
				mcp.Description("Optional section name to filter by"),
			),
		),
		handlers.GetSectionsHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("add_section",
			mcp.WithDescription("Add a new section to the configuration"),
			mcp.WithString("name",
				mcp.Description("The section name"),
				mcp.Required(),
			),
			mcp.WithString("content",
				mcp.Description("The section content"),
				mcp.Required(),
			),
			mcp.WithNumber("priority",
				mcp.Description("Section priority (1-10, optional)"),
			),
		),
		handlers.AddSectionHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("update_section",
			mcp.WithDescription("Update an existing section"),
			mcp.WithString("name",
				mcp.Description("The current name of the section to update"),
				mcp.Required(),
			),
			mcp.WithString("new_name",
				mcp.Description("The new section name (optional)"),
			),
			mcp.WithString("content",
				mcp.Description("The new section content (optional)"),
			),
			mcp.WithNumber("priority",
				mcp.Description("New priority (optional)"),
			),
		),
		handlers.UpdateSectionHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("delete_section",
			mcp.WithDescription("Delete a section from the configuration"),
			mcp.WithNumber("index",
				mcp.Description("The index of the section to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteSectionHandler,
	)
}

// registerAgentsTools registers agent-related tools
func (s *Server) registerAgentsTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_agents",
			mcp.WithDescription("Get all agents from the configuration"),
			mcp.WithString("name",
				mcp.Description("Optional agent name to filter by"),
			),
		),
		handlers.GetAgentsHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("add_agent",
			mcp.WithDescription("Add a new agent to the configuration"),
			mcp.WithString("name",
				mcp.Description("The agent name"),
				mcp.Required(),
			),
			mcp.WithString("description",
				mcp.Description("Agent description"),
				mcp.Required(),
			),
			mcp.WithNumber("priority",
				mcp.Description("Agent priority (1-10, optional)"),
			),
			mcp.WithArray("tools",
				mcp.Description("List of tools available to the agent"),
			),
			mcp.WithString("system_prompt",
				mcp.Description("System prompt for the agent"),
			),
		),
		handlers.AddAgentHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("update_agent",
			mcp.WithDescription("Update an existing agent"),
			mcp.WithNumber("index",
				mcp.Description("The index of the agent to update"),
				mcp.Required(),
			),
			mcp.WithString("name",
				mcp.Description("The new agent name"),
			),
			mcp.WithString("description",
				mcp.Description("New description"),
			),
			mcp.WithNumber("priority",
				mcp.Description("New priority"),
			),
			mcp.WithArray("tools",
				mcp.Description("Updated list of tools"),
			),
			mcp.WithString("system_prompt",
				mcp.Description("New system prompt"),
			),
		),
		handlers.UpdateAgentHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("delete_agent",
			mcp.WithDescription("Delete an agent from the configuration"),
			mcp.WithNumber("index",
				mcp.Description("The index of the agent to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteAgentHandler,
	)
}

// registerOutputsTools registers output-related tools
func (s *Server) registerOutputsTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_outputs",
			mcp.WithDescription("Get all outputs from the configuration"),
			mcp.WithString("path",
				mcp.Description("Optional path to filter by"),
			),
		),
		handlers.GetOutputsHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("add_output",
			mcp.WithDescription("Add a new output to the configuration"),
			mcp.WithString("path",
				mcp.Description("The output path"),
				mcp.Required(),
			),
			mcp.WithString("type",
				mcp.Description("The type of content to output (rule, section, agent)"),
			),
			mcp.WithString("naming_scheme",
				mcp.Description("Naming scheme for directory outputs"),
			),
		),
		handlers.AddOutputHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("update_output",
			mcp.WithDescription("Update an existing output"),
			mcp.WithNumber("index",
				mcp.Description("The index of the output to update"),
				mcp.Required(),
			),
			mcp.WithString("path",
				mcp.Description("The new output path"),
			),
			mcp.WithString("type",
				mcp.Description("The new type (rule, section, agent)"),
			),
			mcp.WithString("naming_scheme",
				mcp.Description("New naming scheme"),
			),
		),
		handlers.UpdateOutputHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("delete_output",
			mcp.WithDescription("Delete an output from the configuration"),
			mcp.WithNumber("index",
				mcp.Description("The index of the output to delete"),
				mcp.Required(),
			),
		),
		handlers.DeleteOutputHandler,
	)
}

// registerProjectTools registers project-related tools
func (s *Server) registerProjectTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("generate_output",
			mcp.WithDescription("Generate output files from the current configuration"),
			mcp.WithString("config_file",
				mcp.Description("Path to the configuration file (optional)"),
			),
		),
		handlers.GenerateOutputHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("validate_config",
			mcp.WithDescription("Validate the configuration file"),
			mcp.WithString("config_file",
				mcp.Description("Path to the configuration file to validate"),
				mcp.Required(),
			),
		),
		handlers.ValidateConfigHandler,
	)

	s.mcpServer.AddTool(
		mcp.NewTool("init_project",
			mcp.WithDescription("Initialize a new ai-rulez project"),
			mcp.WithString("name",
				mcp.Description("Project name"),
				mcp.Required(),
			),
			mcp.WithString("preset",
				mcp.Description("Preset template to use (claude, cursor, windsurf, etc.)"),
			),
			mcp.WithString("use_agent",
				mcp.Description("AI agent to use for generation"),
			),
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
