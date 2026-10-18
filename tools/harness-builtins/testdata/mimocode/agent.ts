// Excerpt of https://github.com/XiaomiMiMo/MiMo-Code/blob/b53297d8e6f01d3cfeecb75eac59b542869901e7/packages/cli/src/agent/agent.ts
        const agents: Record<string, Info> = {
          build: {
            name: "build",
            color: "#fb8147",
            mode: "primary",
            native: true,
          },
          // Max mode is experimental and opt-in: only registered when
          // `experimental.maxMode` is configured.
          ...(cfg.experimental?.maxMode
            ? {
                max: {
                  name: "max",
                  mode: "primary" as const,
                  native: true,
                },
              }
            : {}),
          // name: "commented-out",
          title: {
            name: "title",
            mode: "subagent",
            native: true,
            hidden: true,
          },
        }

        for (const [key, value] of Object.entries(cfg.agent ?? {})) {
