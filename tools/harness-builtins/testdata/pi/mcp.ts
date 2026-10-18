// Excerpt of https://github.com/earendil-works/pi/blob/v1.3.0/packages/coding-agent/src/extensions/mcp/index.ts (lines 1260-1262)
		pi.registerCommand("mcp", {
			description: "Manage MCP servers: sign in, reconnect, enable or disable, and change exposure",
			getArgumentCompletions: (prefix) => {
