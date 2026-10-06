package settings

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// VS Code (GitHub Copilot agent) and Zoo Code share .vscode/settings.json, so one
// translator renders the keys of whichever of the two presets is configured and
// both presets produce the same document.
//
// Copilot: chat.tools.terminal.autoApprove (prefix keys, true auto-approves,
// false ALWAYS requires approval), chat.tools.edits.autoApprove (workspace globs)
// and chat.tools.urls.autoApprove. A false value only forces a prompt; VS Code has
// no hard deny, so a deny rule is written as false and reported as not enforced.
// There is no read gate and no MCP key among these settings.
//
// Zoo Code: zoo-code.allowedCommands / zoo-code.deniedCommands (case-insensitive
// prefixes, `*` for any command; the longest matching prefix wins and a tie goes
// to deny). Anything in neither list asks.
//
// Sources (read 2026-10-05):
//   - https://code.visualstudio.com/docs/agents/run/approvals
//   - https://code.visualstudio.com/docs/chat/review-code-edits
//   - Zoo-Code-Org/Zoo-Code src/package.json and src/core/auto-approval/commands.ts
var (
	_ = registerPermissionDialect(config.HarnessCopilot, buildVSCode)
	_ = registerPermissionDialect(harnessZoocode, buildVSCode)
)

// harnessZoocode is the preset name of Zoo Code.
const harnessZoocode = "zoocode"

const (
	vscTerminal = "chat.tools.terminal.autoApprove"
	vscEdits    = "chat.tools.edits.autoApprove"
	vscURLs     = "chat.tools.urls.autoApprove"
	zooAllowed  = "zoo-code.allowedCommands"
	zooDenied   = "zoo-code.deniedCommands"
)

func hasPreset(cfg *config.Config, name string) bool {
	for _, p := range cfg.Presets {
		if p.GetName() == name {
			return true
		}
	}
	return false
}

func buildVSCode(t *translation) ([]jsonmerge.OwnedKey, error) {
	var keys []jsonmerge.OwnedKey
	if hasPreset(t.cfg, config.HarnessCopilot) {
		keys = append(keys, vscodeCopilot(t)...)
	}
	if hasPreset(t.cfg, harnessZoocode) {
		keys = append(keys, vscodeZoo(t)...)
	}
	return keys, nil
}

