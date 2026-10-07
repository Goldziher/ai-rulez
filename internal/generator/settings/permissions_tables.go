package settings

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// Mistral Vibe: .vibe/config.toml `[tools.<tool>]` tables. A shell tool carries
// `allowlist` and `denylist` of command prefixes (a command matches when it equals
// the prefix or starts with the prefix and a space); file tools carry path globs
// that are matched against the absolute path, so a deny is written as `*/<glob>`.
// `permission = "never"` blocks a whole tool. A per-pattern ask has no
// equivalent. Setting a list REPLACES Vibe's shipped default for it.
//
// Source: github.com/mistralai/mistral-vibe README and vibe/core/tools (main,
// read 2026-10-05; no dated docs page exists).
var vibeWholeTools = map[string][]string{
	toolRead:      {nativeReadFile},
	toolEdit:      {nativeWriteFile, ocEdit},
	toolWrite:     {nativeWriteFile},
	toolWebFetch:  {nativeWebFetch},
	toolWebSearch: {"web_search"},
}

func buildVibe(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	lists := map[string][]any{} // tools.<tool>.<list>
	never := map[string]bool{}
	for ix := range t.entries {
		e := t.entries[ix]
		if e.Action == ActionAsk {
			continue
		}
		if why := vibeAdd(e, lists, never); why != "" {
			t.drop(e, why)
		}
	}
	var keys []jsonmerge.OwnedKey
	names := make([]string, 0, len(lists))
	for name := range lists {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tool, list, _ := strings.Cut(name, ".")
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{keyTools, tool, list}, lists[name]))
	}
	tools := make([]string, 0, len(never))
	for tool := range never {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		if key, ok := scalarKey(t, []string{keyTools, tool, keyPermission}, "never"); ok {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func vibeAdd(e permEntry, lists map[string][]any, never map[string]bool) string {
	r := e.Rule
	switch r.Kind {
	case KindShell:
		return vibeShell(e, lists, never)
	case KindRead, KindEdit, KindFetch, KindSearch:
		return vibeTool(e, lists, never)
	}
	return "the harness has no equivalent of " + r.Tool + " rules"
}

// vibeShell files a shell rule into the bash allow or deny list, or switches the
// tool off; the returned reason is empty when it was expressed.
func vibeShell(e permEntry, lists map[string][]any, never map[string]bool) string {
	list := "allowlist"
	if e.Action == ActionDeny {
		list = "denylist"
	}
	p := e.Rule.Shell()
	switch {
	case p.Kind == ShellAny && e.Action == ActionDeny:
		never[ocBash] = true
	case p.Kind == ShellPrefix, p.Kind == ShellExact && e.Action == ActionDeny:
		lists["bash."+list] = append(lists["bash."+list], p.Literal)
	case p.Kind == ShellExact:
		return "Vibe matches command prefixes, so an exact-command allow would be widened"
	default:
		return "Vibe shell lists hold command prefixes; wildcards and allow-all cannot be expressed"
	}
	return ""
}

// vibeTool files a file or web rule: Vibe only denies, and only by whole tool or
// absolute-path glob.
func vibeTool(e permEntry, lists map[string][]any, never map[string]bool) string {
	r := e.Rule
	tools := vibeWholeTools[r.Tool]
	if tools == nil {
		return "the harness has no equivalent of " + r.Tool + " rules"
	}
	if e.Action != ActionDeny {
		return "Vibe cannot scope an allow to a path or a domain, and allowing the whole tool would widen the rule"
	}
	if r.Bare {
		for _, tool := range tools {
			never[tool] = true
		}
		return ""
	}
	if r.Kind == KindFetch || r.Kind == KindSearch {
		return "Vibe's web tools have no domain list, only a whole-tool switch"
	}
	if r.Path.Anchor != AnchorCwd && r.Path.Anchor != AnchorProject {
		return "home and absolute path anchors are not expressed as the absolute-path globs Vibe matches"
	}
	for _, tool := range tools {
		lists[tool+".denylist"] = append(lists[tool+".denylist"], "*/"+r.Path.Glob)
	}
	return ""
}

// Poolside: .poolside/settings.yaml `tools.shell.{allow,deny}` (glob patterns, `*`
// matches any run of characters) and `paths.{allow,deny}` (project-relative
// globs; `write: true` makes an allow writable). There is no ask list: whatever is
// not allowed prompts. `tools.shell.disabled` switches the tool off.
//
// Source: https://docs.poolside.ai/tool-permissions and
// https://docs.poolside.ai/settings-file-reference (read 2026-10-05).
func buildPoolside(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	shell := map[PermAction][]any{}
	paths := map[PermAction][]any{}
	var keys []jsonmerge.OwnedKey
	for ix := range t.entries {
		e := t.entries[ix]
		if e.Action == ActionAsk {
			continue
		}
		r := e.Rule
		switch r.Kind {
		case KindShell:
			if key, ok := poolsideShell(t, e, shell); ok {
				keys = append(keys, key)
			}
		case KindRead, KindEdit:
			poolsidePathRule(t, e, paths)
		default:
			t.drop(e, "the harness has no documented equivalent of "+r.Tool+" rules")
		}
	}
	for _, action := range []PermAction{ActionAllow, ActionDeny} {
		if len(shell[action]) > 0 {
			keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{keyTools, "shell", string(action)}, dedupe(shell[action])))
		}
		if len(paths[action]) > 0 {
			keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{"paths", string(action)}, paths[action]))
		}
	}
	return keys, nil
}

