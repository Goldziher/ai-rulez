// Package hookplugins renders the top-level [[hooks]] for harnesses whose hooks
// are code, not a settings file: OpenCode, Kilo and MiMo Code load a JavaScript
// plugin, Pi a TypeScript extension and Amp a TypeScript plugin.
//
// The generated module is owned wholly by ai-rulez. It carries the hook
// declarations as one JSON literal produced by encoding/json, so no hook text is
// ever spliced into code, and a fixed runtime that spawns each hook command with a
// Claude Code style JSON document on stdin, honours its timeout and blocks a
// pre-tool call on exit code 2. Only the adapter between the harness's event API
// and that runtime differs per flavor. Event names, tool names and file
// locations were read from each vendor's plugin documentation and source (see
// docs/settings.md for the sources and dates).
package hookplugins

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/toolnames"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Flavor is the plugin dialect of a harness.
type Flavor string

// The plugin dialects.
const (
	// FlavorOpencode is OpenCode: a default export carrying the v2 setup(ctx) and the
	// v1 server(ctx), so the file loads on either generation of the plugin API.
	FlavorOpencode Flavor = "opencode"
	// FlavorOpencodeV1 is the v1 plugin API of the OpenCode forks Kilo and MiMo Code:
	// a default export of { id, server }.
	FlavorOpencodeV1 Flavor = "opencode-v1"
	// FlavorPi is a Pi extension: a default-exported factory receiving the ExtensionAPI.
	FlavorPi Flavor = "pi"
	// FlavorAmp is an Amp plugin: a default-exported function receiving the PluginAPI.
	FlavorAmp Flavor = "amp"
)

// Flavors lists the dialects, in a stable order.
var Flavors = []Flavor{FlavorOpencode, FlavorOpencodeV1, FlavorPi, FlavorAmp}

// FileName is the name of the generated module in the plugin directory.
const (
	fileNameJS = "ai-rulez-hooks.js"
	fileNameTS = "ai-rulez-hooks.ts"
)

// Project-relative paths of the generated module, per harness. Every tool loads the
// files of the directory without a registration step.
const (
	OpencodePath = ".opencode/plugins/" + fileNameJS
	KiloPath     = ".kilo/plugins/" + fileNameJS
	MimocodePath = ".mimocode/plugins/" + fileNameJS
	PiPath       = ".pi/extensions/" + fileNameTS
	AmpPath      = ".amp/plugins/" + fileNameTS
)

// User-scope paths, relative to the user's home directory.
const (
	OpencodeUserPath = ".config/opencode/plugins/" + fileNameJS
	KiloUserPath     = ".config/kilo/plugins/" + fileNameJS
	MimocodeUserPath = ".config/mimocode/plugins/" + fileNameJS
	PiUserPath       = ".pi/agent/extensions/" + fileNameTS
	AmpUserPath      = ".config/amp/plugins/" + fileNameTS
)

// IsModulePath reports whether the slash-separated, project-relative path is a
// module this package generates. The plugin directories also hold hand-written
// plugins, so only the module itself is ever gitignored, never its directory.
func IsModulePath(rel string) bool {
	dir, file := path.Split(rel)
	if file != fileNameJS && file != fileNameTS {
		return false
	}
	dir = strings.TrimSuffix(dir, "/")
	return strings.HasSuffix(dir, "/plugins") || strings.HasSuffix(dir, "/extensions")
}

// FlavorFor returns the dialect of a built-in plugin harness.
func FlavorFor(harness string) (Flavor, bool) {
	switch harness {
	case config.HarnessOpencode:
		return FlavorOpencode, true
	case config.HarnessKilo, config.HarnessMimocode:
		return FlavorOpencodeV1, true
	case config.HarnessPi:
		return FlavorPi, true
	case config.HarnessAmp:
		return FlavorAmp, true
	}
	return "", false
}

// IsFlavor reports whether f names a dialect.
func IsFlavor(f string) bool { return slices.Contains(Flavors, Flavor(f)) }

// Claude Code event names used by the tables below.
const (
	eventSessionStart       = "SessionStart"
	eventSessionEnd         = "SessionEnd"
	eventUserPromptSubmit   = "UserPromptSubmit"
	eventPreToolUse         = "PreToolUse"
	eventPostToolUse        = "PostToolUse"
	eventPostToolUseFailure = "PostToolUseFailure"
	eventPreCompact         = "PreCompact"
	eventPostCompact        = "PostCompact"
	eventStop               = "Stop"
)

// flavorSpec describes how one dialect expresses a hook.
type flavorSpec struct {
	template string
	// events maps a Claude Code event name to the tool's own, for the tables and docs.
	events map[string]string
	// blocking lists the events whose hook can stop the occurrence.
	blocking []string
	// subjects lists the events whose matcher is tested against something the tool
	// reports: the tool name for the tool events, the source of a session start, the
	// trigger of a compaction. A matcher on any other event is not expressible.
	subjects []string
	// tools maps a native tool name to the Claude Code names that also select it.
	tools map[string][]string
}

