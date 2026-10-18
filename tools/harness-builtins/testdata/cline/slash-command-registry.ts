// Excerpt of https://github.com/cline/cline/blob/v4.1.25/apps/cli/src/tui/commands/slash-command-registry.ts
export type LocalSlashCommandName =
  | "settings"
  | "config"
  | "mcp"
  | "plugins"
  | "account"
  | "model"
  | "theme"
  | "compact"
  | "skills"
  | "fork"
  | "undo"
  | "clear"
  | "history"
  | "quit"
  | "help";

const TUI_LOCAL_COMMANDS: Array<{
  name: LocalSlashCommandName;
  description: string;
  visible?: boolean;
  preserveInput?: boolean;
}> = [
  {
    name: "settings",
    description: "Modify agent configuration",
  },
  {
    name: "config",
    description: "Modify agent configuration",
    visible: false,
  },
  {
    name: "model",
    description: "Switch model or provider",
  },
  {
    name: "theme",
    description: "Change color theme",
  },
  {
    name: "account",
    description: "View Cline account",
  },
  {
    name: "mcp",
    description: "Manage MCP servers",
  },
  {
    name: "plugins",
    description: "Manage plugins",
  },
  {
    name: "compact",
    description: "Compact context",
  },
  {
    name: "skills",
    description: "Browse skills and workflows",
    preserveInput: true,
  },
  {
    name: "fork",
    description: "Create a named fork of the current session",
  },
  {
    name: "undo",
    description: "Restore to a previous checkpoint",
  },
  {
    name: "clear",
    description: "Start a new session",
  },
  {
    name: "history",
    description: "View session history",
  },
  {
    name: "help",
    description: "Show help",
  },
  {
    name: "quit",
    description: "Exit Cline",
  },
];

function addEntry(byName: Map<string, SlashCommandRegistryEntry>, entry: SlashCommandRegistryEntry): void {
  const normalized = normalizeCommandName(entry.name);
  if (!normalized || byName.has(normalized)) {
    return;
  }
  byName.set(normalized, {
    ...entry,
    name: normalized,
    description: normalizeCommandDescription(entry.description),
  });
}
