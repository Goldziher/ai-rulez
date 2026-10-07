package lint

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

// Agents, skills and commands can carry `hooks` and `mcpServers` in their own
// frontmatter. Those declarations run commands exactly as the settings and
// config.toml ones do, so they get the same checks: the hook schema (AR507),
// a script that is missing or not executable (AR501, AR502), an unpinned
// `npx -y` (AR021), and for inline MCP servers the shape, pin, credential and
// PATH checks (AR602, AR012, AR015, AR601).

func registerArFmcomponents(s *ruleSet) {
	s.addItemCheck(checkFrontmatterHooks, AnalyzerHooks, AnalyzerSecurity)
	s.itemChecks[len(s.itemChecks)-1].unit.deps = true // a hook script is a dependency of the file that declares it
}

// hasFrontmatterHooks reports the content kinds whose frontmatter declares hooks
// and mcpServers.
func hasFrontmatterHooks(kind string) bool {
	return kind == kindAgent || kind == kindSkill || kind == kindCommand
}

// lineAfter is the 1-based line of the first line at or after from (1-based)
// that holds needle, or from.
func lineAfter(lines []string, from int, needle string) int {
	for i := max(from-1, 0); i < len(lines); i++ {
		if strings.Contains(lines[i], needle) {
			return i + 1
		}
	}
	return max(from, 1)
}

// hookHandler is one command handler of a frontmatter hooks block.
type hookHandler struct {
	event, command string
}

func checkFrontmatterHooks(r *runner, it *item, d doc, fm frontmatter) {
	if !hasFrontmatterHooks(it.kind) {
		return
	}
	k, ok := fm.top(keyHooks)
	if !ok || k.Value == nil {
		return
	}
	raw, err := json.Marshal(k.Value)
	if err != nil {
		return
	}
	for _, p := range jsonHookProblems(raw) {
		r.add(CodeHookSchema, it.abs, lineAfter(d.lines, k.Line, p.needle), "%s", p.msg)
	}
	for _, h := range frontmatterHookCommands(raw) {
		line := lineAfter(d.lines, k.Line, firstLine(h.command))
		r.checkHookCommand(it.abs, h.event, h.command)
		r.checkRelativeHookScript(it, h, line)
		for _, seg := range segSplitRe.Split(stripShellComment(h.command), -1) {
			if pkg, why := pinProblem(shellWords(seg), true, false); pkg != "" {
				r.add(CodeUnpinnedExec, it.abs, line, "%s hook runs %q without pinning it (%s); pin a version so a new release cannot change what runs", h.event, pkg, why)
			}
		}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if len(line) > 40 {
		line = line[:40]
	}
	return line
}

// frontmatterHookCommands lists the command handlers of a hooks block, ordered
// by event and position. A block of the wrong shape yields none: AR507 reports it.
func frontmatterHookCommands(raw json.RawMessage) []hookHandler {
	var byEvent map[string][]struct {
		Hooks []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &byEvent) != nil {
		return nil
	}
	events := make([]string, 0, len(byEvent))
	for e := range byEvent {
		events = append(events, e)
	}
	sort.Strings(events)
	var out []hookHandler
	for _, e := range events {
		for _, g := range byEvent[e] {
			for _, h := range g.Hooks {
				if (h.Type == "" || h.Type == hookTypeCommand) && strings.TrimSpace(h.Command) != "" {
					out = append(out, hookHandler{event: e, command: h.Command})
				}
			}
		}
	}
	return out
}

// checkRelativeHookScript checks a command that starts with a ./ path: hooks run
// from the project directory, so the script is resolved against it.
func (r *runner) checkRelativeHookScript(it *item, h hookHandler, line int) {
	words := shellWords(strings.TrimSpace(h.command))
	if len(words) == 0 || !strings.HasPrefix(words[0], "./") {
		return
	}
	rel := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(words[0])), "./")
	found := ""
	for _, cand := range []string{rel, joinRel(r.baseRel, rel)} {
		if r.tree.Exists(cand) {
			found = cand
			break
		}
	}
	if found == "" {
		r.add(CodeHookMissing, it.abs, line, "%s hook runs %q, which does not exist", h.event, words[0])
		return
	}
	abs := filepath.Join(r.tree.Top, filepath.FromSlash(found))
	r.dep(it.abs, abs)
	if exe, known := r.tree.Executable(found); known && !exe {
		r.addFix(chmodFix(abs), CodeHookNotExecutable, it.abs, line, "%s hook runs %q, which is not executable", h.event, words[0])
	}
}

// frontmatterMCPServers decodes the inline `mcpServers` of every owned agent,
// skill and command. An entry that is only a name refers to a server defined
// elsewhere and is skipped. The servers carry the line of their name in the file.
func (r *runner) frontmatterMCPServers() []*mcpServer {
	if r.fmMCPDone {
		return r.fmMCP
	}
	r.fmMCPDone = true
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc || !hasFrontmatterHooks(it.kind) {
			continue
		}
		d, ok := r.docs[it.abs]
		if !ok {
			continue
		}
		k, found := parseFrontmatterDoc(d).top(keyMCPServers)
		if !found {
			continue
		}
		for _, s := range decodeFrontmatterMCP(it.abs, k) {
			s.line = lineAfter(d.lines, k.Line, s.name)
			r.fmMCP = append(r.fmMCP, s)
		}
	}
	return r.fmMCP
}

// decodeFrontmatterMCP accepts both documented shapes: a mapping of server name
// to definition, and a list whose entries are a name or a one-key mapping.
func decodeFrontmatterMCP(file string, k fmKey) []*mcpServer {
	defs := map[string]any{}
	switch v := k.Value.(type) {
	case map[string]any:
		for name, def := range v {
			defs[name] = def
		}
	case []any:
		for _, entry := range v {
			if m, ok := entry.(map[string]any); ok {
				for name, def := range m {
					defs[name] = def
				}
			}
		}
	}
	for name, def := range defs {
		if _, isMap := def.(map[string]any); !isMap {
			delete(defs, name)
		}
	}
	if len(defs) == 0 {
		return nil
	}
	data, err := json.Marshal(map[string]any{keyMCPServers: defs})
	if err != nil {
		return nil
	}
	return decodeMCPJSON(file, keyMCPServers, data)
}