// nativeTools returns the shared tool vocabulary (internal/toolnames) of a harness
// as native name -> Claude Code names, which is what the runtime tests a matcher
// against. OpenCode's is https://opencode.ai/docs/tools/.
func nativeTools(harness string) map[string][]string {
	vocab, ok := toolnames.For(harness)
	if !ok {
		return nil
	}
	return vocab.Inverse()
}

var opencodeTools = nativeTools("opencode")

var opencodeEvents = map[string]string{
	eventSessionStart: "session.created", eventPreToolUse: "tool.execute.before",
	eventPostToolUse: "tool.execute.after", eventStop: "session.idle", eventPostCompact: "session.compacted",
}

var specs = map[Flavor]flavorSpec{
	FlavorOpencode: {
		template: "opencode.js.tmpl", events: opencodeEvents, blocking: []string{eventPreToolUse},
		subjects: []string{eventSessionStart, eventPreToolUse, eventPostToolUse}, tools: opencodeTools,
	},
	FlavorOpencodeV1: {
		template: "opencode.js.tmpl", events: opencodeEvents, blocking: []string{eventPreToolUse},
		subjects: []string{eventSessionStart, eventPreToolUse, eventPostToolUse}, tools: opencodeTools,
	},
	FlavorPi: {
		template: "pi.ts.tmpl",
		events: map[string]string{
			eventSessionStart: "session_start", eventSessionEnd: "session_shutdown", eventUserPromptSubmit: "input",
			eventPreToolUse: "tool_call", eventPostToolUse: "tool_result", eventPostToolUseFailure: "tool_result",
			eventPreCompact: "session_before_compact", eventPostCompact: "session_compact", eventStop: "agent_settled",
		},
		blocking: []string{eventPreToolUse, eventUserPromptSubmit},
		subjects: []string{
			eventSessionStart, eventPreToolUse, eventPostToolUse, eventPostToolUseFailure, eventPreCompact, eventPostCompact,
		},
		// Pi's built-in tools (extensions types: isToolCallEventType).
		tools: nativeTools("pi"),
	},
	FlavorAmp: {
		template: "amp.ts.tmpl",
		events: map[string]string{
			eventSessionStart: "session.start", eventUserPromptSubmit: "agent.start", eventPreToolUse: "tool.call",
			eventPostToolUse: "tool.result", eventPostToolUseFailure: "tool.result", eventStop: "agent.end",
		},
		blocking: []string{eventPreToolUse},
		subjects: []string{eventSessionStart, eventPreToolUse, eventPostToolUse, eventPostToolUseFailure},
		// Amp's own tool names beyond the ones it shares with Claude Code (Bash, Read, Grep, Task).
		tools: nativeTools("amp"),
	},
}

