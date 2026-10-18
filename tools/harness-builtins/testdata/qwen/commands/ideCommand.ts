// Excerpt of https://github.com/QwenLM/qwen-code/blob/v0.25.2/packages/cli/src/ui/commands/ideCommand.ts (lines 143-149)
export const ideCommand = async (): Promise<SlashCommand> => {
  const ideClient = await IdeClient.getInstance();
  const currentIDE = ideClient.getCurrentIde();
  if (!currentIDE) {
    return {
      name: 'ide',
      get description() {
