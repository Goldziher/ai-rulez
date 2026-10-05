package toolnames

// Every vocabulary below was read from the vendor page it cites on 2026-10-05.
// A Claude tool that a vendor does not document is left out on purpose: a hook
// matcher naming it is skipped for that harness rather than guessed.
func init() {
	// OpenCode and Kilo: https://opencode.ai/docs/tools/. Kilo shares the table.
	register(&Vocabulary{
		Harness: "opencode", Source: "https://opencode.ai/docs/tools/",
		Tools: map[string][]string{
			Bash: {nativeBash}, Read: {nativeRead}, Edit: {nativeEdit, nativeApplyPatch}, MultiEdit: {nativeEdit, nativeApplyPatch},
			Write: {nativeWrite, nativeApplyPatch}, Grep: {nativeGrep}, Glob: {nativeGlob}, TodoWrite: {"todowrite"},
			WebFetch: {"webfetch"}, WebSearch: {"websearch"}, Task: {nativeTask}, Agent: {nativeTask}, Skill: {"skill"},
		},
	})
	// Pi: built-in tools of the extension API (isToolCallEventType).
	register(&Vocabulary{
		Harness: "pi", Source: "Pi extension types, isToolCallEventType",
		Tools: map[string][]string{
			Bash: {nativeBash}, Read: {nativeRead}, Edit: {nativeEdit}, MultiEdit: {nativeEdit}, Write: {nativeWrite},
			Grep: {nativeGrep}, Glob: {"find"}, LS: {"ls"},
		},
	})
	// Amp's own names beyond the ones it shares with Claude Code (Bash, Read, Grep, Task).
	register(&Vocabulary{
		Harness: "amp", Source: "amp tools list",
		Tools: map[string][]string{
			Edit: {"edit_file"}, MultiEdit: {"edit_file"}, Write: {"create_file"}, Glob: {nativeGlob},
			WebSearch: {"web_search"}, WebFetch: {"read_web_page"}, TodoWrite: {"todo_write"},
		},
	})

	// Cursor preToolUse/postToolUse: the matcher is a regex over the tool type
	// (Shell, Read, Write, Grep, Delete, Task) and file edits are Write operations,
	// so Edit, MultiEdit and Write all select Write: Cursor does not tell creating
	// a file from editing one. MCP tools are `MCP:<tool_name>` without the server,
	// which a Claude `mcp__server__tool` cannot be rewritten into.
	// https://cursor.com/docs/hooks
	register(&Vocabulary{
		Harness: "cursor", Source: "https://cursor.com/docs/hooks (read 2026-10-05)", Search: true, MatchAll: "*",
		Tools: map[string][]string{
			Bash: {"Shell"}, Read: {"Read"}, Edit: {Write}, MultiEdit: {Write}, Write: {Write},
			Grep: {"Grep"}, Task: {Task}, Agent: {Task},
		},
	})
	// Gemini CLI BeforeTool/AfterTool: a regex over the tool name; MCP tools are
	// mcp_<server>_<tool>. https://geminicli.com/docs/hooks/reference/ and
	// https://geminicli.com/docs/reference/tools/
	register(&Vocabulary{
		Harness: "gemini", Source: "https://geminicli.com/docs/reference/tools/ (read 2026-10-05)",
		Search: true, MatchAll: ".*", MCP: "mcp_{server}_{tool}",
		Tools: map[string][]string{
			Bash: {"run_shell_command"}, Read: {"read_file", "read_many_files"}, Edit: {"replace"},
			MultiEdit: {"replace"}, Write: {"write_file"}, Grep: {"grep_search"}, Glob: {nativeGlob},
			LS: {"list_directory"}, WebFetch: {nativeWebFetch}, WebSearch: {"google_web_search"},
			TodoWrite: {"write_todos"},
		},
	})
	// GitHub Copilot and Copilot CLI preToolUse/postToolUse: an optional `matcher`
	// compiled as ^(?:PATTERN)$ against toolName. The page documents no MCP naming
	// and no ls, web search or notebook tool. Bash is `bash` only: the generated
	// entry sets the `bash` command, not the `powershell` one.
	// https://docs.github.com/en/copilot/reference/hooks-configuration
	for _, harness := range []string{"copilot", "copilot-cli"} {
		register(&Vocabulary{
			Harness: harness, Source: "https://docs.github.com/en/copilot/reference/hooks-configuration (read 2026-10-05)",
			Tools: map[string][]string{
				Bash: {nativeBash}, Read: {nativeView}, Edit: {nativeEdit}, MultiEdit: {nativeEdit}, Write: {"create"},
				Grep: {nativeGrep}, Glob: {nativeGlob}, WebFetch: {nativeWebFetch}, Task: {nativeTask}, Agent: {nativeTask},
			},
		})
	}
	// Factory Droid: a case-sensitive regex over the tool name; MCP tools are
	// mcp__<server>__<tool> like Claude Code. ApplyPatch is not mapped: the page
	// does not say it is what Claude's Edit or Write select.
	// https://docs.factory.com/reference/hooks-reference
	register(&Vocabulary{
		Harness: "factory", Source: "https://docs.factory.com/reference/hooks-reference (read 2026-10-05)",
		Search: true, MatchAll: "*", MCP: nativeMCPPattern,
		Tools: map[string][]string{
			Bash: {"Execute"}, Read: {"Read"}, Edit: {"Edit"}, Write: {"Create"}, Grep: {"Grep"}, Glob: {"Glob"},
			LS: {"LS"}, WebFetch: {"FetchUrl"}, WebSearch: {"WebSearch"}, Task: {Task}, Agent: {Task},
		},
	})
	// Devin CLI: a regex over tool_name, documented with anchored patterns; MCP
	// tools are mcp__<server>__<tool>. Only the core tools below are documented.
	// https://docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks
	register(&Vocabulary{
		Harness: "devin", Source: "https://docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks (read 2026-10-05)",
		Search: true, MCP: nativeMCPPattern,
		Tools: map[string][]string{
			Bash: {"exec"}, Read: {nativeRead}, Edit: {nativeEdit}, Write: {nativeWrite}, Grep: {nativeGrep}, Glob: {nativeGlob},
			WebFetch: {"webfetch"},
		},
	})
	// Augment Auggie CLI: a case-sensitive regex over the tool name. MCP tools
	// (`mcp:` prefix, <tool>_<server>) are not rewritten.
	// https://docs.augmentcode.com/cli/hooks
	register(&Vocabulary{
		Harness: "augment", Source: "https://docs.augmentcode.com/cli/hooks (read 2026-10-05)", Search: true,
		Tools: map[string][]string{
			Bash: {"launch-process"}, Read: {nativeView}, Edit: {"str-replace-editor"}, MultiEdit: {"str-replace-editor"},
			Write: {"save-file"}, WebFetch: {"web-fetch"}, WebSearch: {"web-search"},
		},
	})
	// Google Antigravity: a regex over the tool name. The page lists the tools
	// by category; no MCP naming is documented.
	// https://antigravity.google/docs/hooks
	register(&Vocabulary{
		Harness: "antigravity", Source: "https://antigravity.google/docs/hooks (read 2026-10-05)", Search: true,
		Tools: map[string][]string{
			Bash: {"run_command"}, Read: {"view_file"}, Edit: {"replace_file_content"},
			MultiEdit: {"multi_replace_file_content"}, Write: {"write_to_file"}, Grep: {"grep_search"},
			Glob: {"find_by_name"}, LS: {"list_dir"}, WebFetch: {"read_url_content"}, WebSearch: {"search_web"},
			Task: {"invoke_subagent"}, Agent: {"invoke_subagent"},
		},
	})

	// Kiro: the matcher is a category (read, write, shell, web, spec, *), an
	// @-prefix or an exact tool name. Only `shell` is exactly Claude's Bash: the
	// read, write and web categories each span several tools Claude names apart
	// (read covers grep and ls too), so those are left unmapped.
	// https://kiro.dev/docs/hooks/types/
	register(&Vocabulary{
		Harness: "kiro", Source: "https://kiro.dev/docs/hooks/types/ (read 2026-10-05)", NoAlternation: true,
		MatchAll: "*", Tools: map[string][]string{Bash: {nativeShell}},
	})
	// Block goose: the matcher is a regular expression (`.*`, never `*`); the
	// developer extension's tools are unprefixed and every other extension's are
	// {extension}__{tool}. https://goose-docs.ai/docs/guides/context-engineering/hooks/
	register(&Vocabulary{
		Harness: "goose", Source: "https://goose-docs.ai/docs/guides/context-engineering/hooks/ (read 2026-10-05)",
		Search: true, MatchAll: ".*", MCP: "{server}__{tool}",
		Tools: map[string][]string{Bash: {nativeShell}, Write: {nativeWrite}, Edit: {nativeEdit}, MultiEdit: {nativeEdit}},
	})
	// Charm Crush: a regex over lowercase tool names; MCP tools are
	// mcp_<server>_<tool>. https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md
	register(&Vocabulary{
		Harness: "crush", Source: "https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md (read 2026-10-05)",
		Search: true, MCP: "mcp_{server}_{tool}",
		Tools: map[string][]string{
			Bash: {nativeBash}, Read: {nativeView}, Edit: {nativeEdit}, Write: {nativeWrite}, MultiEdit: {"multiedit"},
			Grep: {nativeGrep}, Glob: {nativeGlob}, LS: {"ls"}, Task: {"agent"}, Agent: {"agent"},
		},
	})
	// Snowflake Cortex Code CLI: a case-sensitive regex over lowercase runtime
	// tool names; MCP tools are mcp__<server>__<tool>. NotebookEdit is the edit
	// tool only, not notebook_run_cell.
	// https://docs.snowflake.com/en/user-guide/cortex-code/extensibility
	register(&Vocabulary{
		Harness: "cortex", Source: "https://docs.snowflake.com/en/user-guide/cortex-code/extensibility (read 2026-10-05)",
		Search: true, MatchAll: "*", MCP: nativeMCPPattern,
		Tools: map[string][]string{
			Bash: {nativeBash}, Read: {nativeRead}, Edit: {nativeEdit}, Write: {nativeWrite}, Grep: {nativeGrep}, Glob: {nativeGlob},
			NotebookEdit: {"notebook_edit_cell"},
		},
	})
	// Poolside Pool: "" or "*" is any tool, a bare name or a pipe list is exact,
	// anything else a regular expression. The page documents no MCP naming.
	// https://docs.poolside.ai/hooks
	register(&Vocabulary{
		Harness: "poolside", Source: "https://docs.poolside.ai/hooks (read 2026-10-05)", MatchAll: "*",
		Tools: map[string][]string{
			Bash: {nativeShell}, Read: {nativeRead}, Edit: {nativeEdit}, MultiEdit: {nativeEdit}, Write: {nativeWrite},
			WebFetch: {nativeWebFetch}, WebSearch: {"web_search"},
		},
	})
	// Mistral Vibe: `match` is an fnmatch glob (or a regex behind `re:`), so no
	// alternation is written. The hooks page names only `bash`; grep is the name
	// its configuration page gives for the search tool.
	// https://docs.mistral.ai/vibe/code/cli/hooks and /configuration
	register(&Vocabulary{
		Harness: "vibe", Source: "https://docs.mistral.ai/vibe/code/cli/hooks (read 2026-10-05)",
		NoAlternation: true, MatchAll: "*", Glob: true,
		Tools: map[string][]string{Bash: {nativeBash}, Grep: {nativeGrep}},
	})
}
