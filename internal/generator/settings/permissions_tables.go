package settings

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/toolnames"
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
var _ = registerPermissionDialect("vibe", buildVibe)

var vibeWholeTools = map[string][]string{
	toolnames.Read:      {toolReadFile},
	toolnames.Edit:      {toolWriteFile, ocEdit},
	toolnames.Write:     {toolWriteFile},
	toolnames.WebFetch:  {toolWebFetch},
	toolnames.WebSearch: {"web_search"},
}

func buildVibe(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	lists := map[string][]any{} // tools.<tool>.<list>
	never := map[string]bool{}
	for eIndex := range t.entries {
		e := &t.entries[eIndex]
		if e.Action == ActionAsk {
			continue
		}
		if why := vibeAdd(*e, lists, never); why != "" {
			t.drop(*e, why)
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
	list := "allowlist"
	if e.Action == ActionDeny {
		list = "denylist"
	}
	switch r.Kind {
	case KindShell:
		return vibeShellRule(e, list, lists, never)
	case KindRead, KindEdit, KindFetch, KindSearch:
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
	return "the harness has no equivalent of " + r.Tool + " rules"
}

// Poolside: .poolside/settings.yaml `tools.shell.{allow,deny}` (glob patterns, `*`
// matches any run of characters) and `paths.{allow,deny}` (project-relative
// globs; `write: true` makes an allow writable). There is no ask list: whatever is
// not allowed prompts. `tools.shell.disabled` switches the tool off.
//
// Source: https://docs.poolside.ai/tool-permissions and
// https://docs.poolside.ai/settings-file-reference (read 2026-10-05).
var _ = registerPermissionDialect("poolside", buildPoolside)

func buildPoolside(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	shell := map[PermAction][]any{}
	paths := map[PermAction][]any{}
	var keys []jsonmerge.OwnedKey
	keys = collectPoolsideRules(t, shell, paths)
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
var _ = registerPermissionDialect("omp", buildOmp)

var ompApproval = map[PermAction]string{ActionAllow: string(ActionAllow), ActionAsk: "prompt", ActionDeny: string(ActionDeny)}

var ompTools = map[string]string{toolnames.Bash: ocBash, toolnames.Read: ocRead, toolnames.Edit: ocEdit, toolnames.Write: toolWrite, toolnames.WebSearch: "web_search", "Task": "task"}

var ompName = regexp.MustCompile(`[^a-z0-9_]+`)

func buildOmp(t *translation) ([]jsonmerge.OwnedKey, error) {
	var patterns []any
	tools := map[string]PermAction{}
	// First match wins: deny, then ask, then allow.
	for _, action := range []PermAction{ActionDeny, ActionAsk, ActionAllow} {
		entries := t.only(action)
		for eIndex := range entries {
			e := &entries[eIndex]
			ps, tool, why := ompEntry(*e)
			if why != "" {
				t.drop(*e, why)
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
var _ = registerPermissionDialect("augment", buildAugment)

var augmentTools = map[string][]string{
	toolnames.Bash: {terminalTool}, toolnames.Read: {ocRead}, toolnames.Edit: {ocEdit, toolWrite}, "MultiEdit": {ocEdit},
	toolnames.Write: {toolWrite}, toolnames.WebFetch: {"web-fetch"}, toolnames.WebSearch: {"web-search"},
}

func buildAugment(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	var elements []any
	for _, action := range []PermAction{ActionDeny, ActionAllow} {
		entries := t.only(action)
		for eIndex := range entries {
			e := &entries[eIndex]
			els, why := augmentEntries(*e)
			if why != "" {
				t.drop(*e, why)
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

func augmentEntries(e permEntry) (value []any, reason string) {
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

func vibeShellRule(e permEntry, list string, lists map[string][]any, never map[string]bool) string {
	r := e.Rule

	p := r.Shell()
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

func collectPoolsideRules(t *translation, shell, paths map[PermAction][]any) []jsonmerge.OwnedKey {
	var keys []jsonmerge.OwnedKey
	for eIndex := range t.entries {
		e := &t.entries[eIndex]
		if e.Action == ActionAsk {
			continue
		}
		r := e.Rule
		switch r.Kind {
		case KindShell:
			keys = append(keys, poolsideShellRule(t, *e, shell)...)
		case KindRead, KindEdit:
			if r.Bare {
				t.drop(*e, "paths rules need a path")
				continue
			}
			if r.Path.Anchor != AnchorCwd && r.Path.Anchor != AnchorProject && !t.cfg.UserScope {
				t.drop(*e, "the shared project file takes project-relative paths only")
				continue
			}
			entry := map[string]any{"path": poolsidePath(r)}
			if e.Action == ActionAllow && r.Kind == KindEdit {
				entry[toolWrite] = true
			}
			paths[e.Action] = append(paths[e.Action], entry)
		default:
			t.drop(*e, "the harness has no documented equivalent of "+r.Tool+" rules")
		}
	}
	return keys
}

func poolsideShellRule(t *translation, e permEntry, shell map[PermAction][]any) []jsonmerge.OwnedKey {
	r := e.Rule
	var keys []jsonmerge.OwnedKey

	p := r.Shell()
	switch {
	case p.Kind == ShellAny && e.Action == ActionDeny:
		if key, ok := scalarKey(t, []string{keyTools, "shell", "disabled"}, true); ok {
			keys = append(keys, key)
		}
	case p.Kind == ShellAny:
		t.drop(e, "allowing every command is not documented")
	case p.Kind == ShellPrefix:
		shell[e.Action] = append(shell[e.Action], p.Literal, p.Literal+" *")
	default:
		shell[e.Action] = append(shell[e.Action], p.Literal)
	}
	return keys
}
