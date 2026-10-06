package importer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Permission rules are read as Claude Code permission rules, the syntax of
// [permissions]: .claude/settings.json already is, rulesync's
// `permission.<tool>.<pattern> = action` and Cursor's `Shell(...)` rules are
// rewritten. A rule that cannot be expressed is reported, never approximated into
// a broader one.

const (
	actionAllow = "allow"
	actionAsk   = "ask"
	actionDeny  = "deny"

	claudeSettingsFile = ".claude/settings.json"
	cursorCLIFile      = ".cursor/cli.json"
)

// permissionBuilder collects rules into p.Permissions.
type permissionBuilder struct{ p *Plan }

func (b *permissionBuilder) add(action, rule string) {
	switch action {
	case actionAllow:
		b.p.Permissions.Allow = append(b.p.Permissions.Allow, rule)
	case actionAsk:
		b.p.Permissions.Ask = append(b.p.Permissions.Ask, rule)
	case actionDeny:
		b.p.Permissions.Deny = append(b.p.Permissions.Deny, rule)
	}
}

// claudeSettingsPermissions are the keys of settings.json `permissions` that
// have no [permissions] equivalent.
var claudeSettingsPermissionExtras = map[string]string{
	"defaultMode":                  "the default permission mode has no ai-rulez setting",
	"additionalDirectories":        "extra working directories have no ai-rulez setting",
	"disableBypassPermissionsMode": "bypass-mode switches have no ai-rulez setting",
}

// importClaudeSettings reads hooks and permissions from .claude/settings.json
// and reports its other keys.
func importClaudeSettings(p *Plan, r *reader) {
	if _, ok := r.exists(claudeSettingsFile); !ok {
		return
	}
	data, err := r.read(claudeSettingsFile)
	if err != nil {
		p.add(newFinding(StatusDropped, claudeSettingsFile, "", "", skipReasonOr(err)))
		return
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(data), &doc); err != nil {
		p.add(newFinding(StatusDropped, claudeSettingsFile, "", "", "not valid JSON, so its hooks and permissions were not read: "+err.Error()))
		return
	}
	if raw, ok := doc["permissions"]; ok {
		(&permissionBuilder{p: p}).claudeSettings(raw)
	}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "hooks", "permissions", "$schema":
		default:
			p.add(newFinding(StatusDropped, claudeSettingsFile, k, "", "settings key is not imported; set it by hand (env and skillOverrides can live in [claude.settings.managed])"))
		}
	}
}

func (b *permissionBuilder) claudeSettings(raw json.RawMessage) {
	var perms map[string]json.RawMessage
	if err := json.Unmarshal(raw, &perms); err != nil {
		b.p.add(newFinding(StatusUnsupported, claudeSettingsFile, "permissions", "", "permissions is not an object"))
		return
	}
	keys := make([]string, 0, len(perms))
	for k := range perms {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case actionAllow, actionAsk, actionDeny:
			var rules []string
			if err := json.Unmarshal(perms[k], &rules); err != nil {
				b.p.add(newFinding(StatusUnsupported, claudeSettingsFile, "permissions."+k, "", "not a list of strings"))
				continue
			}
			for _, rule := range rules {
				if rule = strings.TrimSpace(rule); rule != "" {
					b.add(k, rule)
				}
			}
			b.p.add(newFinding(StatusMapped, claudeSettingsFile, "permissions."+k, "permissions."+k, ""))
		default:
			reason := claudeSettingsPermissionExtras[k]
			if reason == "" {
				reason = "permissions key has no ai-rulez equivalent"
			}
			b.p.add(newFinding(StatusDropped, claudeSettingsFile, "permissions."+k, "", reason))
		}
	}
}

// reportUnmappedPermissions lists the permission files whose rules are not
// imported: Gemini's tool lists and Codex's Starlark rules do not translate back
// to Claude Code rules without guessing.
func reportUnmappedPermissions(p *Plan, r *reader) {
	const geminiFile = ".gemini/settings.json"
	if _, ok := r.exists(geminiFile); ok {
		if data, err := r.read(geminiFile); err == nil {
			var doc struct {
				Tools struct {
					Allowed []string `json:"allowed"`
					Exclude []string `json:"exclude"`
				} `json:"tools"`
			}
			if json.Unmarshal(stripJSONC(data), &doc) == nil && len(doc.Tools.Allowed)+len(doc.Tools.Exclude) > 0 {
				p.add(newFinding(StatusUnsupported, geminiFile, "tools", "",
					"Gemini tool allow and exclude lists are not translated back to permission rules; add them to [permissions] by hand"))
			}
		}
	}
	const codexRules = ".codex/rules"
	if _, ok := r.exists(codexRules); ok {
		p.add(newFinding(StatusUnsupported, codexRules, "", "",
			"Codex Starlark rules are not translated back to permission rules; add them to [permissions] by hand"))
	}
}

// cursorToolNames maps the tool names of Cursor's permission rules to Claude Code's.
var cursorToolNames = map[string]string{"Shell": "Bash", "Read": "Read", "Write": "Edit", "WebFetch": "WebFetch"}

