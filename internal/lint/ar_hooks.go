package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// CodeHookSchema reports hook declarations that load but will not behave as
// written. The event and matcher vocabulary comes from the same tables the
// config loader uses (config.KnownHookEvents and friends), so the two never
// disagree about what an event is.
const CodeHookSchema = "AR507"

func init() {
	registerRules(RuleInfo{CodeHookSchema, "hook-schema-invalid", SeverityWarning, "a hook declaration has an unknown event, a missing or unknown type, no command, url or prompt, an invalid timeout, or a matcher or `if` on an event that ignores it"})
	registerRunCheck(checkHookSchema)
}

// hookHandlerTypes are the handler types Claude Code documents.
var hookHandlerTypes = []string{hookTypeCommand, transportHTTP, hookTypePrompt, hookTypeAgent, "mcp_tool"}

// maxHookTimeout is the largest timeout, in seconds, that is plausible (a day).
const maxHookTimeout = 86400

// hookProblem is one finding about a hook declaration.
type hookProblem struct {
	kind, event, needle, msg string
}

func checkHookSchema(r *runner) {
	seen := map[string]bool{}
	if cfgPath := r.configFilePath(); cfgPath != "" && (len(r.cfg.Hooks) > 0 || (r.cfg.Plugin != nil && len(r.cfg.Plugin.Hooks) > 0)) {
		lines := r.fileLines(cfgPath)
		groups := append([]config.HookGroup(nil), r.cfg.Hooks...)
		if r.cfg.Plugin != nil {
			groups = append(groups, r.cfg.Plugin.Hooks...)
		}
		for _, p := range configHookProblems(groups) {
			seen[p.kind+"|"+p.event] = true
			r.add(CodeHookSchema, cfgPath, lineContaining(lines, p.needle), "%s", p.msg)
		}
	}
	for _, file := range r.hookJSONFiles() {
		data, err := readSmallFile(file)
		if err != nil {
			continue
		}
		var root map[string]json.RawMessage
		if json.Unmarshal(data, &root) != nil {
			continue
		}
		raw, ok := root[keyHooks]
		if !ok {
			continue
		}
		lines := r.fileLines(file)
		for _, p := range jsonHookProblems(raw) {
			if seen[p.kind+"|"+p.event] {
				continue
			}
			r.add(CodeHookSchema, file, lineContaining(lines, p.needle), "%s", p.msg)
		}
	}
}