// poolsideShell files a shell rule into the shell lists. Denying every command
// instead switches the tool off, which is the owned key it returns.
func poolsideShell(t *translation, e permEntry, shell map[PermAction][]any) (key jsonmerge.OwnedKey, ok bool) {
	p := e.Rule.Shell()
	switch {
	case p.Kind == ShellAny && e.Action == ActionDeny:
		return scalarKey(t, []string{keyTools, "shell", "disabled"}, true)
	case p.Kind == ShellAny:
		t.drop(e, "allowing every command is not documented")
	case p.Kind == ShellPrefix:
		shell[e.Action] = append(shell[e.Action], p.Literal, p.Literal+" *")
	default:
		shell[e.Action] = append(shell[e.Action], p.Literal)
	}
	return jsonmerge.OwnedKey{}, false
}

// poolsidePathRule files a Read or Edit rule into the paths lists.
func poolsidePathRule(t *translation, e permEntry, paths map[PermAction][]any) {
	r := e.Rule
	if r.Bare {
		t.drop(e, "paths rules need a path")
		return
	}
	if r.Path.Anchor != AnchorCwd && r.Path.Anchor != AnchorProject && !t.cfg.UserScope {
		t.drop(e, "the shared project file takes project-relative paths only")
		return
	}
	entry := map[string]any{"path": poolsidePath(r)}
	if e.Action == ActionAllow && r.Kind == KindEdit {
		entry[ocWrite] = true
	}
	paths[e.Action] = append(paths[e.Action], entry)
}

func poolsidePath(r Rule) string {
	switch r.Path.Anchor {
	case AnchorHome:
		return "~/" + r.Path.Glob
	case AnchorAbsolute:
		return r.Path.Glob
	}
	return r.Path.Glob
}

