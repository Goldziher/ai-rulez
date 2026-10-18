// Excerpt of https://gitlab.com/gitlab-org/editor-extensions/gitlab-lsp/-/raw/7af2780a003c/packages/cli/app/src/slash_commands/handlers/doctor_command_handler.ts, fetched 2026-10-18
export class Handler {
  command = {
    name: '/doctor',
    description: 'Show diagnostics for the GitLab Duo CLI environment',
    action: 'doctor',
  } as const;
}
