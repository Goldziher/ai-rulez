package importer

import (
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Hooks and permissions are read from the tool files that carry them and become
// [[hooks]] and [permissions]. They are never enabled by the importer: convert
// writes them as a commented block of config.toml unless --enable-hooks (and
// --enable-permissions for allow rules) is given, because a hook runs a command
// on the user's machine and an allow rule widens what every harness may do.
//
// Every group keeps the harness it was read from in `targets`, so a hook that
// only ever ran in one tool does not start running in the others, and a matcher
// written in a tool's own vocabulary is stored under `matchers.<harness>`.

const (
	hookTypeCommand = "command"
	// maxHookSeconds bounds a timeout read from a tool file, so a value in
	// milliseconds that was not converted cannot become a day-long hook.
	maxHookSeconds = 3600
)

// claudeVocabulary lists the harnesses whose matchers use Claude Code's tool names.
var claudeVocabulary = map[string]bool{config.HarnessClaude: true, config.HarnessCodex: true}

// hookEventTables maps the native event names of a harness to Claude Code's.
var (
	geminiHookEvents = map[string]string{
		litSessionStart: litSessionStart, litSessionEnd: litSessionEnd, "BeforeTool": litPreToolUse,
		"AfterTool": litPostToolUse, litNotification: litNotification, "PreCompress": litPreCompact,
		"BeforeAgent": litUserPromptSubmit, "AfterAgent": litStop,
	}
	cursorHookEvents = map[string]string{
		litSessionStartCamel: litSessionStart, litSessionEndCamel: litSessionEnd, litPreToolUseCamel: litPreToolUse,
		litPostToolUseCamel: litPostToolUse, litPostToolUseFailureCamel: litPostToolUseFailure,
		litSubagentStartCamel: litSubagentStart, litSubagentStopCamel: litSubagentStop, litPreCompactCamel: litPreCompact,
		"stop": litStop, "beforeSubmitPrompt": litUserPromptSubmit,
	}
	copilotHookEvents = map[string]string{
		litSessionStartCamel: litSessionStart, litSessionEndCamel: litSessionEnd, litPreToolUseCamel: litPreToolUse,
		litPostToolUseCamel: litPostToolUse, litPostToolUseFailureCamel: litPostToolUseFailure,
		litSubagentStartCamel: litSubagentStart, litSubagentStopCamel: litSubagentStop, litPreCompactCamel: litPreCompact,
		"agentStop": litStop, "userPromptSubmitted": litUserPromptSubmit, "notification": litNotification,
		"permissionRequest": "PermissionRequest",
	}
	// rulesyncHookEvents are rulesync's canonical camelCase events that have a
	// Claude Code name (src/types/hooks.ts, CANONICAL_TO_CLAUDE_EVENT_NAMES).
	rulesyncHookEvents = map[string]string{
		litSessionStartCamel: litSessionStart, litSessionEndCamel: litSessionEnd, litPreToolUseCamel: litPreToolUse,
		litPostToolUseCamel: litPostToolUse, "beforeSubmitPrompt": litUserPromptSubmit, "stop": litStop,
		litSubagentStopCamel: litSubagentStop, litPreCompactCamel: litPreCompact, "permissionRequest": "PermissionRequest",
		"notification": litNotification, "setup": "Setup", "worktreeCreate": "WorktreeCreate",
		"worktreeRemove": "WorktreeRemove", "messageDisplay": "MessageDisplay",
		"instructionsLoaded": "InstructionsLoaded", "userPromptExpansion": "UserPromptExpansion",
		litPostToolUseFailureCamel: litPostToolUseFailure, "postToolBatch": "PostToolBatch",
		"permissionDenied": "PermissionDenied", litSubagentStartCamel: litSubagentStart,
		"taskCreated": "TaskCreated", "taskCompleted": "TaskCompleted", "stopFailure": "StopFailure",
		"teammateIdle": "TeammateIdle", "configChange": "ConfigChange", "cwdChanged": "CwdChanged",
		"fileChanged": "FileChanged", "directoryAdded": "DirectoryAdded", "postCompact": "PostCompact",
		"elicitation": "Elicitation", "elicitationResult": "ElicitationResult",
		"preModelSwitch": "PreModelSwitch", "postModelSwitch": "PostModelSwitch",
	}
	// rulesyncToolBlocks maps a rulesync per-tool hooks block to the ai-rulez
	// harness and the event table of that tool's native names.
	rulesyncToolBlocks = map[string]struct {
		harness string
		events  map[string]string
	}{
		"claudecode": {config.HarnessClaude, rulesyncHookEvents},
		"codexcli":   {config.HarnessCodex, rulesyncHookEvents},
		litCursor:    {config.HarnessCursor, cursorHookEvents},
		litCopilot:   {config.HarnessCopilot, copilotHookEvents},
		"copilotcli": {config.HarnessCopilot, copilotHookEvents},
	}
)

// hookSource describes one tool file's hooks: how events and matchers translate.
type hookSource struct {
	file    string
	harness string // ai-rulez harness the hooks are restricted to; "" for rulesync's shared block
	events  map[string]string
	// timeoutMs is set when the file stores timeouts in milliseconds.
	timeoutMs bool
}

// event translates a native event name to Claude Code's. Claude Code and Codex
// files use Claude Code's own names, so those pass through; so does a Claude Code
// name written in a rulesync per-tool block.
func (s hookSource) event(native string) (string, bool) {
	if e, ok := s.events[native]; ok {
		return e, true
	}
	if s.events == nil || claudeVocabulary[s.harness] {
		for _, known := range config.KnownHookEvents {
			if known == native {
				return native, true
			}
		}
		return native, s.events == nil
	}
	return "", false
}

// hookDoc is a hook handler as the tool files write it; only what ai-rulez can
// express is read, the rest is reported key by key.
type hookDoc map[string]json.RawMessage

func (d hookDoc) str(key string) string {
	var s string
	if raw, ok := d[key]; ok && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func (d hookDoc) number(key string) (float64, bool) {
	var n float64
	if raw, ok := d[key]; ok && json.Unmarshal(raw, &n) == nil {
		return n, true
	}
	return 0, false
}

func (d hookDoc) boolean(key string) (value, present bool) {
	if raw, ok := d[key]; ok && json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	return false, false
}

// hookKeys are the handler keys importHandler reads; every other key is reported.
var hookKeys = map[string]bool{
	"type": true, "command": true, "bash": true, "args": true, litTimeout: true, "timeoutSec": true,
	"async": true, "if": true, "statusMessage": true, litMatcher: true,
}

// hookBuilder collects the groups of one importer.
type hookBuilder struct{ p *Plan }

// hooksOf reads the event table of a native hooks object: event -> groups. A
// group is either {matcher, hooks:[handler]} (nested) or a handler itself (flat).
func (b *hookBuilder) fromEventTable(src hookSource, table map[string]json.RawMessage, where string) {
	events := make([]string, 0, len(table))
	for e := range table {
		events = append(events, e)
	}
	sort.Strings(events)
	var unknown []string
	for _, native := range events {
		event, ok := src.event(native)
		if !ok {
			unknown = append(unknown, native)
			continue
		}
		var entries []json.RawMessage
		if err := json.Unmarshal(table[native], &entries); err != nil {
			b.p.add(newFinding(StatusUnsupported, src.file, where+native, "", "hook entries are not a list"))
			continue
		}
		for i, raw := range entries {
			b.entry(src, where+native, event, i, raw)
		}
	}
	if len(unknown) > 0 {
		b.p.add(newFinding(StatusUnsupported, src.file, strings.TrimSuffix(where, "."), "",
			"events with no ai-rulez hook event were not imported: "+strings.Join(unknown, ", ")))
	}
}

// entry converts one element of an event's list.
func (b *hookBuilder) entry(src hookSource, field, event string, index int, raw json.RawMessage) {
	var d hookDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		b.p.add(newFinding(StatusUnsupported, src.file, field, "", "hook entry is not an object"))
		return
	}
	at := fmt.Sprintf("%s[%d]", field, index)
	group := config.HookGroup{Event: event}
	matcher := d.str(litMatcher)
	var handlers []hookDoc
	if inner, nested := d[litHooks]; nested {
		var list []hookDoc
		if err := json.Unmarshal(inner, &list); err != nil {
			b.p.add(newFinding(StatusUnsupported, src.file, at+".hooks", "", "handlers are not a list of objects"))
			return
		}
		handlers = list
		for k := range d {
			if k != litMatcher && k != litHooks {
				b.p.add(newFinding(StatusDropped, src.file, at+"."+k, "", "group key has no ai-rulez equivalent"))
			}
		}
	} else {
		handlers = []hookDoc{d}
	}
	for i, h := range handlers {
		action, ok := b.handler(src, fmt.Sprintf("%s.hooks[%d]", at, i), h, len(handlers) == 1 && !hasNested(d))
		if ok {
			group.Hooks = append(group.Hooks, action)
		}
	}
	if len(group.Hooks) == 0 {
		return
	}
	b.setMatcher(src, &group, matcher, at)
	b.add(src, group)
}

func hasNested(d hookDoc) bool { _, ok := d[litHooks]; return ok }

// setMatcher stores a matcher in the vocabulary it was written in.
func (b *hookBuilder) setMatcher(src hookSource, g *config.HookGroup, matcher, at string) {
	if matcher == "" {
		return
	}
	if src.harness == "" || claudeVocabulary[src.harness] {
		g.Matcher = matcher
		return
	}
	g.Matchers = map[string]string{src.harness: matcher}
	b.p.add(newFinding(StatusApproximated, src.file, at+".matcher", "hooks.matchers."+src.harness,
		"the matcher uses "+src.harness+"'s own tool names, so it is kept as matchers."+src.harness+" and the group is restricted to that harness"))
}

// handler converts one handler object. flat handlers carry the matcher beside
// the command, so litMatcher is not a handler key for them.
func (b *hookBuilder) handler(src hookSource, field string, d hookDoc, flat bool) (config.HookAction, bool) {
	typ := d.str("type")
	if typ != "" && typ != hookTypeCommand {
		b.p.add(newFinding(StatusUnsupported, src.file, field, "",
			fmt.Sprintf("a %q hook is not a command; ai-rulez hooks run commands only", typ)))
		return config.HookAction{}, false
	}
	command := d.str("command")
	if command == "" {
		command = d.str("bash")
	}
	if command == "" {
		b.p.add(newFinding(StatusUnsupported, src.file, field, "", "hook has no command to run"))
		return config.HookAction{}, false
	}
	if isOwnGuard(command) {
		b.p.add(newFinding(StatusDropped, src.file, field, "", "this is ai-rulez's own guard hook, which generate adds itself when [guard] is on"))
		return config.HookAction{}, false
	}
	if on, present := d.boolean("enabled"); present && !on {
		b.p.add(newFinding(StatusDropped, src.file, field, "", "the hook is marked disabled in the source"))
		return config.HookAction{}, false
	}
	a := config.HookAction{Command: command}
	if raw, ok := d["args"]; ok {
		if err := json.Unmarshal(raw, &a.Args); err != nil {
			b.p.add(newFinding(StatusDropped, src.file, field+".args", "", "args is not a list of strings"))
			a.Args = nil
		}
	}
	for _, key := range []string{litTimeout, "timeoutSec"} {
		if n, ok := d.number(key); ok {
			a.Timeout = b.seconds(src, field+"."+key, n, src.timeoutMs && key == litTimeout)
		}
	}
	a.Async, _ = d.boolean("async")
	a.If = d.str("if")
	a.StatusMessage = d.str("statusMessage")
	var left []string
	for k := range d {
		if hookKeys[k] || (k == litMatcher && flat) {
			continue
		}
		left = append(left, k)
	}
	if len(left) > 0 {
		sort.Strings(left)
		b.p.add(newFinding(StatusDropped, src.file, field, "", "handler keys with no ai-rulez equivalent were not carried: "+strings.Join(left, ", ")))
	}
	switch {
	case strings.Contains(command, ".rulesync/"):
		b.p.add(newFinding(StatusNeedsAction, src.file, field+".command", "",
			"the command runs a script from the rulesync input tree; copy the script into the project and point the hook at it (a script path)"))
	case strings.Contains(command, "PLUGIN_ROOT"):
		b.p.add(newFinding(StatusNeedsAction, src.file, field+".command", "",
			"the command runs a script from the package root, which does not exist in this project; copy the script into the project and point the hook at it (a script path)"))
	}
	return a, true
}

// seconds converts a timeout to seconds, rounding a millisecond value up.
func (b *hookBuilder) seconds(src hookSource, field string, n float64, ms bool) int {
	if ms {
		n = math.Ceil(n / 1000)
	}
	if n < 0 || n > maxHookSeconds {
		b.p.add(newFinding(StatusDropped, src.file, field, "", fmt.Sprintf("timeout %v is outside 0-%d seconds and was not carried", n, maxHookSeconds)))
		return 0
	}
	return int(n)
}

func isOwnGuard(command string) bool {
	return strings.Contains(command, "ai-rulez") && strings.HasSuffix(strings.TrimSpace(command), " guard")
}

func (b *hookBuilder) add(src hookSource, g config.HookGroup) {
	if src.harness != "" {
		g.Targets = []string{src.harness}
	}
	b.p.Hooks = append(b.p.Hooks, g)
	b.p.add(newFinding(StatusMapped, src.file, "hooks."+g.Event, "hooks."+g.Event, ""))
}

// importNativeHooks reads the hooks of the tool files that hold them.
func importNativeHooks(p *Plan, r *reader) {
	b := &hookBuilder{p: p}
	for _, spec := range []struct {
		hookSource
		container string // key holding the event table; "" when the table is the document
	}{
		{hookSource{file: ".claude/settings.json", harness: config.HarnessClaude}, litHooks},
		{hookSource{file: ".codex/hooks.json", harness: config.HarnessCodex}, litHooks},
		{hookSource{file: ".gemini/settings.json", harness: config.HarnessGemini, events: geminiHookEvents, timeoutMs: true}, litHooks},
		{hookSource{file: ".cursor/hooks.json", harness: config.HarnessCursor, events: cursorHookEvents}, litHooks},
	} {
		if table, ok := b.readTable(r, spec.hookSource, spec.container); ok {
			b.fromEventTable(spec.hookSource, table, "hooks.")
		}
	}
	b.copilotHooks(r)
	b.codexInline(r)
}

// readTable reads a JSON(C) file and returns the object under key.
func (b *hookBuilder) readTable(r *reader, src hookSource, key string) (map[string]json.RawMessage, bool) {
	if _, ok := r.exists(src.file); !ok {
		return nil, false
	}
	data, err := r.read(src.file)
	if err != nil {
		b.p.add(newFinding(StatusDropped, src.file, "", "", skipReasonOr(err)))
		return nil, false
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(data), &doc); err != nil {
		b.p.add(newFinding(StatusUnsupported, src.file, "", "", "not valid JSON, so its hooks were not read: "+err.Error()))
		return nil, false
	}
	raw, ok := doc[key]
	if !ok {
		return nil, false
	}
	var table map[string]json.RawMessage
	if err := json.Unmarshal(raw, &table); err != nil {
		b.p.add(newFinding(StatusUnsupported, src.file, key, "", "hooks is not an object keyed by event"))
		return nil, false
	}
	return table, true
}

const copilotHooksDir = ".github/hooks"

// nativeSettingsFiles are the tool files read for hooks and permissions, listed
// by Detect so a project that has only those is still recognized.
var nativeSettingsFiles = []string{
	claudeSettingsFile, ".codex/hooks.json", ".cursor/hooks.json", cursorCLIFile, copilotHooksDir,
}

// copilotHooks reads the flat hook files of GitHub Copilot, except the one
// ai-rulez generates itself.
func (b *hookBuilder) copilotHooks(r *reader) {
	if _, ok := r.exists(copilotHooksDir); !ok {
		return
	}
	for _, rel := range r.walkFiles(copilotHooksDir, func(name, reason string) { b.p.add(newFinding(StatusDropped, name, "", "", reason)) }) {
		file := path.Join(copilotHooksDir, rel)
		if !strings.HasSuffix(rel, ".json") {
			continue
		}
		if rel == "ai-rulez.json" {
			b.p.add(newFinding(StatusDropped, file, "", "", "written by ai-rulez generate; not imported as source"))
			continue
		}
		src := hookSource{file: file, harness: config.HarnessCopilot, events: copilotHookEvents}
		if table, ok := b.readTable(r, src, litHooks); ok {
			b.fromEventTable(src, table, "hooks.")
		}
	}
}

// codexInline reports hooks declared inside .codex/config.toml, whose layout is
// not read: they are not in .codex/hooks.json, which is.
func (b *hookBuilder) codexInline(r *reader) {
	const file = ".codex/config.toml"
	if _, ok := r.exists(file); !ok {
		return
	}
	data, err := r.read(file)
	if err != nil {
		return
	}
	for _, line := range strings.Split(normalizeText(string(data)), "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "[hooks") || strings.HasPrefix(t, "[[hooks") {
			b.p.add(newFinding(StatusNeedsAction, file, litHooks, "",
				"hooks declared inline in config.toml are not imported; move them to .codex/hooks.json and rerun, or add [[hooks]] by hand"))
			return
		}
	}
}

// importRulesyncHooks reads .rulesync/hooks.jsonc: the shared `hooks` block and
// the per-tool override blocks.
func (b *rulesyncPlanner) importHooks(file string) {
	doc, ok := b.readJSONC(file)
	if !ok {
		return
	}
	hb := &hookBuilder{p: b.p}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case litSchema, litVersion:
		case litHooks:
			var table map[string]json.RawMessage
			if json.Unmarshal(doc[k], &table) != nil {
				b.p.add(newFinding(StatusUnsupported, file, k, "", "hooks is not an object keyed by event"))
				continue
			}
			hb.fromEventTable(hookSource{file: file, events: rulesyncHookEvents}, table, "hooks.")
		default:
			tool, known := rulesyncToolBlocks[k]
			var scoped struct {
				Hooks map[string]json.RawMessage `json:"hooks"`
			}
			if json.Unmarshal(doc[k], &scoped) != nil || len(scoped.Hooks) == 0 {
				b.p.add(newFinding(StatusDropped, file, k, "", "unrecognized key"))
				continue
			}
			if !known {
				b.p.add(newFinding(StatusUnsupported, file, k+".hooks", "",
					"hooks for "+k+" are not imported: ai-rulez renders hooks for claude, codex, cursor, gemini and copilot (and the settings-file harnesses of docs/settings.md) from one declaration"))
				continue
			}
			hb.fromEventTable(hookSource{file: file, harness: tool.harness, events: tool.events}, scoped.Hooks, k+".hooks.")
		}
	}
}

// mergeHookGroups removes groups that say the same thing and joins the targets
// of identical ones, so a hook found in two tools' files is declared once for
// both. The result is ordered by event, matcher and command.
func mergeHookGroups(in []config.HookGroup) []config.HookGroup {
	type slot struct {
		group config.HookGroup
		all   bool // some copy has no targets: it applies to every harness
	}
	order := []string{}
	byKey := map[string]*slot{}
	for _, g := range in {
		key := hookGroupKey(g)
		s, ok := byKey[key]
		if !ok {
			cp := g
			cp.Targets = nil
			byKey[key] = &slot{group: cp, all: len(g.Targets) == 0}
			s = byKey[key]
			order = append(order, key)
		}
		if len(g.Targets) == 0 {
			s.all = true
		}
		s.group.Targets = append(s.group.Targets, g.Targets...)
		if len(g.Matchers) > 0 {
			if s.group.Matchers == nil {
				s.group.Matchers = map[string]string{}
			}
			for h, m := range g.Matchers {
				s.group.Matchers[h] = m
			}
		}
	}
	out := make([]config.HookGroup, 0, len(order))
	for _, key := range order {
		s := byKey[key]
		if s.all {
			s.group.Targets = nil
		} else {
			sort.Strings(s.group.Targets)
			s.group.Targets = dedupeStrings(s.group.Targets)
		}
		out = append(out, s.group)
	}
	sort.SliceStable(out, func(i, j int) bool { return hookLess(out[i], out[j]) })
	return out
}

// hookGroupKey identifies a group by what it does, ignoring where it applies.
func hookGroupKey(g config.HookGroup) string {
	data, err := json.Marshal(struct {
		Event, Matcher string
		Hooks          []config.HookAction
		Matchers       map[string]string
	}{g.Event, g.Matcher, g.Hooks, g.Matchers})
	if err != nil {
		// Strings, ints and maps of strings always marshal; fall back to Go syntax.
		return fmt.Sprintf("%#v", g)
	}
	if len(g.Matchers) > 0 {
		// A harness-specific matcher is a different group even with the same handlers.
		return string(data) + "|" + strings.Join(g.Targets, ",")
	}
	return string(data)
}

func hookLess(a, b config.HookGroup) bool {
	if a.Event != b.Event {
		return a.Event < b.Event
	}
	if a.Matcher != b.Matcher {
		return a.Matcher < b.Matcher
	}
	return firstCommand(a) < firstCommand(b)
}

func firstCommand(g config.HookGroup) string {
	if len(g.Hooks) > 0 {
		return g.Hooks[0].Command
	}
	return ""
}
