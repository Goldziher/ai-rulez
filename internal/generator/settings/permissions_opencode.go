package settings

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// OpenCode, Kilo Code and MiMo Code share one permission surface: the
// `permission` object of their JSONC config, one pattern map per tool
// (`"bash": {"git *": "allow"}`). `*` matches any run of characters, `?` one
// character, and the LAST matching rule wins.
//
// Sources (vendor documentation, read 2026-10-05):
//   - opencode: https://opencode.ai/docs/permissions/ (pattern semantics from
//     packages/opencode/src/util/wildcard.ts)
//   - kilo:     https://kilo.ai/docs/code-with-ai/platforms/cli
//   - mimocode: https://mimo.xiaomi.com/mimocode/permissions
//
// Entries are owned one by one (a Members key per tool). Because the last match
// wins and the merge writes a map in key order, the translator checks that no
// weaker rule would land after an overlapping stricter one (its own or one the
// user wrote) and drops the weaker rule when it would: a rule is never relaxed.
var (
	_ = registerPermissionDialect("opencode", buildOpencode)
	_ = registerPermissionDialect("kilo", buildOpencode)
	_ = registerPermissionDialect("mimocode", buildOpencode)
)

const (
	ocBash      = "bash"
	ocRead      = "read"
	ocEdit      = "edit"
	ocWebfetch  = "webfetch"
	ocWebsearch = "websearch"
	ocTask      = "task"
)

type ocRule struct {
	pattern string
	entry   permEntry
}