// importCursorPermissions reads .cursor/cli.json permissions.
func importCursorPermissions(p *Plan, r *reader) {
	if _, ok := r.exists(cursorCLIFile); !ok {
		return
	}
	data, err := r.read(cursorCLIFile)
	if err != nil {
		p.add(newFinding(StatusDropped, cursorCLIFile, "", "", skipReasonOr(err)))
		return
	}
	var doc struct {
		Permissions map[string][]string `json:"permissions"`
	}
	if err := json.Unmarshal(stripJSONC(data), &doc); err != nil {
		p.add(newFinding(StatusUnsupported, cursorCLIFile, "", "", "not valid JSON or permissions is not {allow, deny} lists: "+err.Error()))
		return
	}
	b := &permissionBuilder{p: p}
	actions := make([]string, 0, len(doc.Permissions))
	for a := range doc.Permissions {
		actions = append(actions, a)
	}
	sort.Strings(actions)
	for _, action := range actions {
		if action != actionAllow && action != actionDeny {
			p.add(newFinding(StatusDropped, cursorCLIFile, "permissions."+action, "", "unknown permission list"))
			continue
		}
		for _, rule := range doc.Permissions[action] {
			if claude, ok := cursorRule(rule, action); ok {
				b.add(action, claude)
				continue
			}
			p.add(newFinding(StatusUnsupported, cursorCLIFile, "permissions."+action, "",
				fmt.Sprintf("rule %s has no Claude Code permission rule and was not imported", rule)))
		}
		p.add(newFinding(StatusApproximated, cursorCLIFile, "permissions."+action, "permissions."+action,
			"Cursor rules were rewritten to Claude Code syntax (Shell to Bash, Write to Edit, Mcp to mcp__server__tool); review them"))
	}
}

// cursorRule rewrites one Cursor rule, Tool(specifier), to Claude Code syntax.
// Cursor's Shell(rm) matches every command that starts with rm, while Claude
// Code's Bash(rm) matches exactly "rm": a deny rule is widened to Bash(rm:*) so it
// still blocks "rm -rf x"; an allow rule stays exact, which is the safe direction.
func cursorRule(rule, action string) (string, bool) {
	rule = strings.TrimSpace(rule)
	name, spec, hasSpec := strings.Cut(rule, "(")
	spec = strings.TrimSuffix(spec, ")")
	if name == "Mcp" && hasSpec {
		server, tool, _ := strings.Cut(spec, ":")
		if server == "" || server == "*" {
			return "", false
		}
		if tool == "" || tool == "*" {
			return "mcp__" + server, true
		}
		return "mcp__" + server + "__" + tool, true
	}
	claude, ok := cursorToolNames[name]
	if !ok {
		return "", false
	}
	if !hasSpec {
		return claude, true
	}
	if name == "Shell" && action == actionDeny && spec != "" && !strings.Contains(spec, "*") {
		spec += ":*"
	}
	return claude + "(" + spec + ")", true
}

// rulesyncToolNames maps rulesync's permission categories to Claude Code tools.
var rulesyncToolNames = map[string]string{
	"bash": "Bash", "read": "Read", "edit": "Edit", "write": "Write", "webfetch": "WebFetch",
	"websearch": "WebSearch", "grep": "Grep", "glob": "Glob", "notebookedit": "NotebookEdit", "agent": "Agent",
}

// claudePathAliases are the tools whose path rules Claude Code reads as another
// tool's: a Write(src/**) rule is an Edit(src/**) rule (rulesync does the same).
var claudePathAliases = map[string]string{"Write": "Edit", "NotebookEdit": "Edit", "Glob": "Read"}

// rulesyncPermissionBlocks are the top-level keys of permissions.jsonc that are
// tool-scoped overrides rather than the shared `permission` block.
const rulesyncSharedPermission = "permission"

// importPermissions reads .rulesync/permissions.jsonc.
func (b *rulesyncPlanner) importPermissions(file string) {
	doc, ok := b.readJSONC(file)
	if !ok {
		return
	}
	pb := &permissionBuilder{p: b.p}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "$schema":
		case rulesyncSharedPermission:
			pb.rulesyncCategories(file, doc[k])
		default:
			b.p.add(newFinding(StatusDropped, file, k, "",
				"tool-scoped permission settings are not carried; [permissions] applies to every harness"))
		}
	}
}

func (b *permissionBuilder) rulesyncCategories(file string, raw json.RawMessage) {
	var table map[string]map[string]string
	if err := json.Unmarshal(raw, &table); err != nil {
		b.p.add(newFinding(StatusUnsupported, file, rulesyncSharedPermission, "", "permission is not {category: {pattern: action}}"))
		return
	}
	categories := make([]string, 0, len(table))
	for c := range table {
		categories = append(categories, c)
	}
	sort.Strings(categories)
	for _, category := range categories {
		field := rulesyncSharedPermission + "." + category
		if category == "*" {
			b.p.add(newFinding(StatusUnsupported, file, field, "", "a rule for every tool has no Claude Code permission rule; list the tools"))
			continue
		}
		tool := rulesyncToolNames[strings.ToLower(category)]
		if tool == "" {
			tool = category // an MCP tool name such as mcp__server__tool is passed through
		}
		patterns := make([]string, 0, len(table[category]))
		for pat := range table[category] {
			patterns = append(patterns, pat)
		}
		sort.Strings(patterns)
		for _, pat := range patterns {
			action := table[category][pat]
			if action != actionAllow && action != actionAsk && action != actionDeny {
				b.p.add(newFinding(StatusDropped, file, field+"."+pat, "", fmt.Sprintf("unknown action %q", action)))
				continue
			}
			rule := tool
			if pat != "*" {
				name := tool
				if alias, ok := claudePathAliases[tool]; ok {
					name = alias
				}
				rule = name + "(" + pat + ")"
			}
			b.add(action, rule)
		}
		b.p.add(newFinding(StatusMapped, file, field, "permissions", ""))
	}
}

// dedupePermissions sorts and de-duplicates every list.
func dedupePermissions(p *config.Permissions) {
	for _, list := range []*[]string{&p.Allow, &p.Ask, &p.Deny} {
		sort.Strings(*list)
		*list = dedupeStrings(*list)
	}
}