// SupportedEvents returns the Claude Code event names a dialect can express, in
// config.KnownHookEvents order.
func SupportedEvents(flavor Flavor) []string {
	spec, ok := specs[flavor]
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

// NativeEvent returns the dialect's name for a Claude Code event.
func NativeEvent(flavor Flavor, event string) (string, bool) {
	spec, ok := specs[flavor]
	if !ok {
		return "", false
	}
	native, ok := spec.events[event]
	return native, ok
}

// hookEntry is one command of an event, as the runtime reads it.
type hookEntry struct {
	Matcher string `json:"matcher,omitempty"`
	Command string `json:"command"`
	// Timeout is in milliseconds; zero leaves the runtime default.
	Timeout int  `json:"timeout,omitempty"`
	Async   bool `json:"async,omitempty"`
}

// runtimeConfig is the JSON literal embedded in the module. Maps marshal with
// sorted keys, so the output is deterministic.
type runtimeConfig struct {
	Harness  string                 `json:"harness"`
	Blocking []string               `json:"blocking"`
	Tools    map[string][]string    `json:"tools"`
	Hooks    map[string][]hookEntry `json:"hooks"`
}

type templateData struct {
	Harness string
	Config  string
	V2      bool
	Has     map[string]bool
}

// Render renders the module for a harness in a dialect. ok is false when no
// [[hooks]] group applies, in which case no file is generated. A declaration the
// harness cannot express is skipped with a warning, never approximated.
func Render(cfg *config.Config, harness string, flavor Flavor) (body string, ok bool, err error) {
	spec, known := specs[flavor]
	if !known {
		return "", false, fmt.Errorf("unknown hook plugin flavor %q", flavor)
	}
	if cfg == nil || len(cfg.Hooks) == 0 || (cfg.Run != nil && cfg.Run.Scope != nil) {
		return "", false, nil
	}
	hooks := collect(cfg, harness, flavor, &spec)
	if len(hooks) == 0 {
		return "", false, nil
	}
	rc := runtimeConfig{Harness: harness, Blocking: slices.Clone(spec.blocking), Tools: spec.tools, Hooks: hooks}
	literal, err := json.MarshalIndent(rc, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal %s hook configuration: %w", harness, err)
	}
	data := templateData{
		Harness: harness,
		Config:  string(literal),
		V2:      flavor == FlavorOpencode,
		Has:     make(map[string]bool, len(hooks)),
	}
	for event := range hooks {
		data.Has[event] = true
	}
	tmpl, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return "", false, fmt.Errorf("parse hook plugin templates: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, spec.template, data); err != nil {
		return "", false, fmt.Errorf("render %s hook plugin: %w", harness, err)
	}
	return tidy(out.String()), true, nil
}

// tidy collapses the blank lines the template actions leave and ends the file with
// one newline.
func tidy(s string) string {
	var out []string
	blank := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

// warn reports a declaration the harness cannot express, once per run.
func warn(harness, msg string, kv ...any) {
	rulefiles.Warn(fmt.Sprintf("[[hooks]] not generated for %s: %s", harness, msg), kv...)
}

// collect resolves the groups that apply to the harness into the entries of each
// supported event, in configuration order.
func collect(cfg *config.Config, harness string, flavor Flavor, spec *flavorSpec) map[string][]hookEntry {
	hooks := map[string][]hookEntry{}
	for i := range cfg.Hooks {
		g := &cfg.Hooks[i]
		if !g.HookTargetsHarness(harness) {
			continue
		}
		if _, ok := spec.events[g.Event]; !ok {
			warn(harness, fmt.Sprintf("the event %s has no equivalent in the %s plugin API", g.Event, harness),
				"hint", "restrict the group with targets, or remove it")
			continue
		}
		matcher, ok := groupMatcher(g, harness, spec)
		if !ok {
			continue
		}
		for j := range g.Hooks {
			if entry, ok := entryFor(cfg, harness, g, &g.Hooks[j], matcher); ok {
				hooks[g.Event] = append(hooks[g.Event], entry)
			}
		}
	}
	return hooks
}

// groupMatcher resolves the matcher a group renders with: the harness override or
// the group's own. A matcher on an event the plugin API reports no subject for would
// run the group on every occurrence, so such a group is skipped.
func groupMatcher(g *config.HookGroup, harness string, spec *flavorSpec) (string, bool) {
	matcher := g.Matcher
	if override, ok := g.Matchers[harness]; ok {
		matcher = override
	}
	if matcher == "" || matcher == "*" {
		return "", true
	}
	switch {
	case slices.Contains(spec.subjects, g.Event):
		if problem := matcherProblem(matcher); problem != "" {
			warn(harness, fmt.Sprintf("the matcher %q of the %s group is not a valid portable regular expression (%s), "+
				"so the group is skipped instead of running on every call or silently never", matcher, g.Event, problem),
				"hint", "fix the matcher, or set targets to leave this harness out")
			return "", false
		}
		return matcher, true
	case slices.Contains(config.HookEventsWithoutMatcher, g.Event):
		return "", true // Claude Code ignores it there too
	}
	warn(harness, fmt.Sprintf("the %s plugin API reports nothing to match the matcher %q of the %s group against, "+
		"so the group would run on every occurrence", harness, matcher, g.Event),
		"hint", "remove the matcher or set targets to leave this harness out")
	return "", false
}

func entryFor(cfg *config.Config, harness string, g *config.HookGroup, action *config.HookAction, matcher string,
) (hookEntry, bool) {
	if action.Type != "" && action.Type != config.HookTypeCommand {
		warn(harness, fmt.Sprintf("a %s handler has the type %q; only command handlers run", g.Event, action.Type))
		return hookEntry{}, false
	}
	if action.If != "" {
		warn(harness, fmt.Sprintf("a %s handler sets 'if', which the %s plugin API has no equivalent of; "+
			"running it unconditionally would widen it", g.Event, harness))
		return hookEntry{}, false
	}
	if action.Script != "" && !config.IsSafeHookScript(action.Script) {
		warn(harness, fmt.Sprintf("a %s handler has an unsafe script %q; a script path may only contain letters, digits, '.', '_', '-' and '/'",
			g.Event, action.Script))
		return hookEntry{}, false
	}
	return hookEntry{
		Matcher: matcher,
		Command: command(cfg, action),
		Timeout: action.Timeout * 1000,
		Async:   action.Async,
	}, true
}

// command resolves the shell command line of an action. A `script` is a file of
// the project (or, for user scope, of the user config directory); the hook runs
// from the project directory, so a project script is addressed relative to it.
func command(cfg *config.Config, action *config.HookAction) string {
	if action.Script == "" {
		if len(action.Args) == 0 {
			return action.Command
		}
		return action.Command + " " + shellJoin(action.Args)
	}
	script := path.Clean(filepath.ToSlash(action.Script))
	target := "./" + script
	if cfg.UserScope {
		target = filepath.Join(cfg.ConfigDir, filepath.FromSlash(script))
	}
	line := shellQuote(target)
	if len(action.Args) > 0 {
		line += " " + shellJoin(action.Args)
	}
	return line
}

// shellQuote single-quotes s for a POSIX shell: nothing inside single quotes is
// special, so only the quote itself needs closing, escaping and reopening.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

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
