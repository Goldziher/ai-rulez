package settings

import (
	"encoding/json"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// handlerShape names the handler fields of a harness rendered through the
// generic path. Every harness in dialectSpecs was read from its vendor's hooks
// documentation; the source and date of each are in docs/settings.md and the
// comment above its entry.
type handlerShape struct {
	// typed writes "type": "command" first.
	typed bool
	// commandField is the field holding the command line ("command" unless the
	// harness names it otherwise).
	commandField string
	// timeoutField is the timeout field; timeoutMS says its unit is milliseconds
	// (the configuration is in seconds).
	timeoutField string
	timeoutMS    bool
	// matcherField names the matcher of a flat entry ("matcher" unless the harness
	// says "match"); argsField the exec-form argument list ("args" unless "argv").
	matcherField string
	argsField    string
}

// ordered is a JSON object that keeps its fields in insertion order, so the
// generated hooks read the way the vendor documents them.
type ordered []struct {
	key   string
	value any
}

func (o *ordered) set(key string, value any) {
	*o = append(*o, struct {
		key   string
		value any
	}{key, value})
}

// MarshalJSON writes the fields in insertion order.
func (o ordered) MarshalJSON() ([]byte, error) {
	out := []byte{'{'}
	for i, field := range o {
		if i > 0 {
			out = append(out, ',')
		}
		key, err := json.Marshal(field.key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(field.value)
		if err != nil {
			return nil, err
		}
		out = append(append(append(out, key...), ':'), value...)
	}
	return append(out, '}'), nil
}

// handler builds the handler object of an action.
func (s *handlerShape) handler(spec hookSpec, matcher, command string, args []string, action *config.HookAction) ordered {
	var o ordered
	if s.typed {
		o.set("type", config.HookTypeCommand)
	}
	if matcher != "" && !spec.nested {
		o.set(fieldOr(s.matcherField, "matcher"), matcher)
	}
	o.set(fieldOr(s.commandField, "command"), command)
	if len(args) > 0 {
		o.set(fieldOr(s.argsField, "args"), args)
	}
	if action.Timeout > 0 {
		timeout := action.Timeout
		if s.timeoutMS {
			timeout *= 1000
		}
		o.set(fieldOr(s.timeoutField, "timeout"), timeout)
	}
	if action.Async && spec.async {
		o.set("async", true)
	}
	if action.If != "" && spec.condition {
		o.set("if", action.If)
	}
	if action.StatusMessage != "" && spec.status {
		o.set("statusMessage", action.StatusMessage)
	}
	return o
}

// fieldOr returns the vendor's field name, or the common one when the shape does
// not rename it.
func fieldOr(name, common string) string {
	if name == "" {
		return common
	}
	return name
}

// containerPath is the key path of the object holding the event arrays.
func (s hookSpec) containerPath(cfg *config.Config) []string {
	switch {
	case cfg != nil && cfg.UserScope && s.userContainer != nil:
		return s.userContainer
	case s.rootKeyed:
		return nil
	case s.container != nil:
		return s.container
	}
	return []string{keyHooks}
}

var claudeShape = &handlerShape{typed: true}

func events(pairs ...string) map[string]string {
	m := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return m
}

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func allBut(all map[string]string, excluded ...string) map[string]bool {
	m := map[string]bool{}
	for _, native := range all {
		m[native] = true
	}
	for _, n := range excluded {
		delete(m, n)
	}
	return m
}

var qwenEvents = identityEvents([]string{
	eventSessionStart, eventSessionEnd, eventUserPromptSubmit, "UserPromptExpansion", eventPreToolUse,
	eventPostToolUse, eventPostToolUseFailure, "PostToolBatch", eventPermissionRequest, eventPermissionDenied,
	eventNotify, "MessageDisplay", eventSubagentStart, eventSubagentStop, eventStop, eventStopFailure,
	eventPreCompact, eventPostCompact, eventInstructionsLoaded,
})

var codebuddyEvents = identityEvents([]string{
	eventPreToolUse, eventPostToolUse, eventPostToolUseFailure, eventSessionStart, eventSessionEnd, eventStop,
	eventSubagentStart, eventSubagentStop, eventStopFailure, eventUserPromptSubmit, eventNotify, eventPermissionRequest,
	eventPermissionDenied, "Elicitation", "ElicitationResult", eventPreCompact, eventPostCompact, eventInstructionsLoaded,
	"ConfigChange", "TaskCreated", "TaskCompleted", "FileChanged", "CwdChanged", "WorktreeCreate", "WorktreeRemove",
})

var qoderEvents = identityEvents([]string{
	eventSessionStart, eventSessionEnd, eventUserPromptSubmit, eventPreToolUse, eventPostToolUse, eventPostToolUseFailure,
	eventPermissionRequest, eventPermissionDenied, eventStop, eventStopFailure, eventSubagentStart, eventSubagentStop,
	eventPreCompact, eventPostCompact, eventNotify, eventInstructionsLoaded, "ConfigChange", "CwdChanged", "FileChanged",
	"WorktreeCreate", "WorktreeRemove", "Elicitation", "ElicitationResult",
})

var factoryEvents = identityEvents([]string{
	eventPreToolUse, eventPostToolUse, eventUserPromptSubmit, eventNotify, eventStop, eventSubagentStop,
	eventPreCompact, eventSessionStart, eventSessionEnd,
})

// dialectSpecs are the harnesses rendered through the generic handler shape,
// built on first use. Each entry cites the vendor page it was read from (read
// 2026-10-05); events the vendor does not document are left out, and a matcher
// is only accepted on the events the vendor says honor one.
var dialectSpecs = sync.OnceValue(func() map[string]hookSpec {
	specs := baseDialectSpecs()
	for name, spec := range moreDialectSpecs() {
		specs[name] = spec
	}
	return specs
})

func baseDialectSpecs() map[string]hookSpec {
	return map[string]hookSpec{
		// Qwen Code: .qwen/settings.json `hooks`, Claude nesting, seconds, regex matcher
		// that accepts Claude tool aliases. https://qwenlm.github.io/qwen-code-docs/en/users/features/hooks/
		config.HarnessQwen: {
			name: config.HarnessQwen, events: qwenEvents, nested: true, matcherPassthrough: true,
			async: true, status: true, shape: claudeShape,
			matcherEvents: allBut(qwenEvents, "PostToolBatch", eventUserPromptSubmit, "MessageDisplay", eventStop),
			scriptVar:     "$QWEN_PROJECT_DIR",
		},
		// Augment (Auggie CLI): .augment/settings.json `hooks`, five events, matcher on
		// tool events only and in Augment's own tool names, timeout in milliseconds.
		// The docs name no way to address a project script. https://docs.augmentcode.com/cli/hooks
		config.HarnessAugment: {
			name: config.HarnessAugment,
			events: identityEvents([]string{
				eventSessionStart, eventSessionEnd, eventPreToolUse, eventPostToolUse, eventStop,
			}),
			nested: true, shape: &handlerShape{typed: true, timeoutMS: true},
			matcherEvents: set(eventPreToolUse, eventPostToolUse),
		},
		// CodeBuddy Code: .codebuddy/settings.json `hooks`, Claude nesting and tool names,
		// seconds. https://www.codebuddy.ai/docs/cli/hooks
		config.HarnessCodeBuddy: {
			name: config.HarnessCodeBuddy, events: codebuddyEvents, nested: true, matcherPassthrough: true,
			shape:         claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPreCompact, eventSessionStart, eventNotify),
			scriptVar:     "$CODEBUDDY_PROJECT_DIR",
		},
		// Qoder CLI: .qoder/settings.json `hooks`, Claude nesting and tool names, seconds,
		// exec-form args, async and a glob `if`. https://docs.qoder.com/cli/hooks
		config.HarnessQoder: {
			name: config.HarnessQoder, events: qoderEvents, nested: true, matcherPassthrough: true,
			args: true, async: true, condition: true, shape: claudeShape,
			matcherEvents: allBut(qoderEvents, eventUserPromptSubmit, eventStop, eventStopFailure, "CwdChanged",
				"WorktreeCreate", "WorktreeRemove"),
			scriptVar: "$QODER_PROJECT_DIR",
		},
		// Command Code: .commandcode/settings.json `hooks`, four events, seconds; the
		// matcher is a regex over the tool display name (SHELL, READ, WRITE, EDIT), so a
		// Claude matcher does not carry over. https://commandcode.ai/docs/hooks
		config.HarnessCommandCode: {
			name: config.HarnessCommandCode,
			events: identityEvents([]string{
				eventSessionStart, eventPreToolUse, eventPostToolUse, eventStop,
			}),
			nested: true, shape: claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse),
			scriptVar:     "$COMMANDCODE_PROJECT_DIR",
		},
		// Letta Code: .letta/settings.json `hooks`, Claude nesting and tool names,
		// milliseconds; hooks run from the project directory.
		// https://docs.letta.com/letta-code/hooks
		config.HarnessLetta: {
			name: config.HarnessLetta,
			events: identityEvents([]string{
				eventPreToolUse, eventPostToolUse, eventPostToolUseFailure, eventPermissionRequest, eventUserPromptSubmit,
				eventNotify, eventStop, eventSubagentStop, eventPreCompact, eventSessionStart, eventSessionEnd,
			}),
			nested: true, matcherPassthrough: true, shape: &handlerShape{typed: true, timeoutMS: true},
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPostToolUseFailure, eventPermissionRequest),
			scriptCwd:     true,
		},
		// Factory Droid: .factory/hooks.json keyed by event at the document root, Claude
		// nesting, seconds; tools are Execute/Create rather than Bash/Write, and
		// scripts are addressed through $FACTORY_PROJECT_DIR.
		// https://docs.factory.com/reference/hooks-reference
		config.HarnessFactory: {
			name: config.HarnessFactory, events: factoryEvents, nested: true, rootKeyed: true, shape: claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse),
			scriptVar:     "$FACTORY_PROJECT_DIR",
		},
		// Google Antigravity: .agents/hooks.json is a map of named hook groups; ai-rulez
		// owns the group "ai-rulez". Three events overlap Claude Code's, seconds, tools
		// are snake_case (run_command). https://antigravity.google/docs/hooks
		config.HarnessAntigravity: {
			name: config.HarnessAntigravity,
			events: identityEvents([]string{
				eventPreToolUse, eventPostToolUse, eventStop,
			}),
			nested: true, container: []string{antigravityGroup}, shape: claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse),
			scriptCwd:     true,
		},
		// GitLab Duo CLI: .gitlab/duo/hooks.json `hooks`, SessionStart only; the matcher
		// is a regex over the session source (startup, resume), seconds. Project hooks
		// stay off until the user opts in. https://docs.gitlab.com/user/gitlab_duo_cli/customize/
		config.HarnessGitLabDuo: {
			name:   config.HarnessGitLabDuo,
			events: identityEvents([]string{eventSessionStart}),
			nested: true, shape: claudeShape,
			matcherEvents: set(eventSessionStart),
			scriptVar:     "$DUO_PROJECT_DIR",
			note: "Duo CLI ignores project hooks until they are enabled: run it with --enable-project-hooks " +
				"or set GITLAB_ENABLE_PROJECT_HOOKS=true",
		},
		// Devin CLI: .devin/hooks.v1.json keyed by event at the document root (user scope
		// is the `hooks` key of ~/.config/devin/config.json), eight events, seconds.
		// https://docs.devin.ai/cli/extensibility/hooks/overview
		config.HarnessDevin: {
			name: config.HarnessDevin,
			events: events(
				eventSessionStart, eventSessionStart, eventSessionEnd, eventSessionEnd, eventPreToolUse, eventPreToolUse,
				eventPostToolUse, eventPostToolUse, eventPermissionRequest, eventPermissionRequest,
				eventUserPromptSubmit, eventUserPromptSubmit, eventStop, eventStop, eventPostCompact, "PostCompaction",
			),
			nested: true, rootKeyed: true, userContainer: []string{keyHooks}, shape: claudeShape,
			matcherEvents: set(eventPreToolUse, eventPostToolUse, eventPermissionRequest),
			scriptVar:     "$DEVIN_PROJECT_DIR",
			note:          "Devin also loads the hooks of .claude/settings.json, so a hook generated for both claude and devin runs twice",
		},
	}
}

// antigravityGroup is the name of the hook group ai-rulez owns in .agents/hooks.json.
const antigravityGroup = "ai-rulez"
