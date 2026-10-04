package settings

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
)

// Handler documents. They differ per harness in field names and units, and are
// separate from the plugin renderer's hookActionDoc on purpose: a settings file
// omits `async` when it is false, and the project-root commands below have no
// plugin root to rewrite.
type (
	claudeHandler struct {
		Type          string   `json:"type"`
		Command       string   `json:"command"`
		Args          []string `json:"args,omitempty"`
		Timeout       int      `json:"timeout,omitempty"`
		Async         bool     `json:"async,omitempty"`
		If            string   `json:"if,omitempty"`
		StatusMessage string   `json:"statusMessage,omitempty"`
	}
	codexHandler struct {
		Type          string `json:"type"`
		Command       string `json:"command"`
		Timeout       int    `json:"timeout,omitempty"`
		Async         bool   `json:"async,omitempty"`
		StatusMessage string `json:"statusMessage,omitempty"`
	}
	geminiHandler struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		// Timeout is in milliseconds.
		Timeout int `json:"timeout,omitempty"`
	}
	group struct {
		Matcher string            `json:"matcher,omitempty"`
		Hooks   []json.RawMessage `json:"hooks"`
	}
	cursorEntry struct {
		Type    string `json:"type,omitempty"`
		Command string `json:"command"`
		Timeout int    `json:"timeout,omitempty"`
		Matcher string `json:"matcher,omitempty"`
	}
	copilotEntry struct {
		Type       string `json:"type"`
		Bash       string `json:"bash"`
		TimeoutSec int    `json:"timeoutSec,omitempty"`
	}
)

// hookRender is the rendering of [[hooks]] for one harness: the entries of each
// native event, in the order the events first appear in the configuration.
type hookRender struct {
	events  []string
	entries map[string][]json.RawMessage
}

func (r *hookRender) add(event string, raw json.RawMessage) {
	if r.entries == nil {
		r.entries = map[string][]json.RawMessage{}
	}
	if _, seen := r.entries[event]; !seen {
		r.events = append(r.events, event)
	}
	r.entries[event] = append(r.entries[event], raw)
}

// warn reports a declaration the harness cannot express. The channel is the one
// presets use for generate-time advice, so it is deduplicated per run and
// silenced by clean.
func warn(harness, msg string, kv ...any) {
	rulefiles.Warn(fmt.Sprintf("[[hooks]] not generated for %s: %s", harness, msg), kv...)
}

// renderHooks renders cfg.Hooks for a harness.
func renderHooks(cfg *config.Config, spec hookSpec) (hookRender, error) {
	var out hookRender
	for i := range cfg.Hooks {
		g := &cfg.Hooks[i]
		if !g.HookTargetsHarness(spec.name) {
			continue
		}
		native, ok := spec.events[g.Event]
		if !ok {
			warn(spec.name, fmt.Sprintf("the event %s has no equivalent", g.Event),
				"hint", "restrict the group with targets, or remove it")
			continue
		}
		matcher, ok := groupMatcher(g, spec)
		if !ok {
			continue
		}
		handlers := make([]json.RawMessage, 0, len(g.Hooks))
		for j := range g.Hooks {
			raw, ok, err := renderHandler(cfg, spec, g, &g.Hooks[j], matcher)
			if err != nil {
				return hookRender{}, err
			}
			if ok {
				handlers = append(handlers, raw)
			}
		}
		if len(handlers) == 0 {
			continue
		}
		if !spec.nested {
			for _, raw := range handlers {
				out.add(native, raw)
			}
			continue
		}
		raw, err := json.Marshal(group{Matcher: matcher, Hooks: handlers})
		if err != nil {
			return hookRender{}, fmt.Errorf("marshal %s hook group: %w", spec.name, err)
		}
		out.add(native, raw)
	}
	return out, nil
}

// groupMatcher resolves the matcher a group renders with for a harness. ok is
// false when the group cannot be rendered there: a Claude matcher does not carry
// over to a harness with other tool names, so it needs an explicit override.
func groupMatcher(g *config.HookGroup, spec hookSpec) (string, bool) {
	override, hasOverride := g.Matchers[spec.name]
	matcher := g.Matcher
	if hasOverride {
		matcher = override
	}
	switch {
	case matcher == "":
		return "", true
	case spec.matcherless:
		warn(spec.name, fmt.Sprintf("%s hooks have no matcher, so the %s group would run on every occurrence", spec.name, g.Event),
			"hint", "remove the matcher or set targets to leave this harness out")
		return "", false
	case hasOverride || spec.matcherPassthrough:
		return matcher, true
	}
	warn(spec.name, fmt.Sprintf("the matcher %q of the %s group names Claude Code tools", matcher, g.Event),
		"hint", fmt.Sprintf("set matchers.%s to the %s equivalent, or restrict the group with targets", spec.name, spec.name))
	return "", false
}