func buildOpencode(t *translation) ([]jsonmerge.OwnedKey, error) {
	byTool := map[string][]ocRule{}
	for _, e := range t.entries {
		tool, patterns, why := opencodePatterns(e)
		if why != "" {
			t.drop(e, why)
			continue
		}
		if r := e.Rule; (r.Kind == KindRead || r.Kind == KindEdit) && !r.Bare && e.Action != ActionAllow &&
			(r.Path.Anchor == AnchorHome || r.Path.Anchor == AnchorAbsolute) {
			t.cfg.Diag.Warn(fmt.Sprintf("SECURITY: [permissions] %s rule %q may not be enforced by %s: it matches paths relative to the worktree, "+
				"so a home or absolute path pattern may never apply", e.Action, r.Raw, t.harness),
				"severity", "error", "hint", "enforce it another way (sandbox, hook) or remove the harness from the project")
		}
		for _, p := range patterns {
			byTool[tool] = append(byTool[tool], ocRule{pattern: p, entry: e})
		}
	}
	tools := make([]string, 0, len(byTool))
	for tool := range byTool {
		tools = append(tools, tool)
	}
	sort.Strings(tools)

	var keys []jsonmerge.OwnedKey
	for _, tool := range tools {
		path := []string{"permission", tool}
		existing, ok := existingObject(t.cfg, t.docPath, path)
		if !ok {
			for _, r := range byTool[tool] {
				t.drop(r.entry, "permission."+tool+" is not an object in the document, so it is the consumer's")
			}
			continue
		}
		entries := orderSafe(t, byTool[tool], existing)
		if len(entries) == 0 {
			continue
		}
		values := make(map[string]any, len(entries))
		for pattern, action := range entries {
			values[pattern] = string(action)
		}
		if key, ok := docMembersKey(t, path, values); ok {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// existingObject returns the object at path in the document: ok is false when the
// member exists but is not an object.
func existingObject(cfg *config.Config, docPath string, path []string) (map[string]any, bool) {
	v, found := jsonmerge.LookupTree(docTree(cfg, docPath), path)
	if !found {
		return nil, true
	}
	m, isMap := v.(map[string]any)
	return m, isMap
}

// orderSafe returns the pattern->action map to write for one tool. Same-pattern
// rules keep the strictest action; a rule that would sort after an overlapping
// stricter rule, ours or the user's, is dropped.
func orderSafe(t *translation, rules []ocRule, existing map[string]any) map[string]PermAction {
	chosen := map[string]ocRule{}
	for _, r := range rules {
		prev, dup := chosen[r.pattern]
		switch {
		case !dup:
			chosen[r.pattern] = r
		case strictness(r.entry.Action) > strictness(prev.entry.Action):
			t.drop(prev.entry, "the stricter rule "+r.entry.Rule.Raw+" covers the same pattern")
			chosen[r.pattern] = r
		default:
			t.drop(r.entry, "the stricter rule "+prev.entry.Rule.Raw+" covers the same pattern")
		}
	}
	patterns := make([]string, 0, len(chosen))
	for p := range chosen {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)

	out := map[string]PermAction{}
	for i, p := range patterns {
		r := chosen[p]
		if blocker := stricterBefore(patterns[:i], chosen, r, existing); blocker != "" {
			t.drop(r.entry, "it would be evaluated after the overlapping, stricter rule "+blocker+" and override it")
			continue
		}
		out[p] = r.entry.Action
	}
	return out
}

// stricterBefore returns the pattern of a rule that overlaps r, is stricter and
// is evaluated before it: one of our earlier patterns, or any rule the user
// already has (their order is unknown, so every one is assumed to come first).
func stricterBefore(earlier []string, chosen map[string]ocRule, r ocRule, existing map[string]any) string {
	for _, p := range earlier {
		if strictness(chosen[p].entry.Action) > strictness(r.entry.Action) && globsOverlap(p, r.pattern) {
			return p
		}
	}
	names := make([]string, 0, len(existing))
	for name := range existing {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, ours := chosen[name]; ours {
			continue
		}
		action, isString := existing[name].(string)
		if !isString {
			action = ""
		}
		if strictness(PermAction(action)) > strictness(r.entry.Action) && globsOverlap(name, r.pattern) {
			return name
		}
	}
	return ""
}

// opencodePatterns maps one rule to its permission key and patterns.
func opencodePatterns(e permEntry) (tool string, patterns []string, why string) {
	r := e.Rule
	switch r.Kind {
	case KindShell:
		return shellPatterns(e)
	case KindRead, KindEdit:
		return pathPatterns(e)
	case KindFetch:
		return fetchPatterns(e)
	case KindSearch:
		if !r.Bare {
			return "", nil, "WebSearch takes no specifier in OpenCode"
		}
		return ocWebsearch, []string{"*"}, ""
	case KindAgent:
		if r.Bare {
			return ocTask, []string{"*"}, ""
		}
		return ocTask, []string{r.Specifier}, ""
	case KindMCP:
		return "", nil, "MCP tool names are not a documented permission key"
	}
	return "", nil, "the harness has no equivalent of " + r.Tool + " rules"
}

func shellPatterns(e permEntry) (tool string, patterns []string, why string) {
	p := e.Rule.Shell()
	if e.Action == ActionAllow && strings.Contains(p.Literal, "?") {
		return "", nil, "`?` is a literal in Claude rules but a wildcard here, which would widen the rule"
	}
	switch p.Kind {
	case ShellAny:
		return ocBash, []string{"*"}, ""
	case ShellPrefix:
		return ocBash, []string{p.Literal + " *"}, ""
	case ShellExact:
		return ocBash, []string{p.Literal}, ""
	}
	return ocBash, []string{p.Literal}, ""
}

// pathPatterns maps a path rule. OpenCode matches a pattern against the path
// relative to the worktree (read.ts passes path.relative(worktree, file)), and `*`
// there matches `/` as well, unlike in Claude rules. So a single `*` in an allow
// would widen it, and a home or absolute path never matches a relative one: both
// allows are dropped. A deny is written for the root and for every depth, because
// a pattern is anchored at the worktree root; a home or absolute deny is written
// as given but reported, since it may not be enforced.
func pathPatterns(e permEntry) (tool string, patterns []string, why string) {
	r := e.Rule
	tool = ocRead
	if r.Kind == KindEdit {
		tool = ocEdit
		if r.Tool != "Edit" && e.Action == ActionAllow {
			return "", nil, "the edit permission also covers every other edit tool, so a " + r.Tool + " allow rule would be widened"
		}
	}
	if r.Bare {
		return tool, []string{"*"}, ""
	}
	glob := r.Path.Glob
	if e.Action == ActionAllow {
		switch {
		case strings.Contains(glob, "?"):
			return "", nil, "`?` is a literal in Claude rules but a wildcard here, which would widen the rule"
		case hasSingleStar(glob):
			return "", nil, "`*` matches across `/` here but only within one path segment in Claude rules, which would widen the rule"
		case r.Path.Anchor == AnchorHome || r.Path.Anchor == AnchorAbsolute:
			return "", nil, "OpenCode matches paths relative to the worktree, so a home or absolute path rule would not apply"
		}
		return tool, []string{glob}, ""
	}
	switch r.Path.Anchor {
	case AnchorHome:
		return tool, []string{"~/" + glob}, ""
	case AnchorAbsolute:
		return tool, []string{glob}, ""
	}
	return tool, denyPathVariants(glob), ""
}

// hasSingleStar reports whether glob holds a `*` that is not part of `**`.
func hasSingleStar(glob string) bool {
	for i := 0; i < len(glob); i++ {
		if glob[i] != '*' {
			continue
		}
		if i+1 < len(glob) && glob[i+1] == '*' {
			i++
			continue
		}
		return true
	}
	return false
}

// denyPathVariants returns the glob as written, for the root, and under any
// directory: `**/` needs a `/` before the name, so `**/.env` alone misses `.env`.
func denyPathVariants(glob string) []string {
	bare := strings.TrimPrefix(glob, "**/")
	return []string{bare, "**/" + bare}
}

func fetchPatterns(e permEntry) (tool string, patterns []string, why string) {
	r := e.Rule
	if r.Bare {
		return ocWebfetch, []string{"*"}, ""
	}
	if r.Domain == "" || strings.ContainsAny(r.Domain, "*?/") {
		return "", nil, "only WebFetch(domain:host) rules carry over, matched as a URL pattern"
	}
	patterns = []string{"https://" + r.Domain + "/*"}
	if e.Action != ActionAllow {
		// A URL with a port is not matched by `https://host/*`: add the port forms.
		patterns = append(patterns, "https://"+r.Domain, "http://"+r.Domain, "http://"+r.Domain+"/*",
			"https://"+r.Domain+":*", "http://"+r.Domain+":*")
	}
	return ocWebfetch, patterns, ""
}
