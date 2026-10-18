// Excerpt of https://gitlab.com/gitlab-org/editor-extensions/gitlab-lsp/-/raw/7af2780a003c/packages/cli/app/src/slash_commands/handlers/custom_command_handler.ts, fetched 2026-10-18
export class Handler {
  command = {
    name: '/custom-command',
    description: 'Run a custom slash command.',
    action: SlashCommandAction.CustomCommand,
    internal: true,
  } as const;
}
