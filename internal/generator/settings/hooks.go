package settings

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/toolnames"
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
		Matcher    string `json:"matcher,omitempty"`
		Bash       string `json:"bash"`
		TimeoutSec int    `json:"timeoutSec,omitempty"`
	}
)

// hookRender is the rendering of [[hooks]] for one harness: the entries of each
// native event, in the order the events first appear in the configuration.
type hookRender struct {
	events  []string
	entries map[string][]json.RawMessage
	// flat is the single list of a harness whose entries carry their own event
	// (Kiro, Vibe), in configuration order.
	flat []json.RawMessage
}

// count is the number of handlers rendered so far; flat entries are named after it.
func (r *hookRender) count() int {
	n := len(r.flat)
	for _, entries := range r.entries {
		n += len(entries)
	}
	return n
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
func warn(d *diag.Collector, harness, msg string, kv ...any) {
	d.Warn(fmt.Sprintf("[[hooks]] not generated for %s: %s", harness, msg), kv...)
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
			warn(cfg.Diag, spec.name, fmt.Sprintf("the event %s has no equivalent", g.Event),
				"hint", "restrict the group with targets, or remove it")
			continue
		}
		matcher, ok := groupMatcher(cfg.Diag, g, spec)
		if !ok {
			continue
		}
		if matcher == "" && spec.defaultMatcher != "" && (spec.matcherRequired || spec.matcherEvents[native]) {
			matcher = spec.defaultMatcher
		}
		if matcher != "" && matcher != spec.defaultMatcher && spec.matcherEvents != nil && !spec.matcherEvents[native] {
			warn(cfg.Diag, spec.name, fmt.Sprintf("%s ignores a matcher on %s, so the group would run on every occurrence", spec.name, native),
				"hint", "remove the matcher or set targets to leave this harness out")
			continue
		}
		handlers := make([]json.RawMessage, 0, len(g.Hooks))
		for j := range g.Hooks {
			name := fmt.Sprintf("ai-rulez-%s-%d", strings.ToLower(native), out.count()+len(handlers)+1)
			raw, ok, err := renderHandler(cfg, spec, g, &g.Hooks[j], handlerContext{name: name, event: native, matcher: matcher})
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
		if spec.flat != nil {
			out.flat = append(out.flat, handlers...)
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
// over to a harness with other tool names, so it is rewritten through the
// harness's tool vocabulary (internal/toolnames) where the vendor documents it,
// and otherwise needs an explicit override.
func groupMatcher(d *diag.Collector, g *config.HookGroup, spec hookSpec) (string, bool) {
	override, hasOverride := g.Matchers[spec.name]
	matcher := g.Matcher
	if hasOverride {
		matcher = override
	}
	switch {
	case matcher == "":
		return "", true
	case hasOverride || spec.matcherPassthrough:
		return matcher, true
	}
	if vocab, ok := toolnames.For(spec.name); ok {
		translated, unmapped, ok := vocab.TranslateMatcher(matcher)
		if ok {
			return translated, true
		}
		warn(d, spec.name, fmt.Sprintf("the matcher %q of the %s group names Claude Code tools; %s documents no equivalent of %q",
			matcher, g.Event, spec.name, unmapped),
			"hint", fmt.Sprintf("set matchers.%s to the %s equivalent, or restrict the group with targets", spec.name, spec.name))
		return "", false
	}
	warn(d, spec.name, fmt.Sprintf("the matcher %q of the %s group names Claude Code tools", matcher, g.Event),
		"hint", fmt.Sprintf("set matchers.%s to the %s equivalent, or restrict the group with targets", spec.name, spec.name))
	return "", false
}

// handlerContext is what a handler is rendered under: its generated name (flat
// layouts need one), the native event and the resolved matcher.
type handlerContext struct{ name, event, matcher string }

func renderHandler(cfg *config.Config, spec hookSpec, g *config.HookGroup, action *config.HookAction, hc handlerContext,
) (raw json.RawMessage, ok bool, err error) {
	matcher := hc.matcher
	if action.Type != "" && action.Type != config.HookTypeCommand {
		warn(cfg.Diag, spec.name, fmt.Sprintf("a %s handler has type %q; only command handlers are generated", g.Event, action.Type))
		return nil, false, nil
	}
	if action.If != "" && !spec.condition {
		warn(cfg.Diag, spec.name, fmt.Sprintf("a %s handler sets 'if', which %s has no equivalent of; running it unconditionally would widen it", g.Event, spec.name))
		return nil, false, nil
	}
	if action.Async && !spec.async {
		warn(cfg.Diag, spec.name, fmt.Sprintf("a %s handler is async, which %s cannot express", g.Event, spec.name))
		return nil, false, nil
	}
	if action.Script != "" && !config.IsSafeHookScript(action.Script) {
		warn(cfg.Diag, spec.name, fmt.Sprintf("a %s handler has an unsafe script %q; a script path may only contain letters, digits, '.', '_', '-' and '/'",
			g.Event, action.Script))
		return nil, false, nil
	}
	command, args, ok := handlerCommand(cfg, spec, action)
	if !ok {
		warn(cfg.Diag, spec.name, fmt.Sprintf("a %s handler uses script %q, but %s documents no way to address a project file", g.Event, action.Script, spec.name),
			"hint", "use 'command' with a path the harness resolves, or set targets to leave this harness out")
		return nil, false, nil
	}
	if len(args) > 0 && !spec.args {
		command += " " + shellJoin(args)
		args = nil
	}

	var value any
	switch {
	case spec.flat != nil:
		value = spec.flat(hc, command, action)
	case spec.shape != nil:
		value = spec.shape.handler(spec, matcher, command, args, action)
	}
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
	case config.HarnessCopilot, config.HarnessCopilotCLI:
		value = copilotEntry{Type: config.HookTypeCommand, Matcher: matcher, Bash: command, TimeoutSec: action.Timeout}
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
func handlerCommand(cfg *config.Config, spec hookSpec, action *config.HookAction) (command string, args []string, ok bool) {
	if action.Script == "" {
		return action.Command, action.Args, true
	}
	script := path.Clean(filepath.ToSlash(action.Script))
	if cfg.UserScope {
		return quote(filepath.ToSlash(filepath.Join(cfg.ConfigDir, filepath.FromSlash(script)))), action.Args, true
	}
	// The script path is single-quoted on top of the config-time allowlist
	// (config.IsSafeHookScript); only the root variable stays double-quoted, because
	// it has to expand.
	quoted := quote(script)
	switch spec.name {
	case config.HarnessClaude:
		return `"${CLAUDE_PROJECT_DIR}"/` + quoted, action.Args, true
	case config.HarnessGemini:
		return `"$GEMINI_PROJECT_DIR"/` + quoted, action.Args, true
	case config.HarnessCodex, config.HarnessCopilot, config.HarnessCopilotCLI:
		return `"$(git rev-parse --show-toplevel)"/` + quoted, action.Args, true
	}
	switch {
	case spec.scriptVar != "":
		return `"` + spec.scriptVar + `"/` + quoted, action.Args, true
	case spec.scriptCwd:
		return quote("./" + script), action.Args, true
	}
	return "", nil, false
}

// quote single-quotes s for a POSIX shell. Inside single quotes nothing is special,
// so unlike double quotes ($, `, \, ! and " all stay live there) only the quote
// itself needs closing, escaping and reopening.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

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
	if spec.userOnly && !cfg.UserScope {
		if targetsAny(cfg, harness) {
			warn(cfg.Diag, harness, harness+" ignores project-level hooks, so they are only generated with --user")
		}
		return nil, nil
	}
	rendered, err := renderHooks(cfg, spec)
	if err != nil {
		return nil, err
	}
	if len(rendered.events) > 0 && spec.note != "" {
		cfg.Diag.Warn(fmt.Sprintf("[[hooks]] for %s: %s", harness, spec.note))
	}
	keys := make([]jsonmerge.OwnedKey, 0, len(rendered.events)+2)
	container := spec.containerPath(cfg)
	if len(rendered.flat) > 0 {
		keys = append(keys, arrayKey(cfg, docPath, container, rendered.flat))
	}
	for _, event := range rendered.events {
		keys = append(keys, arrayKey(cfg, docPath, append(slices.Clone(container), event), rendered.entries[event]))
	}
	if len(keys) > 0 {
		keys = append(spec.requiredKeys(cfg, docPath), keys...)
	}
	return keys, nil
}

// requiredKeys are the scalars a harness's hooks document needs to be valid
// (Cursor's and Kiro's `version`, ZCode's `hooks.enabled`). ai-rulez owns one
// only when the document lacks it or an earlier run added it, and asks clean to
// drop it only when nothing else is left in the file: a value the consumer wrote
// is theirs.
func (s hookSpec) requiredKeys(cfg *config.Config, docPath string) []jsonmerge.OwnedKey {
	var keys []jsonmerge.OwnedKey
	for _, req := range s.required {
		if req.userOnly && !cfg.UserScope {
			continue
		}
		if existing := readPath(cfg, docPath, req.path); existing != nil && !hookClaimedBefore(cfg, docPath, req.path) {
			if wanted, err := json.Marshal(req.value); err == nil && !equalJSON(existing, wanted) {
				cfg.Diag.Warn(fmt.Sprintf("[[hooks]] for %s: %s is %s, but the hooks need %s; the existing value is kept, so they may not run",
					s.name, strings.Join(req.path, "."), existing, wanted))
			}
			continue
		}
		keys = append(keys, jsonmerge.OwnedKey{Path: req.path, Value: req.value, Alone: true})
	}
	return keys
}

func hookClaimedBefore(cfg *config.Config, docPath string, keyPath []string) bool {
	ranged := cfg.Run.PreviousClaims(documentRel(cfg, docPath))
	for ix := range ranged {
		claim := ranged[ix]
		if equalPath(claim.Path, keyPath) {
			return true
		}
	}
	return false
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
	return OwnedHooksDocument(cfg, config.HarnessCopilot)
}

// OwnedHooksDocument renders the hooks file of a harness whose hooks live in a
// file of their own that the harness loads next to any other (Copilot and Copilot
// CLI read every *.json of .github/hooks). The copilot and copilot-cli presets
// write the same path, so both render through here and agree byte for byte.
func OwnedHooksDocument(cfg *config.Config, harness string) (body string, ok bool, err error) {
	keys, ok, err := OwnedHooksKeys(cfg, harness)
	if err != nil || !ok {
		return "", false, err
	}
	body, err = RenderOwnedHooks(keys)
	if err != nil {
		return "", false, fmt.Errorf("marshal %s hooks: %w", harness, err)
	}
	return body, true, nil
}

// HasHookDialect reports whether a harness has a hooks renderer; it is the
// `dialect` a `hooks` sidecar of a provider spec names.
func HasHookDialect(name string) bool {
	_, ok := specFor(name)
	return ok
}

// HookDialectOwnsFile reports whether the harness's hooks file is owned outright
// (see OwnedHooksDocument) rather than merged into a shared document.
func HookDialectOwnsFile(name string) bool {
	spec, _ := specFor(name)
	return spec.ownedFile
}

// targetsAny reports whether any [[hooks]] group applies to the harness.
func targetsAny(cfg *config.Config, harness string) bool {
	for i := range cfg.Hooks {
		if cfg.Hooks[i].HookTargetsHarness(harness) {
			return true
		}
	}
	return false
}
