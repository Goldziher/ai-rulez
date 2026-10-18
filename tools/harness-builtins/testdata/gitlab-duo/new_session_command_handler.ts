// Excerpt of https://gitlab.com/gitlab-org/editor-extensions/gitlab-lsp/-/raw/7af2780a003c/packages/cli/app/src/slash_commands/handlers/new_session_command_handler.ts, fetched 2026-10-18
/**
 * Handler for the /new slash command.
 * Creates a new chat session and clears the chat history.
 */
export class Handler {
  command = {
    name: '/new',
    description: 'Start a new chat session',
    action: 'new_session',
    aliases: ['/clear'],
  } as const;
}
