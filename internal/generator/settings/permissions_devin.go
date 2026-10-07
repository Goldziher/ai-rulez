package settings

import (
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// Devin CLI: .devin/config.json (JSONC) `permissions.{allow,ask,deny}` with
// Read(glob), Write(glob), Exec(command prefix), Fetch(domain:host) and
// mcp__server__tool entries; deny > ask > allow. Exec matches complete words, so
// `Exec(git)` covers `git status` but not `gitk`. Wildcards inside a command have
// no equivalent.
//
// Source: https://docs.devin.ai/cli/reference/permissions and
// https://docs.devin.ai/cli/reference/configuration/config-file (read 2026-10-05).
var _ = registerPermissionDialect("devin", buildDevin)

func buildDevin(t *translation) ([]jsonmerge.OwnedKey, error) {
	lists := map[PermAction][]any{}
	for ix := range t.entries {
		e := t.entries[ix]
		rule, why := devinRule(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		lists[e.Action] = append(lists[e.Action], rule)
	}
	var keys []jsonmerge.OwnedKey
	for _, action := range []PermAction{ActionAllow, ActionAsk, ActionDeny} {
		if len(lists[action]) > 0 {
			keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{keyPermissions, string(action)}, lists[action]))
		}
	}
	return keys, nil
}

func devinRule(e permEntry) (rule, why string) {
	r := e.Rule
	switch r.Kind {
	case KindShell:
		p := r.Shell()
		switch {
		case p.Kind == ShellAny:
			return "exec", ""
		case p.Kind == ShellPrefix, p.Kind == ShellExact && e.Action != ActionAllow:
			return "Exec(" + p.Literal + ")", ""
		case p.Kind == ShellExact:
			return "", "Exec matches word prefixes, so an exact-command allow would be widened"
		}
		return "", "Exec rules are word prefixes; wildcards inside a command cannot be expressed"
	case KindRead, KindEdit:
		tool, bare := toolRead, ocRead
		if r.Kind == KindEdit {
			tool, bare = toolWrite, ocEdit
		}
		if r.Bare {
			return bare, ""
		}
		glob := r.Path.Glob
		switch r.Path.Anchor {
		case AnchorHome:
			glob = "~/" + glob
		case AnchorAbsolute:
			glob = "/" + glob // `//x` is Devin's absolute form
		}
		return tool + "(" + glob + ")", ""
	case KindFetch:
		if r.Domain == "" {
			return "", msgOnlyWebFetchDomain
		}
		return "Fetch(domain:" + r.Domain + ")", ""
	case KindMCP:
		if r.MCPTool == "" {
			return "mcp__" + r.Server + "__*", ""
		}
		return r.Raw, ""
	}
	return "", "the harness has no equivalent of " + r.Tool + " rules"
}

// Grok CLI: .grok/config.toml `[permission]` with `allow`, `ask` and `deny` arrays
// of Claude-style `Tool(pattern)` entries (filters Bash, Edit, Read, Grep, MCPTool,
// WebFetch, WebSearch); deny > ask > allow. Project config may carry only
// [mcp_servers], [plugins] and [permission].
//
// Source: https://docs.x.ai/build/settings/reference and
// https://docs.x.ai/build/features/permissions (read 2026-10-05).
var _ = registerPermissionDialect("grok", buildGrok)

func buildGrok(t *translation) ([]jsonmerge.OwnedKey, error) {
	lists := map[PermAction][]any{}
	for ix := range t.entries {
		e := t.entries[ix]
		rules, why := grokRules(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		for _, r := range rules {
			lists[e.Action] = append(lists[e.Action], r)
		}
	}
	var keys []jsonmerge.OwnedKey
	for _, action := range []PermAction{ActionAllow, ActionAsk, ActionDeny} {
		if len(lists[action]) > 0 {
			keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{keyPermission, string(action)}, lists[action]))
		}
	}
	return keys, nil
}

func grokRules(e permEntry) ([]string, string) {
	r := e.Rule
	switch r.Kind {
	case KindShell:
		p := r.Shell()
		switch p.Kind {
		case ShellAny:
			return []string{toolBash}, ""
		case ShellPrefix:
			return []string{"Bash(" + p.Literal + ")", "Bash(" + p.Literal + " *)"}, ""
		default:
			return []string{"Bash(" + p.Literal + ")"}, ""
		}
	case KindRead, KindEdit:
		tool := toolRead
		if r.Kind == KindEdit {
			tool = toolEdit
			if r.Tool != toolEdit && e.Action == ActionAllow {
				return nil, "Grok has one Edit filter for every edit tool, so a " + r.Tool + " allow rule would be widened"
			}
		}
		if r.Bare {
			return []string{tool}, ""
		}
		if r.Path.Anchor != AnchorCwd && r.Path.Anchor != AnchorProject {
			return nil, "home and absolute path anchors are not documented for Grok"
		}
		return []string{tool + "(" + r.Path.Glob + ")"}, ""
	case KindSearch:
		if r.Bare {
			return []string{toolWebSearch}, ""
		}
	case KindFetch:
		if r.Bare {
			return []string{toolWebFetch}, ""
		}
		return nil, "the WebFetch domain syntax is not documented for Grok"
	case KindMCP:
		if r.MCPTool == "" {
			return []string{"MCPTool(" + r.Server + "__*)"}, ""
		}
		return []string{"MCPTool(" + r.Server + "__" + r.MCPTool + ")"}, ""
	}
	return nil, "the harness has no equivalent of " + r.Tool + " rules"
}
