// Excerpt of https://github.com/CodebuffAI/codebuff/blob/8d0e7b59e038e89d142c799f96ff8c906566c588/cli/src/data/slash-commands.ts
const MODE_COMMANDS: SlashCommand[] = IS_FREEBUFF
  ? []
  : AGENT_MODES.map((mode) => ({
      id: `mode:${mode.toLowerCase()}`,
      label: `mode:${mode.toLowerCase()}`,
      description: `Switch to ${mode} mode`,
      aliases: [`model:${mode.toLowerCase()}`],
    }))

const ALL_SLASH_COMMANDS: SlashCommand[] = [
  {
    id: 'help',
    label: 'help',
    description: 'Display keyboard shortcuts and tips',
    aliases: ['h', '?'],
    implicitCommand: true,
  },
  // {
  //   id: 'undo',
  //   label: 'undo',
  // },
  {
    id: 'plan',
    label: 'plan',
    description: 'Create a plan for how to implement a request',
  },
  ...MODE_COMMANDS,
  {
    id: 'exit',
    label: 'exit',
    aliases: ['quit', 'q'],
  },
]
