// Excerpt of https://github.com/openclaw/openclaw/blob/v2026.10.4/src/auto-reply/commands-registry.shared.ts
return [
  // defineBuiltinCommand("removed", "Gone.", "status", "power"),
  defineBuiltinCommand("help", "Show available commands.", "status", "essential", {
    activeRunSafe: true,
    modelIndependent: "always",
  }),
  defineBuiltinCommand("dashboard", "Create or update this session's dashboard.", "tools", "standard", {
    args: [
      defineCommandArgument("request", "Dashboard requirements", {
        captureRemaining: true,
      }),
    ],
  }),
  defineBuiltinCommand("btw", "Ask a side question without changing future session context.", "tools", "standard", {
    activeRunSafe: true,
    modelIndependent: "no-args",
    nativeAliases: ["side"],
    textAliases: ["/btw", "/side"],
    acceptsArgs: true,
  }),
  defineBuiltinCommand(
    "export-session",
    "Export current session to an owner-only HTML file in the workspace.",
    "status",
    "essential",
    {
      modelIndependent: "always",
      textAliases: ["/export-session", "/export"],
      args: [
        defineCommandArgument("path", "Output path inside workspace (default: workspace)", {
          required: false,
        }),
      ],
    },
  ),
  defineBuiltinCommand("reset", "Reset the current session.", "session", "essential", {
    activeRunSafe: true,
    acceptsArgs: true,
  }),
  defineBuiltinCommand("think", "Set thinking level.", "options", "essential", {
    modelIndependent: "always",
    textAliases: ["/think", "/thinking", "/t"],
    activeRunSafe: true,
    args: [
      defineCommandArgument("level", "Thinking level", {
        choices: ({ provider, model, catalog, agentRuntime }) =>
          listThinkingLevelChoices(provider, model, catalog, agentRuntime),
      }),
    ],
    argsMenu: "auto",
  }),
];