func vscodeCopilot(t *translation) []jsonmerge.OwnedKey {
	t.harness = config.HarnessCopilot
	maps := map[string]map[string]any{vscTerminal: {}, vscEdits: {}, vscURLs: {}}
	for _, e := range t.entries {
		target, patterns, why := vscodeEntry(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		approve := e.Action == ActionAllow
		if e.Action == ActionDeny {
			rulefilesDenyOnlyPrompts(t, e)
		}
		for _, p := range patterns {
			if prev, ok := maps[target][p]; ok && prev == false {
				continue // false always wins
			}
			maps[target][p] = approve
		}
	}
	return membersKeys(t, maps)
}

// rulefilesDenyOnlyPrompts reports that VS Code cannot block, only prompt.
func rulefilesDenyOnlyPrompts(t *translation, e permEntry) {
	t.dropRaw(ActionDeny, e.Rule.Raw, "VS Code has no hard deny; the rule is written as 'always ask' (false), which a user can still approve")
}

func membersKeys(t *translation, maps map[string]map[string]any) []jsonmerge.OwnedKey {
	names := make([]string, 0, len(maps))
	for name := range maps {
		names = append(names, name)
	}
	sort.Strings(names)
	var keys []jsonmerge.OwnedKey
	for _, name := range names {
		if len(maps[name]) == 0 {
			continue
		}
		if key, ok := docMembersKey(t, []string{name}, maps[name]); ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func vscodeEntry(e permEntry) (target string, patterns []string, why string) {
	r := e.Rule
	switch r.Kind {
	case KindShell:
		p := r.Shell()
		switch {
		case p.Kind == ShellAny:
			return vscTerminal, []string{"/.*/"}, ""
		case p.Kind == ShellPrefix && e.Action == ActionAllow:
			// A plain key is a prefix whose word boundary VS Code does not document, so
			// `git` could approve `gitk`; an anchored regular expression cannot.
			return vscTerminal, []string{"/^" + strings.ReplaceAll(regexp.QuoteMeta(p.Literal), "/", `\/`) + `(\s|$)/`}, ""
		case p.Kind == ShellPrefix, p.Kind == ShellExact && e.Action != ActionAllow:
			return vscTerminal, []string{p.Literal}, ""
		case p.Kind == ShellExact:
			return "", nil, "VS Code matches command prefixes, so an exact-command allow would be widened"
		}
		return "", nil, "VS Code terminal rules are prefixes or regular expressions; a command glob is not translated"
	case KindEdit:
		if r.Tool != "Edit" && e.Action == ActionAllow {
			return "", nil, "the edits setting covers every edit tool, so a " + r.Tool + " allow rule would be widened"
		}
		if r.Bare {
			return vscEdits, []string{"**/*"}, ""
		}
		if r.Path.Anchor != AnchorCwd && r.Path.Anchor != AnchorProject {
			return "", nil, "home and absolute path anchors cannot be written as workspace globs"
		}
		return vscEdits, []string{"**/" + strings.TrimPrefix(r.Path.Glob, "**/")}, ""
	case KindFetch:
		if r.Bare {
			return vscURLs, []string{"*"}, ""
		}
		if r.Domain == "" || strings.ContainsAny(r.Domain, "*?/") {
			return "", nil, "only WebFetch(domain:host) rules carry over"
		}
		return vscURLs, []string{"https://" + r.Domain, "https://" + r.Domain + "/*"}, ""
	}
	return "", nil, "VS Code has no setting for " + r.Tool + " rules"
}

func vscodeZoo(t *translation) []jsonmerge.OwnedKey {
	t.harness = harnessZoocode
	t.askUnsupported()
	var allowed, denied []string
	for _, e := range t.entries {
		if e.Action == ActionAsk {
			continue
		}
		prefix, why := zooPrefix(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		if e.Action == ActionAllow {
			allowed = append(allowed, prefix)
		} else {
			denied = append(denied, prefix)
		}
	}
	// The longest matching prefix wins and a deny only wins a tie or a longer
	// match: an allow with a deny prefix of it would override the deny.
	kept := allowed[:0:0]
	for _, a := range allowed {
		blocked := ""
		for _, d := range denied {
			// The allow is written with a trailing space (below), so even an allow equal
			// to a deny prefix is the longer match and would override it.
			if d == "*" || strings.HasPrefix(strings.ToLower(a)+" ", strings.ToLower(d)) && len(a)+1 > len(d) {
				blocked = d
				break
			}
		}
		if blocked != "" && blocked != "*" {
			t.dropRaw(ActionAllow, a, "Zoo Code lets the longer allow prefix override the shorter deny prefix "+blocked)
			continue
		}
		kept = append(kept, a)
	}
	// Zoo Code matches with a plain startsWith, so `git` would also approve `gitk`:
	// an allow prefix ends at a word boundary.
	for i, a := range kept {
		if a != "*" {
			kept[i] = a + " "
		}
	}
	var keys []jsonmerge.OwnedKey
	if len(kept) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{zooAllowed}, stringElements(kept)))
	}
	if len(denied) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{zooDenied}, stringElements(denied)))
	}
	return keys
}

func zooPrefix(e permEntry) (string, string) {
	r := e.Rule
	if r.Kind != KindShell {
		return "", "Zoo Code only has command allow and deny lists"
	}
	p := r.Shell()
	switch {
	case p.Kind == ShellAny:
		return "*", ""
	case p.Kind == ShellPrefix, p.Kind == ShellExact && e.Action == ActionDeny:
		return p.Literal, ""
	case p.Kind == ShellExact:
		return "", "Zoo Code matches command prefixes, so an exact-command allow would be widened"
	}
	return "", "Zoo Code lists hold command prefixes; wildcards inside a command cannot be expressed"
}

