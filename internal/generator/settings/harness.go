// Package settings renders the top-level [[hooks]], [permissions] and
// [claude.settings.managed] blocks into the native settings documents of each
// harness (.claude/settings.json, .codex/hooks.json, .cursor/hooks.json,
// .gemini/settings.json, .github/hooks/ai-rulez.json).
//
// The documents are shared with the consumer, so the package returns
// jsonmerge.OwnedKey values rather than finished files: array entries are owned
// one by one (jsonmerge.OwnedKey.Elements) and map entries member by member, and
// everything the consumer hand-authored in the same file survives generate and
// clean. Event names, handler fields and timeout units differ per harness; the
// tables in this file were read from each vendor's hooks documentation (see
// docs/settings.md for the sources and dates). A declaration a harness cannot
// express is skipped with a warning, never approximated.
package settings

import (
	"github.com/Goldziher/ai-rulez/internal/config"
)

// Document paths, relative to the base directory of the scope being rendered.
const (
	ClaudeSettingsPath = ".claude/settings.json"
	CodexHooksPath     = ".codex/hooks.json"
	CursorHooksPath    = ".cursor/hooks.json"
	GeminiSettingsPath = ".gemini/settings.json"
	CopilotHooksPath   = ".github/hooks/ai-rulez.json"

	// UserCopilotHooksPath is where Copilot CLI loads user-level hook files.
	UserCopilotHooksPath = ".copilot/hooks/ai-rulez.json"
)

// Claude Code event names used by more than one harness table.
const (
	eventPreToolUse         = "PreToolUse"
	eventPostToolUse        = "PostToolUse"
	eventPostToolUseFailure = "PostToolUseFailure"
	eventNotify             = "Notification"
	eventSessionStart       = "SessionStart"
	eventSessionEnd         = "SessionEnd"
	eventSubagentStart      = "SubagentStart"
	eventSubagentStop       = "SubagentStop"
	eventPreCompact         = "PreCompact"
	eventPostCompact        = "PostCompact"
	eventPermissionRequest  = "PermissionRequest"
	eventUserPromptSubmit   = "UserPromptSubmit"
	eventStop               = "Stop"

	keyVersion = "version"
)

// cursorHooksVersion is the required `version` of Cursor's hooks.json.
const cursorHooksVersion = 1

// copilotHooksVersion is the required `version` of a Copilot hooks file.
const copilotHooksVersion = 1

// hookSpec describes how one harness expresses a hook.
type hookSpec struct {
	name string
	// events maps a Claude Code event name to the harness's own name.
	events map[string]string
	// nested is true when a harness groups handlers under a matcher
	// (event -> [{matcher, hooks: [...]}]); false when every handler is a flat
	// entry of the event's array.
	nested bool
	// matcherPassthrough is true when the harness's matcher vocabulary matches
	// Claude Code's (same tool names, same lifecycle strings), so a group's
	// matcher is copied. Elsewhere a matcher needs a per-harness override.
	matcherPassthrough bool
	// matcherless is true when the harness has no matcher at all.
	matcherless bool
	// args, async and condition record which handler fields the harness has.
	args, async, condition bool
}

var (
	claudeSpec = hookSpec{
		name:               config.HarnessClaude,
		events:             identityEvents(config.KnownHookEvents),
		nested:             true,
		matcherPassthrough: true,
		args:               true, async: true, condition: true,
	}
	// Codex uses Claude Code's event names and tool names (apply_patch is also
	// matched by Edit and Write) and has no exec form or `if`.
	codexSpec = hookSpec{
		name: config.HarnessCodex,
		events: identityEvents([]string{
			eventSessionStart, eventSessionEnd, eventSubagentStart, eventSubagentStop, eventPreToolUse,
			eventPermissionRequest, eventPostToolUse, eventPreCompact, eventPostCompact, eventUserPromptSubmit, eventStop,
		}),
		nested:             true,
		matcherPassthrough: true,
		async:              true,
	}
	// Gemini names events after the agent loop. UserPromptSubmit and Stop map to
	// the nearest events (BeforeAgent fires after a prompt is submitted, AfterAgent
	// once per turn after the final response).
	geminiSpec = hookSpec{
		name: config.HarnessGemini,
		events: map[string]string{
			eventSessionStart: eventSessionStart, eventSessionEnd: eventSessionEnd,
			eventPreToolUse: "BeforeTool", eventPostToolUse: "AfterTool",
			eventNotify: eventNotify, eventPreCompact: "PreCompress",
			eventUserPromptSubmit: "BeforeAgent", eventStop: "AfterAgent",
		},
		nested: true,
	}
	cursorSpec = hookSpec{
		name: config.HarnessCursor,
		events: map[string]string{
			eventSessionStart: "sessionStart", eventSessionEnd: "sessionEnd",
			eventPreToolUse: "preToolUse", eventPostToolUse: "postToolUse", eventPostToolUseFailure: "postToolUseFailure",
			eventSubagentStart: "subagentStart", eventSubagentStop: "subagentStop",
			eventPreCompact: "preCompact", eventStop: "stop", eventUserPromptSubmit: "beforeSubmitPrompt",
		},
	}
	copilotSpec = hookSpec{
		name: config.HarnessCopilot,
		events: map[string]string{
			eventSessionStart: "sessionStart", eventSessionEnd: "sessionEnd",
			eventPreToolUse: "preToolUse", eventPostToolUse: "postToolUse", eventPostToolUseFailure: "postToolUseFailure",
			eventSubagentStart: "subagentStart", eventSubagentStop: "subagentStop",
			eventPreCompact: "preCompact", eventStop: "agentStop", eventUserPromptSubmit: "userPromptSubmitted",
			eventNotify: "notification", eventPermissionRequest: "permissionRequest",
		},
		matcherless: true,
	}
)

func identityEvents(names []string) map[string]string {
	m := make(map[string]string, len(names))
	for _, n := range names {
		m[n] = n
	}
	return m
}

func specFor(harness string) (hookSpec, bool) {
	switch harness {
	case config.HarnessClaude:
		return claudeSpec, true
	case config.HarnessCodex:
		return codexSpec, true
	case config.HarnessGemini:
		return geminiSpec, true
	case config.HarnessCursor:
		return cursorSpec, true
	case config.HarnessCopilot:
		return copilotSpec, true
	}
	return hookSpec{}, false
}

// SupportedEvents returns the Claude Code event names a harness can express, in
// KnownHookEvents order. Used by the documentation table and the tests.
func SupportedEvents(harness string) []string {
	spec, ok := specFor(harness)
	if !ok {
		return nil
	}
	var out []string
	for _, event := range config.KnownHookEvents {
		if _, ok := spec.events[event]; ok {
			out = append(out, event)
		}
	}
	return out
}

// NativeEvent returns the harness's name for a Claude Code event.
func NativeEvent(harness, event string) (string, bool) {
	spec, ok := specFor(harness)
	if !ok {
		return "", false
	}
	native, ok := spec.events[event]
	return native, ok
}
