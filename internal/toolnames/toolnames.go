// Package toolnames is the single source of truth for how a harness names the
// tools Claude Code names Bash, Read, Edit, Write and so on. Hook matchers
// (internal/generator/settings) and the plugin-based hook runtimes
// (internal/generator/hookplugins) both read it, so a tool is translated one way
// everywhere.
//
// A mapping is added only where the vendor documents the tool's name; a name
// that is not in a vocabulary is reported as unmapped, never guessed. Each
// vocabulary cites the page it was read from and the date.
package toolnames

import (
	"regexp"
	"slices"
	"strings"
)

// Claude Code tool names, in the order the inverse tables list them.
const (
	Bash         = "Bash"
	Read         = "Read"
	Edit         = "Edit"
	MultiEdit    = "MultiEdit"
	Write        = "Write"
	Grep         = "Grep"
	Glob         = "Glob"
	LS           = "LS"
	WebFetch     = "WebFetch"
	WebSearch    = "WebSearch"
	Task         = "Task"
	Agent        = "Agent"
	NotebookEdit = "NotebookEdit"
	TodoWrite    = "TodoWrite"
	Skill        = "Skill"
)

// ClaudeTools lists the Claude Code tool names a vocabulary can map.
var ClaudeTools = []string{
	Bash, Read, Edit, MultiEdit, Write, Grep, Glob, LS, WebFetch, WebSearch, Task, Agent, NotebookEdit, TodoWrite, Skill,
}

// Vocabulary is how one harness names tools in a hook matcher.
type Vocabulary struct {
	// Harness is the preset name.
	Harness string
	// Source cites the vendor page and the date it was read ("URL (read 2026-10-05)").
	Source string
	// Tools maps a Claude Code tool name to the harness's names for it. Several
	// names mean the tool is spread over them (Edit is edit and apply_patch).
	Tools map[string][]string
	// MCP is the template of a harness's MCP tool name with {server} and {tool}
	// placeholders ("mcp__{server}__{tool}"); empty when the vendor documents
	// none, in which case an MCP token is unmapped.
	MCP string
	// MatchAll is the harness's own spelling of "every tool", for a Claude
	// matcher of "*" or ".*"; empty when the vendor documents none.
	MatchAll string
	// NoAlternation is true when the matcher is not a regular expression (a
	// glob or an exact name), so a matcher naming several tools is unmapped.
	NoAlternation bool
	// Search is true when the vendor tests the matcher as an unanchored regular
	// expression. A Claude matcher matches a whole tool name, so each native name
	// is then anchored to stay exact.
	Search bool
	// Glob is true when `.*` in an MCP pattern is spelled `*`.
	Glob bool
}

var vocabularies = map[string]*Vocabulary{}

// register adds a vocabulary; called from the init of the vocabulary files.
func register(v *Vocabulary) {
	vocabularies[v.Harness] = v
}

// For returns the vocabulary of a harness.
func For(harness string) (*Vocabulary, bool) {
	v, ok := vocabularies[harness]
	return v, ok
}

// Harnesses lists the harnesses with a vocabulary, sorted.
func Harnesses() []string {
	out := make([]string, 0, len(vocabularies))
	for name := range vocabularies {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// Native returns the harness's names for a Claude Code tool.
func (v *Vocabulary) Native(claude string) ([]string, bool) {
	names, ok := v.Tools[claude]
	return names, ok && len(names) > 0
}

// Inverse maps each native name to the Claude Code names that select it, in
// ClaudeTools order. It is what the plugin runtimes match a matcher against.
func (v *Vocabulary) Inverse() map[string][]string {
	out := map[string][]string{}
	for _, claude := range ClaudeTools {
		for _, native := range v.Tools[claude] {
			out[native] = append(out[native], claude)
		}
	}
	return out
}

var (
	toolToken = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
	// mcpSegment is a literal name optionally ending in `.*`, or `.*` alone.
	mcpSegment = regexp.MustCompile(`^(?:[A-Za-z0-9_-]+(?:\.\*)?|\.\*)$`)
)

// TranslateMatcher rewrites a Claude Code hook matcher into the harness's tool
// names, token by token over its top-level alternation (`Edit|Write`, `^Bash$`,
// `mcp__github__.*`). It reports ok false when any token has no documented
// mapping, with that token; a partial translation would widen or narrow the
// hook.
func (v *Vocabulary) TranslateMatcher(matcher string) (translated string, unmapped string, ok bool) {
	tokens := strings.Split(matcher, "|")
	if len(tokens) > 1 && v.NoAlternation {
		return "", matcher, false
	}
	// A regex that mixes anchored and bare names would match the bare ones as
	// substrings, so one anchored token anchors them all.
	anchorAll := slices.ContainsFunc(tokens, func(t string) bool {
		return strings.HasPrefix(t, "^") || strings.HasSuffix(t, "$")
	})
	var out []string
	for _, token := range tokens {
		names, bad := v.translateToken(token, anchorAll)
		if bad {
			return "", token, false
		}
		for _, name := range names {
			if !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	if len(out) > 1 && v.NoAlternation {
		return "", matcher, false
	}
	return strings.Join(out, "|"), "", true
}

func (v *Vocabulary) translateToken(token string, anchorAll bool) (names []string, unmapped bool) {
	anchoredStart := strings.HasPrefix(token, "^")
	anchoredEnd := strings.HasSuffix(token, "$")
	bare := strings.TrimSuffix(strings.TrimPrefix(token, "^"), "$")
	switch {
	case bare == "*" || bare == ".*":
		if v.MatchAll == "" || (anchoredStart || anchoredEnd) && bare != ".*" {
			return nil, true
		}
		return []string{v.MatchAll}, false
	case toolToken.MatchString(bare):
		natives, ok := v.Native(bare)
		if !ok {
			return nil, true
		}
		return v.shape(natives, anchorAll || anchoredStart || anchoredEnd), false
	case strings.HasPrefix(bare, "mcp__"):
		name, ok := v.translateMCP(strings.TrimPrefix(bare, "mcp__"))
		if !ok {
			return nil, true
		}
		return v.shape([]string{name}, anchorAll || anchoredStart || anchoredEnd), false
	}
	return nil, true
}

// shape applies the harness's anchoring: an unanchored-search harness gets each
// name anchored (a Claude matcher is a whole-name match), and anchors the user
// wrote are kept where the harness is a regular expression.
func (v *Vocabulary) shape(natives []string, anchored bool) []string {
	out := make([]string, len(natives))
	for i, name := range natives {
		switch {
		case v.NoAlternation:
			out[i] = name
		case v.Search || anchored:
			out[i] = "^" + name + "$"
		default:
			out[i] = name
		}
	}
	return out
}

func (v *Vocabulary) translateMCP(rest string) (string, bool) {
	if v.MCP == "" {
		return "", false
	}
	server, tool, found := strings.Cut(rest, "__")
	if !found || !mcpSegment.MatchString(server) || !mcpSegment.MatchString(tool) {
		return "", false
	}
	if v.Glob {
		server, tool = globSegment(server), globSegment(tool)
	}
	return strings.NewReplacer("{server}", server, "{tool}", tool).Replace(v.MCP), true
}

func globSegment(s string) string { return strings.ReplaceAll(s, ".*", "*") }
