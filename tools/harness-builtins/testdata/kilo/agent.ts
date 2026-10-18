// Excerpt of https://github.com/Kilo-Org/kilocode/blob/v7.8.14/packages/opencode/src/agent/agent.ts (lines 167-170, 245-252, 274-280, 326-331)
        const agents: Record<string, Info> = {
          build: {
            name: "build",
            description: "The default agent. Executes tools based on configured permissions.",
// ...
            native: true,
          },
          // kilocode_change start - retain Kilo's opt-in repository research agent
          ...(flags.experimentalScout
            ? {
                scout: {
                  name: "scout",
                  permission: Permission.merge(
// ...
                  native: true,
                },
              }
            : {}),
          // kilocode_change end
          compaction: {
            name: "compaction",
// ...

        // kilocode_change start - rename build→code, add debug/orchestrator/ask, patch plan/explore
        KiloAgent.patchAgents(agents, defaults, user, kilo, ctx.worktree, whitelistedDirs)

        const agentConfigs = KiloAgent.preprocessConfig(cfg.agent ?? {})
        for (const [key, value] of Object.entries(agentConfigs)) {