func renderHandler(cfg *config.Config, spec hookSpec, g *config.HookGroup, action *config.HookAction, matcher string,
) (raw json.RawMessage, ok bool, err error) {
	if action.If != "" && !spec.condition {
		warn(spec.name, fmt.Sprintf("a %s handler sets 'if', which %s has no equivalent of; running it unconditionally would widen it", g.Event, spec.name))
		return nil, false, nil
	}
	if action.Async && !spec.async {
		warn(spec.name, fmt.Sprintf("a %s handler is async, which %s cannot express", g.Event, spec.name))
		return nil, false, nil
	}
	command, args := handlerCommand(cfg, spec, action)
	if len(args) > 0 && !spec.args {
		command += " " + shellJoin(args)
		args = nil
	}

	var value any
	switch spec.name {
	case config.HarnessClaude:
		value = claudeHandler{Type: config.HookTypeCommand, Command: command, Args: args, Timeout: action.Timeout,
			Async: action.Async, If: action.If, StatusMessage: action.StatusMessage}
	case config.HarnessCodex:
		value = codexHandler{Type: config.HookTypeCommand, Command: command, Timeout: action.Timeout,
			Async: action.Async, StatusMessage: action.StatusMessage}
	case config.HarnessGemini:
		value = geminiHandler{Type: config.HookTypeCommand, Command: command, Timeout: action.Timeout * 1000}
	case config.HarnessCursor:
		value = cursorEntry{Command: command, Timeout: action.Timeout, Matcher: matcher}
	case config.HarnessCopilot:
		value = copilotEntry{Type: config.HookTypeCommand, Bash: command, TimeoutSec: action.Timeout}
	}
	raw, err = json.Marshal(value)
	if err != nil {
		return nil, false, fmt.Errorf("marshal %s hook handler: %w", spec.name, err)
	}
	return raw, true, nil
}

// handlerCommand resolves the command a harness spawns for an action. A
// `script` is a file of the project (or, for user scope, of the user config
// directory) and is addressed through the variable the harness documents for its
// project root; a `command` is copied verbatim.
func handlerCommand(cfg *config.Config, spec hookSpec, action *config.HookAction) (command string, args []string) {
	if action.Script == "" {
		return action.Command, action.Args
	}
	script := path.Clean(filepath.ToSlash(action.Script))
	if cfg.UserScope {
		return quote(filepath.Join(cfg.ConfigDir, filepath.FromSlash(script))), action.Args
	}
	switch spec.name {
	case config.HarnessClaude:
		return `"${CLAUDE_PROJECT_DIR}"/` + script, action.Args
	case config.HarnessGemini:
		return `"$GEMINI_PROJECT_DIR"/` + script, action.Args
	case config.HarnessCodex, config.HarnessCopilot:
		return `"$(git rev-parse --show-toplevel)"/` + script, action.Args
	default: // cursor runs project hooks from the project root
		return "./" + script, action.Args
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

// shellJoin quotes argv for a shell command line.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		if arg != "" && !strings.ContainsAny(arg, " \t\n\"'\\$`!&|;<>()*?[]{}~#") {
			parts[i] = arg
			continue
		}
		parts[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}

// HookKeys returns the owned keys of the harness's hooks document at docPath for
// cfg.Hooks: one array per native event, element-owned. docPath may be empty when
// no document exists yet (clean's fallback claims). It returns nil for a harness
// without hook support or a configuration without hooks for it.
func HookKeys(cfg *config.Config, harness, docPath string) ([]jsonmerge.OwnedKey, error) {
	spec, ok := specFor(harness)
	if !ok || cfg == nil || len(cfg.Hooks) == 0 || inScopeRun(cfg) {
		return nil, nil
	}
	rendered, err := renderHooks(cfg, spec)
	if err != nil {
		return nil, err
	}
	keys := make([]jsonmerge.OwnedKey, 0, len(rendered.events)+1)
	for _, event := range rendered.events {
		keys = append(keys, arrayKey(cfg, docPath, []string{"hooks", event}, rendered.entries[event]))
	}
	if len(keys) > 0 && harness == config.HarnessCursor {
		if version, ok := cursorVersionKey(cfg, docPath); ok {
			keys = append([]jsonmerge.OwnedKey{version}, keys...)
		}
	}
	return keys, nil
}

// cursorVersionKey owns Cursor's required `version` only when the document lacks
// one or an earlier run added it, and asks clean to drop it only when nothing
// else is left in the file. A version the consumer wrote is theirs.
func cursorVersionKey(cfg *config.Config, docPath string) (jsonmerge.OwnedKey, bool) {
	if readPath(docPath, []string{keyVersion}) != nil {
		for _, claim := range cfg.Run.PreviousClaims(documentRel(cfg, docPath)) {
			if equalPath(claim.Path, []string{keyVersion}) {
				return jsonmerge.OwnedKey{Name: keyVersion, Value: cursorHooksVersion, Alone: true}, true
			}
		}
		return jsonmerge.OwnedKey{}, false
	}
	return jsonmerge.OwnedKey{Name: keyVersion, Value: cursorHooksVersion, Alone: true}, true
}

// inScopeRun reports whether cfg renders a [[scopes]] subdirectory, where
// project-wide settings do not belong.
func inScopeRun(cfg *config.Config) bool {
	return cfg.Run != nil && cfg.Run.Scope != nil
}

// CopilotHooksDocument renders the Copilot hooks file ai-rulez owns outright
// (.github/hooks/ai-rulez.json). ok is false when no hook applies, in which case
// the file is not generated.
func CopilotHooksDocument(cfg *config.Config) (body string, ok bool, err error) {
	if cfg == nil || len(cfg.Hooks) == 0 || inScopeRun(cfg) {
		return "", false, nil
	}
	rendered, err := renderHooks(cfg, copilotSpec)
	if err != nil || len(rendered.events) == 0 {
		return "", false, err
	}
	hooks := make(map[string][]json.RawMessage, len(rendered.events))
	for _, event := range rendered.events {
		hooks[event] = rendered.entries[event]
	}
	data, err := json.MarshalIndent(map[string]any{keyVersion: copilotHooksVersion, "hooks": hooks}, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal copilot hooks: %w", err)
	}
	return string(data) + "\n", true, nil
}
