package lint

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// CodeUndeclaredCapability is the code of the declared-versus-used capability check.
const CodeUndeclaredCapability = "AR058"

func registerArDeclared(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeUndeclaredCapability, "undeclared-capability", SeverityInfo, "a skill that lists allowed-tools runs shell, network or file-mutating commands (in its instructions or scripts) that no allowed-tools entry covers"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeUndeclaredCapability: {
			Why:  "`allowed-tools` records what a skill says it needs. A skill that declares `Read, Grep` but whose scripts run `curl` or `rm` does more than it declares, which is how a reviewer is misled about its reach. The commands come from the capability profile of AR030 (fenced shell blocks, markdown resources and shell scripts). Only under-declaration is reported: a broad grant is AR007, and a declared tool with no matching command is normal for a skill that works through prose. The rule is informational because the field preapproves tools rather than limiting them; a missing entry means a permission prompt, not a block.",
			Bad:  "`allowed-tools: Read, Grep` on a skill whose `scripts/sync.sh` runs `curl` and `git push`",
			Good: "`allowed-tools: Read, Grep, Bash(curl:*), Bash(git push:*)`, or drop the command from the skill",
		},
	})
	s.addRunCheck(checkDeclaredCapabilities, AnalyzerSecurity)
}

func (p *capProfile) note(cmd string, tier int) {
	if p.cmds == nil {
		p.cmds = map[string]int{}
	}
	p.cmds[cmd] = max(p.cmds[cmd], tier)
}

// toolGrants is what an allowed-tools value preapproves.
type toolGrants struct {
	allShell bool
	scopes   []string // the leading word of each Bash(<command>:*) entry
}

func parseGrants(tools []string) toolGrants {
	var g toolGrants
	for _, tool := range tools {
		switch base := toolBase(tool); {
		case tool == "*":
			g.allShell = true
		case base == toolBash && broadTool(tool):
			g.allShell = true
		case base == toolBash:
			inner := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(tool, toolBash), "("), ")")
			if words := strings.Fields(strings.ReplaceAll(inner, ":", " ")); len(words) > 0 {
				g.scopes = append(g.scopes, words[0])
			}
		}
	}
	return g
}

// proseCommandWords are command names that are also plain words, so a template
// or a sentence in an untagged fence reads like a command ("service degradation").
var proseCommandWords = nameSet("service host patch install mount")

func (g toolGrants) covers(cmd string) bool {
	return g.allShell || slices.Contains(g.scopes, cmd)
}

func checkDeclaredCapabilities(r *runner) {
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc || it.kind != kindSkill || it.itemDir == "" {
			continue
		}
		d, ok := r.docs[it.abs]
		if !ok {
			continue
		}
		k, declared := parseFrontmatterDoc(d).top(keyAllowedTools)
		tools := splitTools(k.Value)
		if !declared || len(tools) == 0 {
			continue
		}
		r.reportUndeclared(it, k.Line, parseGrants(tools), r.itemProfile(it))
	}
}

func (r *runner) reportUndeclared(it *item, line int, g toolGrants, p capProfile) {
	var uncovered []string
	network := false
	for _, cmd := range slices.Sorted(maps.Keys(p.cmds)) {
		tier := p.cmds[cmd]
		if tier == tierRead || proseCommandWords[cmd] || g.covers(cmd) {
			continue
		}
		uncovered = append(uncovered, cmd)
		network = network || tier == tierNetwork
	}
	if len(uncovered) == 0 {
		return
	}
	sev := SeverityInfo
	if network {
		sev = SeverityWarning
	}
	r.addSev(sev, CodeUndeclaredCapability, it.abs, line, "%s runs %s, which no allowed-tools entry covers; add the matching Bash(...) entries or remove the commands",
		itemID(it.kind, it.cf), describeCommands(uncovered))
}

const maxListedCommands = 4

func describeCommands(cmds []string) string {
	if len(cmds) <= maxListedCommands {
		return strings.Join(cmds, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(cmds[:maxListedCommands], ", "), len(cmds)-maxListedCommands)
}
