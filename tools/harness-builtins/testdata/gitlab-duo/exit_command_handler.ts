// Excerpt of https://gitlab.com/gitlab-org/editor-extensions/gitlab-lsp/-/raw/7af2780a003c/packages/cli/app/src/slash_commands/handlers/exit_command_handler.ts, fetched 2026-10-18
/**
 * Handler for the /exit slash command.
 * Triggers a graceful shutdown of the CLI application.
 */
export class Handler {
  command = {
    name: '/exit',
    description: 'Exit the application',
    action: SlashCommandAction.Exit,
    aliases: ['/quit'],
  } as const;
}