// GitHub Copilot CLI: .github/copilot/settings.json (JSONC) `allowedUrls` and
// `deniedUrls`. Tool permissions (shell, read, write, MCP) exist only as
// per-session flags and in a CLI-managed per-user file, so only URL rules can be
// written to the repository; deny wins over allow.
//
// Source: https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference
// and .../cli-command-reference (read 2026-10-05).
var _ = registerPermissionDialect("copilot-cli", buildCopilotCLI)

func buildCopilotCLI(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	var allowed, denied []any
	for _, e := range t.entries {
		if e.Action == ActionAsk {
			continue
		}
		r := e.Rule
		if r.Kind != KindFetch || r.Domain == "" {
			t.drop(e, "the Copilot CLI has no repository-level setting for "+r.Tool+" rules (tool permissions are per-session flags)")
			continue
		}
		if e.Action == ActionAllow {
			allowed = append(allowed, r.Domain)
		} else {
			denied = append(denied, r.Domain)
		}
	}
	var keys []jsonmerge.OwnedKey
	if len(allowed) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{"allowedUrls"}, allowed))
	}
	if len(denied) > 0 {
		keys = append(keys, docArrayKey(t.cfg, t.docPath, []string{"deniedUrls"}, denied))
	}
	return keys, nil
}

// Cursor CLI: .cursor/cli.json `permissions.{allow,deny}` with Shell(command),
// Read(glob), Write(glob), WebFetch(domain) and Mcp(server:tool); deny wins over
// allow and there is no ask (anything unmatched prompts). Shell(git) covers every
// git subcommand; `command:args` takes an argument glob.
//
// Source: https://cursor.com/docs/cli/reference/permissions (read 2026-10-05).
var _ = registerPermissionDialect(config.HarnessCursor, buildCursor)

