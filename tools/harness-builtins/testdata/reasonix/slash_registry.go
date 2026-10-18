// Excerpt of https://github.com/esengine/DeepSeek-Reasonix/blob/v2.43.0/internal/frontend/cli/slash_registry.go
package cli

func builtinSlashSpecs() []builtinSlashSpec {
	return []builtinSlashSpec{
		{name: "/compact", insert: "/compact ", hint: i18n.M.CmdCompact, showInHelp: true},
		{name: "/plugins", aliases: []string{"/plugin"}, insert: "/plugins", hint: i18n.M.CmdPlugins, showInHelp: true},
		{name: "/preset", aliases: []string{"/work-mode", "/profile"}, insert: "/preset ", descend: true},
		// {name: "/gone", insert: "/gone"},
		{name: "/docs", aliases: []string{"/reasonix:docs"}, insert: "/docs ", hint: i18n.M.CmdDocs, showInHelp: true},
		{name: "/quit", aliases: []string{"/exit"}, insert: "/quit", hint: i18n.M.CmdQuit},
	}
}

func builtinSlashItems() []compItem {
	return nil
}
