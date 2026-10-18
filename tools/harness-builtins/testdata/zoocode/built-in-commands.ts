// Excerpt of https://github.com/Zoo-Code-Org/Zoo-Code/blob/v3.90.0/src/services/command/built-in-commands.ts (lines 1-13, 284-290)
import { Command } from "./commands"

interface BuiltInCommandDefinition {
	name: string
	description: string
	argumentHint?: string
	content: string
}

const BUILT_IN_COMMANDS: Record<string, BuiltInCommandDefinition> = {
	init: {
		name: "init",
		description: "Analyze codebase and create concise AGENTS.md files for AI assistants",
// ...

Remember: The goal is to create documentation that enables AI assistants to be immediately productive in this codebase, focusing on project-specific knowledge that isn't obvious from the code structure alone.`,
	},
}

/**
 * Get all built-in commands as Command objects
