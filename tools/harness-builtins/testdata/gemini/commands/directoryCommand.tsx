// Excerpt of https://github.com/google-gemini/gemini-cli/blob/v0.64.0/packages/cli/src/ui/commands/directoryCommand.tsx (lines 80-92)
}

export const directoryCommand: SlashCommand = {
  name: 'directory',
  altNames: ['dir'],
  description: 'Manage workspace directories',
  kind: CommandKind.BUILT_IN,
  subCommands: [
    {
      name: 'add',
      description:
        'Add directories to the workspace. Use comma to separate multiple paths',
      kind: CommandKind.BUILT_IN,
