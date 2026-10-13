package templates

import "strings"

// InitAgentsConfigTOML is the configuration `ai-rulez init --source agents`
// writes. The content comes from AGENTS.md and .agents, so the file carries only
// what the standards cannot hold (presets, hooks, MCP servers, permissions).
// inAgentsDir is true for .agents/ai-rulez.toml, where the location already
// selects the source; in any other config file `source = "agents"` says it.
func InitAgentsConfigTOML(projectName string, presets []string, inAgentsDir bool) string {
	doc := InitConfigTOML(projectName, presets)
	note := "# Content: the root AGENTS.md and the .agents directory (Agent Skills in .agents/skills).\n" +
		"# ai-rulez translates them to the other tools and never writes AGENTS.md or .agents.\n"
	if !inAgentsDir {
		note += "source = \"agents\"\n"
	}
	return strings.Replace(doc, "# Optional description", note+"\n# Optional description", 1)
}

// InitAgentsMD is the starter AGENTS.md written when a project has none.
func InitAgentsMD(projectName string) string {
	if strings.TrimSpace(projectName) == "" {
		projectName = "project"
	}
	return "# " + projectName + "\n\nInstructions for AI coding agents working in this repository.\n\n" +
		"## Conventions\n\n- Describe the build, test and lint commands here.\n- Describe the code style the agents must follow.\n"
}