func dedupe(in []any) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		if !containsValue(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// oh-my-pi: .omp/config.yml `bash.patterns` (ordered, FIRST match wins:
// {match, approval: allow|prompt|deny}, literal text and `*`) and
// `tools.approval.<tool>` for a whole tool. There is no per-path or per-domain
// matcher, so path and domain rules are skipped. Rules are appended after the
// user's own, so a user pattern that matches first still wins.
//
// Source: github.com/can1357/oh-my-pi docs/settings.md and docs/approval-mode.md
// (main, read 2026-10-05).
var ompApproval = map[PermAction]string{ActionAllow: string(ActionAllow), ActionAsk: "prompt", ActionDeny: string(ActionDeny)}

var ompTools = map[string]string{toolBash: ocBash, toolRead: ocRead, toolEdit: ocEdit, toolWrite: ocWrite, toolWebSearch: "web_search", "Task": "task"}

var ompName = regexp.MustCompile(`[^a-z0-9_]+`)

func buildOmp(t *translation) ([]jsonmerge.OwnedKey, error) {
	var patterns []any
	tools := map[string]PermAction{}
	// First match wins: deny, then ask, then allow.
	for _, action := range []PermAction{ActionDeny, ActionAsk, ActionAllow} {
		ranged := t.only(action)
		for ix := range ranged {
			e := ranged[ix]
			ps, tool, why := ompEntry(e)
			if why != "" {
				t.drop(e, why)
				continue
			}
			for _, p := range ps {
				patterns = append(patterns, map[string]any{"match": p, "approval": ompApproval[action]})
			}
			if tool != "" {
				if prev, ok := tools[tool]; !ok || strictness(action) > strictness(prev) {
					tools[tool] = action
				}
			}
		}
	}
	var keys []jsonmerge.OwnedKey
	if len(patterns) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{ocBash, "patterns"}, dedupe(patterns)))
	}
	if len(tools) > 0 {
		values := make(map[string]any, len(tools))
		for tool, action := range tools {
			values[tool] = ompApproval[action]
		}
		if key, ok := docMembersKey(t, []string{keyTools, "approval"}, values); ok {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func ompEntry(e permEntry) (patterns []string, tool, why string) {
	r := e.Rule
	if r.Kind == KindShell && !r.Bare {
		p := r.Shell()
		switch p.Kind {
		case ShellPrefix:
			return []string{p.Literal, p.Literal + " *"}, "", ""
		case ShellAny:
			return nil, ocBash, ""
		default:
			return []string{p.Literal}, "", ""
		}
	}
	if r.Kind == KindMCP {
		if r.MCPTool == "" {
			return nil, "", "oh-my-pi names MCP tools one by one; a whole-server rule cannot be expressed"
		}
		return nil, "mcp__" + ompName.ReplaceAllString(strings.ToLower(r.Server), "_") + "_" +
			ompName.ReplaceAllString(strings.ToLower(r.MCPTool), "_"), ""
	}
	name, ok := ompTools[r.Tool]
	if !ok {
		return nil, "", "the harness has no equivalent of " + r.Tool + " rules"
	}
	if !r.Bare {
		return nil, "", "oh-my-pi approves a whole tool; it has no per-path or per-domain matcher"
	}
	return nil, name, ""
}

// Augment Code (Auggie): .augment/settings.json `toolPermissions`, an ordered array
// of {toolName, shellInputRegex?, permission: {type}}. Only allow and deny exist
// (type "ask-user" is not documented); a shell rule is a regex over the command.
// There is no path or domain matcher.
//
// Source: https://docs.augmentcode.com/cli/permissions (read 2026-10-05).
var augmentTools = map[string][]string{
	toolBash: {keyTerminal}, toolRead: {ocRead}, toolEdit: {ocEdit, ocWrite}, "MultiEdit": {ocEdit},
	toolWrite: {ocWrite}, toolWebFetch: {"web-fetch"}, toolWebSearch: {"web-search"},
}

func buildAugment(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	var elements []any
	for _, action := range []PermAction{ActionDeny, ActionAllow} {
		ranged := t.only(action)
		for ix := range ranged {
			e := ranged[ix]
			els, why := augmentEntries(e)
			if why != "" {
				t.drop(e, why)
				continue
			}
			elements = append(elements, els...)
		}
	}
	if len(elements) == 0 {
		return nil, nil
	}
	return []jsonmerge.OwnedKey{docArrayKey(t.cfg, t.docPath, []string{"toolPermissions"}, dedupe(elements))}, nil
}

func augmentEntries(e permEntry) (elements []any, why string) {
	r := e.Rule
	names, ok := augmentTools[r.Tool]
	if !ok {
		return nil, "the harness has no equivalent of " + r.Tool + " rules"
	}
	perm := map[string]any{"type": string(e.Action)}
	if r.Kind == KindShell && !r.Bare {
		p := r.Shell()
		var re string
		switch {
		case p.Kind == ShellAny:
		case e.Action == ActionAllow:
			re = allowShellRegex(p)
		default:
			re = denyShellRegex(p, false)
		}
		el := map[string]any{"toolName": names[0], keyPermission: perm}
		if re != "" {
			el["shellInputRegex"] = re
		}
		return []any{el}, ""
	}
	if !r.Bare {
		return nil, "Augment cannot scope a rule to a path or a domain"
	}
	var out []any
	for _, n := range names {
		out = append(out, map[string]any{"toolName": n, keyPermission: perm})
	}
	return out, ""
}
