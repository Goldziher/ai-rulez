// Excerpt of https://github.com/google-gemini/gemini-cli/blob/v0.64.0/packages/cli/src/ui/commands/extensionsCommand.ts (lines 836-846, 876-917)
  action: linkAction,
};

const uninstallCommand: SlashCommand = {
  name: 'uninstall',
  altNames: ['delete'],
  description: 'Uninstall an extension',
  kind: CommandKind.BUILT_IN,
  autoExecute: false,
  action: uninstallAction,
  completion: completeExtensions,
// ...
import { parseSlashCommand } from '../../utils/commands.js';

export function extensionsCommand(
  enableExtensionReloading?: boolean,
): SlashCommand {
  const conditionalCommands = enableExtensionReloading
    ? [
        disableCommand,
        enableCommand,
        installCommand,
        uninstallCommand,
        linkCommand,
        configCommand,
      ]
    : [];
  const subCommands = [
    listExtensionsCommand,
    updateExtensionsCommand,
    exploreExtensionsCommand,
    reloadCommand,
    ...conditionalCommands,
  ];

  return {
    name: 'extensions',
    description: 'Manage extensions',
    kind: CommandKind.BUILT_IN,
    autoExecute: false,
    subCommands,
    action: async (context, args) => {
      if (args) {
        const parsed = parseSlashCommand(`/${args}`, subCommands);
        if (parsed.commandToExecute?.action) {
          return parsed.commandToExecute.action(context, parsed.args);
        }
      }
      // Default to list if no subcommand is provided
      return listExtensionsCommand.action!(context, args);
    },
  };
}