func buildCursor(t *translation) ([]jsonmerge.OwnedKey, error) {
	t.askUnsupported()
	allow, deny := []any{}, []any{}
	for _, e := range t.entries {
		if e.Action == ActionAsk {
			continue
		}
		rule, why := cursorRule(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		if e.Action == ActionAllow {
			allow = append(allow, rule)
		} else {
			deny = append(deny, rule)
		}
	}
	if len(allow)+len(deny) == 0 {
		return nil, nil
	}
	// An array with nothing to say is not created (and never claimed), so a
	// deny-only config leaves no empty allow behind; one the document already
	// holds, or that an earlier run wrote elements to, is still maintained.
	var keys []jsonmerge.OwnedKey
	for _, arr := range []struct {
		name string
		ours []any
	}{{"allow", allow}, {"deny", deny}} {
		if key, ok := docArrayKeyIfNeeded(t.cfg, t.docPath, []string{"permissions", arr.name}, arr.ours); ok {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func cursorRule(e permEntry) (string, string) {
	r := e.Rule
	switch r.Kind {
	case KindShell:
		p := r.Shell()
		words := strings.Fields(p.Literal)
		switch {
		case p.Kind == ShellAny && e.Action == ActionDeny:
			return "Shell(*)", ""
		case p.Kind == ShellAny:
			return "", "allowing every command is not documented for Cursor"
		case p.Kind == ShellPrefix && len(words) == 1:
			return "Shell(" + words[0] + ")", ""
		case p.Kind == ShellPrefix && e.Action == ActionDeny:
			return "Shell(" + words[0] + ":" + strings.Join(words[1:], " ") + "*)", ""
		case p.Kind == ShellExact && len(words) == 1 && e.Action == ActionDeny:
			return "Shell(" + words[0] + ")", ""
		}
		return "", "Cursor's Shell() rules take a command name; a multi-word, exact or wildcard allow cannot be expressed without widening it"
	case KindRead, KindEdit:
		tool := "Read"
		if r.Kind == KindEdit {
			tool = "Write"
		}
		if r.Bare {
			return tool + "(**)", ""
		}
		glob := r.Path.Glob
		switch r.Path.Anchor {
		case AnchorHome:
			glob = "~/" + glob
		case AnchorAbsolute:
		}
		return tool + "(" + glob + ")", ""
	case KindFetch:
		if r.Bare {
			return "WebFetch(*)", ""
		}
		if r.Domain == "" {
			return "", "only WebFetch(domain:host) rules carry over"
		}
		return "WebFetch(" + r.Domain + ")", ""
	case KindMCP:
		if r.MCPTool == "" {
			return "Mcp(" + r.Server + ":*)", ""
		}
		return "Mcp(" + r.Server + ":" + r.MCPTool + ")", ""
	}
	return "", "the harness has no equivalent of " + r.Tool + " rules"
}

// Zed: agent.tool_permissions in the user settings.json (JSONC). Patterns are Rust
// regular expressions, unanchored unless written with ^ and $; precedence is
// always_deny > always_confirm > always_allow > default. Read tools are not
// permission-gated. Whether a project .zed/settings.json is honored for this key
// is not documented, so ai-rulez writes it to the user settings only.
//
// Source: https://zed.dev/docs/ai/tool-permissions (read 2026-10-05).
var _ = registerPermissionDialect("zed", buildZed)

var zedLists = map[PermAction]string{ActionAllow: "always_allow", ActionAsk: "always_confirm", ActionDeny: "always_deny"}

var zedDefaults = map[PermAction]string{ActionAllow: "allow", ActionAsk: "confirm", ActionDeny: "deny"}

func buildZed(t *translation) ([]jsonmerge.OwnedKey, error) {
	base := []string{"agent", "tool_permissions", "tools"}
	lists := map[string][]any{}
	var keys []jsonmerge.OwnedKey
	for _, e := range t.entries {
		tool, pattern, def, why := zedEntry(e)
		switch {
		case why != "":
			t.drop(e, why)
		case def != "":
			if key, ok := scalarKey(t, append(append([]string{}, base...), tool, "default"), def); ok {
				keys = append(keys, key)
			}
		default:
			name := tool + "." + zedLists[e.Action]
			element := map[string]any{"pattern": pattern}
			if e.Action == ActionAllow {
				element["case_sensitive"] = true // Zed matches case-insensitively by default
			}
			lists[name] = append(lists[name], element)
		}
	}
	names := make([]string, 0, len(lists))
	for n := range lists {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		tool, list, _ := strings.Cut(n, ".")
		keys = append(keys, docArrayKey(t.cfg, t.docPath, append(append([]string{}, base...), tool, list), dedupe(lists[n])))
	}
	return keys, nil
}

func zedEntry(e permEntry) (tool, pattern, def, why string) {
	r := e.Rule
	switch r.Kind {
	case KindShell:
		if r.Bare {
			return "terminal", "", zedDefaults[e.Action], ""
		}
		p := r.Shell()
		if p.Kind == ShellAny {
			return "terminal", "", zedDefaults[e.Action], ""
		}
		// Zed also tests each chained sub-command, so a deny prefix stays anchored.
		if e.Action == ActionAllow {
			return "terminal", allowShellRegex(p), "", ""
		}
		return "terminal", denyShellRegex(p, true), "", ""
	case KindEdit:
		if r.Bare {
			return "edit_file", "", zedDefaults[e.Action], ""
		}
		if e.Action == ActionAllow {
			return "", "", "", "Zed matches a regular expression against a path whose form is not documented, so a path allow would risk matching more than intended"
		}
		return "edit_file", pathRegex(r.Path.Glob), "", ""
	case KindFetch:
		if r.Bare {
			return "fetch", "", zedDefaults[e.Action], ""
		}
		if r.Domain == "" || strings.ContainsAny(r.Domain, "*?/") {
			return "", "", "", "only WebFetch(domain:host) rules carry over"
		}
		return "fetch", `^https?://` + regexp.QuoteMeta(r.Domain) + `(:\d+)?(/|$)`, "", ""
	case KindMCP:
		if r.MCPTool == "" {
			return "", "", "", "Zed sets MCP permissions per tool, not per server"
		}
		return "mcp:" + r.Server + ":" + r.MCPTool, "", zedDefaults[e.Action], ""
	}
	return "", "", "", "Zed does not gate " + r.Tool + " (read tools are not permission-checked)"
}

// pathRegex turns a path glob into an unanchored regular expression that matches
// the glob at a path-segment boundary.
func pathRegex(glob string) string {
	var b strings.Builder
	b.WriteString(`(^|/)`)
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return b.String()
}
