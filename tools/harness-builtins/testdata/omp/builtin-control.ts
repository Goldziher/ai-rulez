// Excerpt of https://github.com/can1357/oh-my-pi/blob/v18.12.0/packages/coding-agent/src/slash-commands/builtin-control.ts
export const BUILTIN_CONTROL_SLASH_COMMANDS: ReadonlyArray<SlashCommandSpec> = [
	{
		name: "force",
		icon: "hammer",
		description: "Force next turn to use a specific tool",
		aliases: ["force:"],
		allowArgs: true,
	},
	{
		name: "live",
		icon: "voice",
	},
	// {
	//	name: "commented-out",
	// },
	{
		name: "todo",
		subcommands: [
			{ name: "edit", description: "Open todos in $EDITOR" },
			{
				name: "append",
				aliases: ["nested-alias"],
			},
		],
	},
	{
		name: "quit",
		aliases: ["q"],
		icon: "power",
	},
];
