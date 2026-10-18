// Excerpt of https://github.com/coder/mux/blob/335c4ce4a79c07274940b42ed0cfccda586e808a/src/browser/utils/slashCommands/registry.ts
const clearCommandDefinition: SlashCommandDefinition = {
  key: "clear",
  description: "Clear conversation history",
};

const planOpenCommandDefinition: SlashCommandDefinition = {
  key: "open",
  description: "Open plan in external editor",
};

const planCommandDefinition: SlashCommandDefinition = {
  key: "plan",
  description: "Show or edit the current plan",
  children: [planOpenCommandDefinition],
};

const speedModeCommandDefinitions: SlashCommandDefinition[] = (["fast", "ultrafast"] as const).map((mode) => ({
  key: mode,
}));

export const SLASH_COMMAND_DEFINITIONS: readonly SlashCommandDefinition[] = [
  clearCommandDefinition,
  planCommandDefinition,

  // vimCommandDefinition,
  ...speedModeCommandDefinitions,
];
