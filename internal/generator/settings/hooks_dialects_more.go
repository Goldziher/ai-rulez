package settings

import (
	"github.com/Goldziher/ai-rulez/internal/config"
)

// Pseudo-events and handler fields of the harnesses whose layout is not the
// Claude Code one. Every entry names the vendor page it was read from (read
// 2026-10-05) and what in it is inferred rather than documented.
func moreDialectSpecs() map[string]hookSpec {
	grokEvents := identityEvents([]string{
		eventSessionStart, eventSessionEnd, eventUserPromptSubmit, eventPreToolUse, eventPostToolUse,
		eventPostToolUseFailure, eventPermissionDenied, eventStop, eventStopFailure, eventNotify, eventSubagentStart,
		eventSubagentStop, eventPreCompact, eventPostCompact,
	})
	return map[string]hookSpec{
		// Grok Build CLI: .grok/hooks/<file>.json with a `hooks` wrapper, Claude nesting,
		// seconds; Claude tool names are mapped by Grok itself. Project hooks need
		// /hooks-trust. https://docs.x.ai/build/features/hooks
		config.HarnessGrok: {
			name: config.HarnessGrok, events: grokEvents, nested: true, matcherPassthrough: true, shape: claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPostToolUseFailure),
			scriptVar:     "$GROK_WORKSPACE_ROOT",
			note:          "Grok only runs project hooks in a trusted folder (/hooks-trust or --trust)",
		},
		// IBM Bob: .bob/settings.json `hooks`, five events, matcher (regex over Bob's own
		// tool names) on tool events only, seconds.
		// https://bob.ibm.com/docs/ide/configuration/lifecycle-hooks
		config.HarnessBob: {
			name: config.HarnessBob,
			events: identityEvents([]string{
				eventSessionStart, eventUserPromptSubmit, eventPreToolUse, eventPostToolUse, eventStop,
			}),
			nested: true, shape: claudeShape, matcherEvents: set(eventPreToolUse, eventPostToolUse), scriptCwd: true,
		},
		// Snowflake Cortex Code: .cortex/settings.json `hooks` (user scope: the
		// dedicated ~/.snowflake/cortex/hooks.json, same schema), eleven events, seconds,
		// $CORTEX_PROJECT_DIR. https://docs.snowflake.com/en/user-guide/cortex-code/extensibility
		config.HarnessCortex: {
			name: config.HarnessCortex,
			events: identityEvents([]string{
				eventPreToolUse, eventPostToolUse, eventPermissionRequest, eventUserPromptSubmit, eventSessionStart,
				eventSessionEnd, eventPreCompact, eventStop, eventSubagentStop, eventNotify, "Setup",
			}),
			nested: true, status: true, shape: claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPermissionRequest),
			scriptVar:     "$CORTEX_PROJECT_DIR",
		},
		// Block goose: hooks live in a plugin directory,
		// .agents/plugins/ai-rulez/hooks/hooks.json, `hooks` wrapper, Claude nesting,
		// seconds. Project scripts cannot be addressed from there.
		// https://goose-docs.ai/docs/guides/context-engineering/hooks/
		config.HarnessGoose: {
			name: config.HarnessGoose,
			events: identityEvents([]string{
				eventSessionStart, eventSessionEnd, eventStop, eventUserPromptSubmit, eventPreToolUse, eventPostToolUse,
				eventPostToolUseFailure,
			}),
			nested: true, shape: claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPostToolUseFailure),
		},
		// LangChain Deep Agents Code: .deepagents/hooks.json `hooks` wrapper, Claude
		// nesting and tool names, seconds, exec form as `argv`, no async; project hooks
		// need the user's trust. https://docs.langchain.com/oss/deepagents/code/hooks
		config.HarnessDeepAgents: {
			name: config.HarnessDeepAgents,
			events: identityEvents([]string{
				eventSessionStart, eventUserPromptSubmit, eventSessionEnd, eventPermissionRequest, eventNotify,
				eventPreToolUse, eventPostToolUse, eventPreCompact, eventStop, eventSubagentStart, eventSubagentStop,
			}),
			nested: true, matcherPassthrough: true, args: true, status: true,
			shape:         &handlerShape{typed: true, argsField: "argv"},
			matcherEvents: set(eventSessionStart, eventSessionEnd, eventPermissionRequest, eventPreToolUse, eventPostToolUse, eventNotify, eventPreCompact, eventSubagentStart, eventSubagentStop),
			scriptVar:     "${CLAUDE_PROJECT_DIR}",
			note:          "Deep Agents asks you to trust a project's hooks.json before it runs them",
		},
		// JetBrains Junie CLI: ~/.junie/config.json `hooks`, Claude nesting and tool
		// names, seconds, seven events. Junie ignores the hooks of a project config file.
		// https://junie.jetbrains.com/docs/junie-cli-hooks.html
		config.HarnessJunie: {
			name: config.HarnessJunie,
			events: identityEvents([]string{
				eventSessionStart, eventUserPromptSubmit, eventPreToolUse, eventStop, eventStopFailure,
				eventPermissionRequest, eventSessionEnd,
			}),
			nested: true, matcherPassthrough: true, async: true, shape: claudeShape, userOnly: true,
			matcherEvents: set(eventSessionStart, eventPreToolUse, eventPermissionRequest, eventStopFailure, eventSessionEnd),
		},
		// Z.ai ZCode: ~/.zcode/cli/config.json `hooks.events`, which needs
		// `hooks.enabled`; milliseconds. ZCode ignores project-level hooks.
		// https://zcode.z.ai/en/docs/hooks
		config.HarnessZCode: {
			name: config.HarnessZCode,
			events: identityEvents([]string{
				eventSessionStart, eventUserPromptSubmit, eventPreToolUse, eventPermissionRequest, eventPostToolUse,
				eventPostToolUseFailure, eventStop,
			}),
			nested: true, matcherPassthrough: true, async: true, status: true,
			container: []string{keyHooks, "events"}, userOnly: true,
			shape:         &handlerShape{typed: true, timeoutField: "timeoutMs", timeoutMS: true},
			matcherEvents: set(eventSessionStart, eventPreToolUse, eventPermissionRequest, eventPostToolUse, eventPostToolUseFailure),
			required:      []requiredKey{{path: []string{keyHooks, "enabled"}, value: true}},
		},
		// Charm Crush: crush.json `hooks.PreToolUse`, flat entries, seconds, lowercase
		// tool names. Crush prefers a crushrc script but still reads crush.json.
		// https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md
		config.HarnessCrush: {
			name: config.HarnessCrush, events: identityEvents([]string{eventPreToolUse}),
			shape: &handlerShape{}, matcherEvents: set(eventPreToolUse), scriptVar: "$CRUSH_PROJECT_DIR",
		},
		// Poolside Pool: .poolside/settings.yaml `hooks.<Event>`, flat entries that
		// always carry a matcher, seconds. Pool documents only absolute script paths.
		// https://docs.poolside.ai/hooks
		config.HarnessPoolside: {
			name: config.HarnessPoolside,
			events: identityEvents([]string{
				eventPreToolUse, eventPostToolUse, eventUserPromptSubmit, eventStop, eventPreCompact, eventSessionStart,
			}),
			shape: &handlerShape{}, matcherEvents: set(eventPreToolUse, eventPostToolUse),
			defaultMatcher: "*", matcherRequired: true,
		},
		// Reasonix: .reasonix/settings.json `hooks.<Event>`, flat entries with `match`,
		// milliseconds. https://github.com/esengine/DeepSeek-Reasonix (docs/DESKTOP_HOOKS.zh-CN.md)
		config.HarnessReasonix: {
			name: config.HarnessReasonix,
			events: identityEvents([]string{
				eventPreToolUse, eventPostToolUse, eventPostToolUseFailure, eventPermissionRequest, eventUserPromptSubmit,
				eventStop, eventStopFailure, eventSessionStart, eventSessionEnd, eventSubagentStop, eventNotify, eventPreCompact,
			}),
			shape:         &handlerShape{timeoutMS: true, matcherField: "match"},
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPostToolUseFailure, eventPermissionRequest),
		},
		// Hermes Agent: ~/.hermes/config.yaml `hooks.<event>`, flat entries, seconds,
		// five events with Hermes names. Hermes documents no project-level hooks.
		// https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks
		config.HarnessHermes: {
			name: config.HarnessHermes,
			events: events(
				eventSessionStart, "on_session_start", eventSessionEnd, "on_session_end", eventPreToolUse, "pre_tool_call",
				eventPostToolUse, "post_tool_call", eventSubagentStop, "subagent_stop",
			),
			shape: &handlerShape{}, userOnly: true, matcherEvents: set("pre_tool_call", "post_tool_call"),
		},
		// Kiro (IDE and CLI v3): .kiro/hooks/ai-rulez.json, {version: "v1", hooks: [...]},
		// each entry carrying its trigger; seconds; lowercase tool categories.
		// https://kiro.dev/docs/hooks/ (checked 2026-10-05: a v1 document uses PascalCase
		// triggers, and CLI v3's canonical SessionStart accepts the 2.x camelCase spelling
		// as an alias; PreToolUse, PostToolUse and Stop are listed as written here, while
		// UserPromptSubmit is inferred from the v3 naming, the docs name promptSubmit for 2.x).
		config.HarnessKiro: {
			name: config.HarnessKiro,
			events: identityEvents([]string{
				eventSessionStart, eventUserPromptSubmit, eventPreToolUse, eventPostToolUse, eventStop,
			}),
			flat: kiroEntry, matcherEvents: set(eventPreToolUse, eventPostToolUse), scriptCwd: true,
			required: []requiredKey{{path: []string{keyVersion}, value: "v1"}},
		},
		// Mistral Vibe: .vibe/hooks.toml, a flat [[hooks]] list with the event in `type`
		// and the tool glob in `match`, seconds. Stop maps to post_agent (inferred).
		// https://docs.mistral.ai/vibe/code/cli/hooks
		config.HarnessVibe: {
			name:   config.HarnessVibe,
			events: events(eventPreToolUse, "pre_tool", eventPostToolUse, "post_tool", eventStop, "post_agent"),
			flat:   vibeEntry, matcherEvents: set("pre_tool", "post_tool"),
			defaultMatcher: "*", scriptCwd: true,
		},
		// Kimi Code: ~/.kimi-code/config.toml, a flat [[hooks]] list with `event`,
		// seconds. Kimi documents no project-level hooks.
		// https://moonshotai.github.io/kimi-code/en/customization/hooks.html
		config.HarnessKimi: {
			name: config.HarnessKimi,
			events: identityEvents([]string{
				eventSessionStart, eventSessionEnd, eventUserPromptSubmit, eventPreToolUse, eventPostToolUse,
				eventPostToolUseFailure, eventPermissionRequest, eventStop, eventStopFailure, eventNotify,
				eventSubagentStart, eventSubagentStop, eventPreCompact, eventPostCompact,
			}),
			flat: kimiEntry, userOnly: true,
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPostToolUseFailure, eventPermissionRequest),
		},
	}
}

func kiroEntry(hc handlerContext, command string, action *config.HookAction) ordered {
	var o ordered
	o.set("name", hc.name)
	o.set("trigger", hc.event)
	if hc.matcher != "" {
		o.set("matcher", hc.matcher)
	}
	o.set("action", ordered{{"type", config.HookTypeCommand}, {"command", command}})
	if action.Timeout > 0 {
		o.set("timeout", action.Timeout)
	}
	return o
}

func vibeEntry(hc handlerContext, command string, action *config.HookAction) ordered {
	var o ordered
	o.set("name", hc.name)
	o.set("type", hc.event)
	if hc.matcher != "" {
		o.set("match", hc.matcher)
	}
	o.set("command", command)
	if action.Timeout > 0 {
		o.set("timeout", action.Timeout)
	}
	return o
}

func kimiEntry(hc handlerContext, command string, action *config.HookAction) ordered {
	var o ordered
	o.set("event", hc.event)
	if hc.matcher != "" {
		o.set("matcher", hc.matcher)
	}
	o.set("command", command)
	if action.Timeout > 0 {
		o.set("timeout", action.Timeout)
	}
	return o
}
