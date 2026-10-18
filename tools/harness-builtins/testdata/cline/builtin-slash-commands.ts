// Excerpt of https://github.com/cline/cline/blob/v4.1.25/apps/vscode/src/sdk/builtin-slash-commands.ts (prompt body elided)
/**
 * Built-in slash commands that expand into a prompt on the SDK runtime.
 *
 * These are appended to the workflow/skill commands the user-instruction
 * service discovers on disk, so the existing expandSlashCommands machinery
 * handles the token matching and replacement.
 *
 * `/deep-planning` is a trimmed port of the legacy extension's prompt
 * (core/prompts/commands/deep-planning): the shell-specific research-command
 * examples are dropped, and legacy's final step — which created an
 * implementation task via the `new_task` tool, unavailable on the SDK
 * runtime — instead has the agent present the plan and wait for explicit
 * user confirmation.
 */

import type { AvailableRuntimeCommand } from "@cline/core"

// ... prompt elided ...

/**
 * Appended after the discovered workflow/skill commands in
 * SdkController.resolveSlashCommands. Declared as kind "skill" so the
 * workflow enable/disable toggles never apply to them.
 */
export const BUILTIN_SLASH_COMMANDS: AvailableRuntimeCommand[] = [
  {
    id: "builtin:deep-planning",
    name: "deep-planning",
    instructions: DEEP_PLANNING_INSTRUCTIONS,
    kind: "skill",
  },
]