// hookJSONFiles lists the JSON hook files: the project's .claude/settings.json
// and every tracked plugin hooks/hooks.json.
func (r *runner) hookJSONFiles() []string {
	var files []string
	if p := filepath.Join(r.rootAbs(), ".claude", "settings.json"); fileExists(p) {
		files = append(files, p)
	}
	for _, rel := range r.tree.Paths() {
		if path.Base(rel) == "hooks.json" && path.Base(path.Dir(rel)) == keyHooks {
			files = append(files, filepath.Join(r.tree.Top, filepath.FromSlash(rel)))
		}
	}
	sort.Strings(files)
	return files
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func quoteNeedle(s string) string { return `"` + s + `"` }

// eventProblems checks what depends on the event alone.
func eventProblems(event string, hasMatcher, hasIf bool) []hookProblem {
	var out []hookProblem
	known := slices.Contains(config.KnownHookEvents, event)
	if !known {
		hint := ""
		if near := closest(event, config.KnownHookEvents, 2); near != "" {
			hint = fmt.Sprintf("; did you mean %q?", near)
		}
		out = append(out, hookProblem{"event", event, quoteNeedle(event), fmt.Sprintf("hook event %q is not a Claude Code event, so the hook never fires%s", event, hint)})
	}
	if known && hasMatcher && slices.Contains(config.HookEventsWithoutMatcher, event) {
		out = append(out, hookProblem{"matcher", event, quoteNeedle(event), fmt.Sprintf("%s has no matchable subject, so its matcher is silently ignored", event)})
	}
	if known && hasIf && !slices.Contains(config.HookEventsEvaluatingIf, event) {
		out = append(out, hookProblem{"if", event, quoteNeedle(event), fmt.Sprintf("%s does not evaluate `if`, so a handler that sets it never runs", event)})
	}
	return out
}

func configHookProblems(groups []config.HookGroup) []hookProblem {
	var out []hookProblem
	for _, g := range groups {
		if g.Event == "" {
			continue // the loader rejects it
		}
		hasIf := false
		for _, a := range g.Hooks {
			hasIf = hasIf || a.If != ""
		}
		matcher := g.Matcher != "" || len(g.Matchers) > 0
		out = append(out, eventProblems(g.Event, matcher, hasIf)...)
		for _, a := range g.Hooks {
			if a.Type != "" && !slices.Contains(hookHandlerTypes, a.Type) {
				out = append(out, hookProblem{"type", g.Event, quoteNeedle(g.Event), fmt.Sprintf("%s hook has unknown type %q (use %s)", g.Event, a.Type, strings.Join(hookHandlerTypes, ", "))})
			}
			if a.Timeout < 0 || a.Timeout > maxHookTimeout {
				out = append(out, hookProblem{"timeout", g.Event, "timeout", fmt.Sprintf("%s hook timeout %d is not between 1 and %d seconds", g.Event, a.Timeout, maxHookTimeout)})
			}
		}
	}
	return out
}

func jsonHookProblems(raw json.RawMessage) []hookProblem {
	var byEvent map[string]json.RawMessage
	if json.Unmarshal(raw, &byEvent) != nil {
		return []hookProblem{{kindShape, "", `"hooks"`, `"hooks" must be an object keyed by event name`}}
	}
	events := make([]string, 0, len(byEvent))
	for e := range byEvent {
		events = append(events, e)
	}
	sort.Strings(events)
	var out []hookProblem
	for _, event := range events {
		var groups []map[string]json.RawMessage
		if json.Unmarshal(byEvent[event], &groups) != nil {
			out = append(out, hookProblem{kindShape, event, quoteNeedle(event), fmt.Sprintf("%s must be a list of matcher groups ({\"matcher\": ..., \"hooks\": [...]})", event)})
			continue
		}
		for _, g := range groups {
			out = append(out, jsonGroupProblems(event, g)...)
		}
	}
	return out
}

func jsonGroupProblems(event string, g map[string]json.RawMessage) []hookProblem {
	var out []hookProblem
	hasMatcher := false
	if m, ok := g["matcher"]; ok {
		var s string
		if json.Unmarshal(m, &s) != nil {
			out = append(out, hookProblem{"matcher-type", event, `"matcher"`, fmt.Sprintf("%s matcher must be a string", event)})
		} else {
			hasMatcher = s != "" && s != "*"
		}
	}
	var handlers []map[string]json.RawMessage
	if h, ok := g[keyHooks]; !ok || json.Unmarshal(h, &handlers) != nil {
		out = append(out, hookProblem{kindShape, event, quoteNeedle(event), fmt.Sprintf("a %s group needs a \"hooks\" list of handlers", event)})
	}
	hasIf := false
	for _, h := range handlers {
		_, hasIfKey := h["if"]
		hasIf = hasIf || hasIfKey
	}
	out = append(out, eventProblems(event, hasMatcher, hasIf)...)
	for _, h := range handlers {
		out = append(out, jsonHandlerProblems(event, h)...)
	}
	return out
}

func jsonString(h map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := h[key]
	if !ok {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", true
	}
	return s, true
}

func jsonHandlerProblems(event string, h map[string]json.RawMessage) []hookProblem {
	var out []hookProblem
	add := func(kind, needle, format string, args ...any) {
		out = append(out, hookProblem{kind, event, needle, fmt.Sprintf(format, args...)})
	}
	typ, hasType := jsonString(h, "type")
	needleOf := func(key string) string {
		if _, ok := h[key]; ok {
			return quoteNeedle(key)
		}
		return quoteNeedle(event)
	}
	switch {
	case !hasType || typ == "":
		add("type", quoteNeedle(event), "a %s handler has no \"type\" (use %s)", event, strings.Join(hookHandlerTypes, ", "))
	case !slices.Contains(hookHandlerTypes, typ):
		add("type", needleOf("type"), "a %s handler has unknown type %q (use %s)", event, typ, strings.Join(hookHandlerTypes, ", "))
	default:
		field := map[string]string{hookTypeCommand: hookTypeCommand, transportHTTP: "url", hookTypePrompt: hookTypePrompt, hookTypeAgent: hookTypePrompt, "mcp_tool": "tool"}[typ]
		if v, has := jsonString(h, field); !has || strings.TrimSpace(v) == "" {
			add("field", quoteNeedle(event), "a %s %s handler needs a non-empty %q", event, typ, field)
		}
	}
	if raw, ok := h["timeout"]; ok {
		var n float64
		if json.Unmarshal(raw, &n) != nil || n <= 0 || n != float64(int64(n)) || n > maxHookTimeout {
			add("timeout", `"timeout"`, "%s handler timeout %s must be a whole number of seconds between 1 and %d", event, strings.TrimSpace(string(raw)), maxHookTimeout)
		}
	}
	return out
}
