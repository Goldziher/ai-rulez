// Excerpt of https://github.com/letta-ai/letta-code/blob/v0.34.16/src/cli/commands/registry.ts
export const commands: Record<string, Command> = {
  // === Page 1: Most commonly used (order 10-19) ===
  "/agents": {
    desc: "Browse agents (pinned, Letta Code, all)",
    order: 10,
    noArgs: true,
    handler: () => {
      // Handled specially in App.tsx to open agent browser
      return "Opening agent browser...";
    },
  },
  "/clear-messages": {
    desc: "Reset all agent messages (destructive)",
    hidden: true,
    noArgs: true,
    handler: () => {
      // Handled specially in App.tsx to reset agent messages
      return "Resetting agent messages...";
    },
  },
  "/chdir": {
    desc: "Change working directory for this TUI session (/chdir <path>)",
    args: "<path>",
    order: 18.5,
    handler: () => {
      // Handled specially in App.tsx to switch local cwd
      return "Changing working directory...";
    },
  },
  "/cd": {
    desc: "Alias for /chdir",
    args: "<path>",
    hidden: true,
    handler: () => {
      // Handled specially in App.tsx to switch local cwd
      return "Changing working directory...";
    },
  },

  // === Page 2: Agent management (order 20-29) ===
  "/pinned": {
    desc: "Browse pinned agents",
    hidden: true, // Alias for /agents (opens to Pinned tab)
    noArgs: true,
    handler: () => {
      return "Opening agent browser...";
    },
  },
  // "/removed": {
  //   desc: "gone",
  // },
};
