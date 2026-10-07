// Package settings renders the top-level [[hooks]], [permissions] and
// [claude.settings.managed] blocks into the native settings documents of each
// harness (.claude/settings.json, .codex/hooks.json, .cursor/hooks.json,
// .gemini/settings.json, .github/hooks/ai-rulez.json).
// The five harnesses above have a handler struct each; the others (Factory, Qwen,
// Kiro, Vibe, Poolside, ... see hooks_dialects.go and hooks_dialects_more.go) are
// rendered from a table of vendor facts, and Cline through wrapper scripts
// (hooks_cline.go).
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
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Document paths, relative to the base directory of the scope being rendered.
const (
	ClaudeSettingsPath = ".claude/settings.json"
	CodexHooksPath     = ".codex/hooks.json"
	CursorHooksPath    = ".cursor/hooks.json"
	GeminiSettingsPath = ".gemini/settings.json"
	CopilotHooksPath   = ".github/hooks/ai-rulez.json"

	// AntigravityHooksPath and DevinHooksPath are the project hooks files of the
	// antigravity and devin presets.
	AntigravityHooksPath = ".agents/hooks.json"
	DevinHooksPath       = ".devin/hooks.v1.json"

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
	// args, async and condition record which handler fields the harness has.
	args, async, condition bool
	// status is true when the harness has a handler field for a progress message.
	status bool

	// The fields below describe a harness rendered through the generic handler
	// shape (hooks_dialects.go); the five harnesses above keep their own structs.

	// shape names the handler fields; nil for the harnesses with their own struct.
	shape *handlerShape
	// container is the key path of the object holding the event arrays: ["hooks"]
	// when empty and rootKeyed is false.
	container []string
	// rootKeyed is true when the events are keyed at the document root.
	rootKeyed bool
	// userContainer / userRootKeyed override the container in user scope, for a
	// harness whose user-level file is a different document (Devin's config.json).
	userContainer []string
	// matcherEvents lists the native events that honour a matcher; nil means all.
	matcherEvents map[string]bool
	// scriptVar is the environment variable naming the project root in a command
	// ("$FACTORY_PROJECT_DIR"); empty when the harness documents none.
	scriptVar string
	// scriptCwd is true when hooks run from the project root, so a project script
	// is addressed as ./path. A harness with neither cannot address a project script.
	scriptCwd bool
	// flat builds the entry of a harness whose single list carries the event in
	// each entry (Kiro, Vibe); the entry is named, since both require a name.
	flat func(hc handlerContext, command string, action *config.HookAction) ordered
	// required are the scalars the document needs to be valid.
	required []requiredKey
	// userOnly is true when the harness ignores project-level hooks.
	userOnly bool
	// defaultMatcher is written for a group without one where the harness requires
	// the field: on every event when matcherRequired, else on matcherEvents.
	defaultMatcher  string
	matcherRequired bool
	// ownedFile is true for a hooks file ai-rulez writes whole (see OwnedHooksDocument).
	ownedFile bool
	// note is advice printed once per run when hooks are generated for the harness.
	note string
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
		// Cursor runs project hooks from the project root.
		scriptCwd: true,
		required:  []requiredKey{{path: []string{keyVersion}, value: cursorHooksVersion}},
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
		// The matcher is an optional regex over toolName on preToolUse and postToolUse.
		matcherEvents: set("preToolUse", "postToolUse"),
		ownedFile:     true,
	}
	// Copilot CLI loads the same .github/hooks/*.json files as Copilot (read
	// 2026-10-05 from docs.github.com/en/copilot/reference/hooks-configuration), so
	// it renders the same document under its own name.
	copilotCLISpec = hookSpec{
		name: config.HarnessCopilotCLI, events: copilotSpec.events, matcherEvents: copilotSpec.matcherEvents, ownedFile: true,
	}
)

// requiredKey is a scalar a hooks document needs: Cursor's `version`, ZCode's
// `hooks.enabled`.
type requiredKey struct {
	path  []string
	value any
	// userOnly restricts the key to user scope.
	userOnly bool
}

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
	case config.HarnessCopilotCLI:
		return copilotCLISpec, true
	}
	spec, ok := dialectSpecs()[harness]
	return spec, ok
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
