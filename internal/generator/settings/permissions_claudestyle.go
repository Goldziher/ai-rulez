package settings

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// Harnesses whose settings.json carries Claude Code's permission rule syntax:
// {"permissions": {"allow": [...], "ask": [...], "deny": [...]}}. They differ in
// the shell tool's name, how a path is anchored, and which rule kinds exist.
//
// Sources (vendor documentation, read 2026-10-05):
//   - codebuddy:   https://www.codebuddy.ai/docs/cli/permissions
//   - commandcode: https://commandcode.ai/docs/permissions
//   - qoder:       https://docs.qoder.com/cli/permissions
//   - qwen:        https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/
//   - letta:       https://docs.letta.com/letta-code/permissions and the
//     letta-ai/letta-code permissions loader (alwaysAsk is documented in source only)
type claudeStyle struct {
	// shell is the name of the Bash tool in this harness.
	shell string
	// askKey is the list ask rules go to (letta: alwaysAsk).
	askKey string
	// kinds are the rule kinds the harness can express.
	kinds map[ToolKind]bool
	// projectPaths rewrites a working-directory path to a project-root path
	// (`/x`), for harnesses whose bare paths match at any depth.
	projectPaths bool
	// editOnly renders Write, MultiEdit and NotebookEdit rules as Edit rules.
	editOnly bool
	// agentTool is the name of the subagent tool, empty when unsupported.
	agentTool string
}

var (
	_ = registerPermissionDialect("codebuddy", claudeStyle{
		shell: toolBash, askKey: string(ActionAsk),
		kinds: kindSet(KindShell, KindRead, KindEdit, KindFetch, KindSearch, KindMCP),
	}.build)
	_ = registerPermissionDialect("commandcode", claudeStyle{
		shell: "Shell", askKey: string(ActionAsk), projectPaths: true,
		kinds: kindSet(KindShell, KindRead, KindEdit, KindFetch, KindSearch, KindMCP),
	}.build)
	_ = registerPermissionDialect("qoder", claudeStyle{
		shell: toolBash, askKey: string(ActionAsk), projectPaths: true, editOnly: true, agentTool: toolAgent,
		kinds: kindSet(KindShell, KindRead, KindEdit, KindMCP, KindAgent),
	}.build)
	_ = registerPermissionDialect("qwen", claudeStyle{
		shell: toolBash, askKey: string(ActionAsk), agentTool: toolAgent,
		kinds: kindSet(KindShell, KindRead, KindEdit, KindFetch, KindSearch, KindMCP, KindAgent),
	}.build)
	_ = registerPermissionDialect("letta", claudeStyle{
		shell: toolBash, askKey: "alwaysAsk",
		kinds: kindSet(KindShell, KindRead, KindEdit),
	}.build)
)

func kindSet(kinds ...ToolKind) map[ToolKind]bool {
	m := make(map[ToolKind]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

func (s claudeStyle) build(t *translation) ([]jsonmerge.OwnedKey, error) {
	lists := map[PermAction][]string{}
	for _, e := range t.entries {
		out, why := s.render(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		lists[e.Action] = append(lists[e.Action], out)
	}
	keyOf := map[PermAction]string{ActionAllow: string(ActionAllow), ActionAsk: s.askKey, ActionDeny: string(ActionDeny)}
	var keys []jsonmerge.OwnedKey
	for _, action := range []PermAction{ActionAllow, ActionAsk, ActionDeny} {
		if len(lists[action]) == 0 {
			continue
		}
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{keyPermissions, keyOf[action]}, stringElements(lists[action])))
	}
	return keys, nil
}

// render writes one rule in the harness's syntax; a non-empty reason means the
// rule cannot be expressed.
func (s claudeStyle) render(e permEntry) (rule, reason string) {
	r := e.Rule
	if !s.kinds[r.Kind] {
		return "", "the harness has no equivalent of " + r.Tool + " rules"
	}
	switch r.Kind {
	case KindMCP:
		return r.Raw, ""
	case KindShell:
		if r.Bare {
			return s.shell, ""
		}
		return s.shell + "(" + r.Specifier + ")", ""
	case KindAgent:
		if r.Bare {
			return s.agentTool, ""
		}
		return s.agentTool + "(" + r.Specifier + ")", ""
	case KindRead, KindEdit:
		return s.renderPath(e)
	case KindFetch:
		if !r.Bare && r.Domain == "" {
			return "", "only WebFetch(domain:...) rules carry over"
		}
	}
	return r.Raw, ""
}

func (s claudeStyle) renderPath(e permEntry) (rule, reason string) {
	r := e.Rule
	tool := r.Tool
	if s.editOnly && r.Kind == KindEdit && tool != toolEdit {
		if e.Action == ActionAllow {
			return "", "Edit rules also cover Write, so a Write allow rule would be widened"
		}
		tool = toolEdit
	}
	if r.Bare {
		return tool, ""
	}
	spec := r.Specifier
	if s.projectPaths && (r.Path.Anchor == AnchorCwd || r.Path.Anchor == AnchorProject) {
		spec = "/" + strings.TrimPrefix(r.Path.Glob, "/")
	}
	return tool + "(" + spec + ")", ""
}
