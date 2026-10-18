// Excerpt of https://github.com/can1357/oh-my-pi/blob/v18.12.0/packages/coding-agent/src/task/agents.ts
const EMBEDDED_AGENT_DEFS: EmbeddedAgentDef[] = [
	{ fileName: "scout.md", template: scoutMd },
	// { fileName: "removed.md", template: removedMd },
	{
		fileName: "task.md",
		frontmatter: {
			name: "task",
			description: "General-purpose subagent with full capabilities for delegated multi-step tasks",
		},
		template: taskMd,
	},
];
