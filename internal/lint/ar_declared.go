package lint

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// CodeUndeclaredCapability is the code of the declared-versus-used capability check.
const CodeUndeclaredCapability = "AR058"

func registerArDeclared(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeUndeclaredCapability, "undeclared-capability", SeverityInfo, "a skill that lists allowed-tools runs shell, network or file-mutating commands (in its instructions or scripts) that no allowed-tools entry covers; one finding per command"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeUndeclaredCapability: {
			Why:  "`allowed-tools` records what a skill says it needs. A skill that declares `Read, Grep` but whose scripts run `curl` or `rm` does more than it declares, which is how a reviewer is misled about its reach. The commands come from the capability profile of AR030 (fenced shell blocks, markdown resources and shell scripts). One finding is reported per uncovered command, so a command added later is a new finding. An entry covers a command whatever wrapper or path it names (`Bash(/usr/bin/curl:*)`, `Bash(sudo curl:*)` and `Bash(env curl:*)` cover `curl`), and a `Bash(git push:*)` entry covers every `git` command. Only under-declaration is reported: a broad grant is AR007, and a declared tool with no matching command is normal for a skill that works through prose. The rule is informational, network commands included, because the field preapproves tools rather than limiting them; a missing entry means a permission prompt, not a block.",
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
			if cmd := grantCommand(strings.Fields(strings.ReplaceAll(inner, ":", " "))); cmd != "" {
				g.scopes = append(g.scopes, cmd)
			}
		}
	}
	return g
}

// grantWrappers are the words a Bash(...) entry may put before the command it
// allows; the command is the word after them.
var grantWrappers = sync.OnceValue(func() map[string]bool {
	return nameSet("sudo env command nohup time exec nice builtin")
})

// grantCommand names the command a Bash(...) entry allows: the first word that is not a
// wrapper or an environment assignment, without its directory (/usr/bin/curl is curl).
func grantCommand(words []string) string {
	for _, w := range words {
		if grantWrappers()[w] || strings.Contains(w, "=") {
			continue
		}
		return filepath.Base(w)
	}
	return ""
}

// proseCommandWords are command names that are also plain words, so a template
// or a sentence in an untagged fence reads like a command ("service degradation").
var proseCommandWords = sync.OnceValue(func() map[string]bool { return nameSet("service host patch install mount") })

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

// reportUndeclared adds one AR058 finding per command no allowed-tools entry covers.
// Each names its command, so a command added later is a new finding even when the
// earlier ones are in a baseline. The rule stays informational: a missing entry is a
// permission prompt, not a defect, whatever the command's tier.
func (r *runner) reportUndeclared(it *item, line int, g toolGrants, p capProfile) {
	for _, cmd := range slices.Sorted(maps.Keys(p.cmds)) {
		if p.cmds[cmd] == tierRead || proseCommandWords()[cmd] || g.covers(cmd) {
			continue
		}
		r.addSev(SeverityInfo, CodeUndeclaredCapability, it.abs, line, "%s runs %s, which no allowed-tools entry covers; add Bash(%s:*) or remove the command",
			itemID(it.kind, it.cf), cmd, cmd)
	}
}
