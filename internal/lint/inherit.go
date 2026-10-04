package lint

import (
	"regexp"
	"strings"
)

// Assistants load the instruction files of every ancestor directory, so a
// nested root may legitimately name a skill, agent, command or rule that lives
// in a parent root (generated under .claude/, or still in its .ai-rulez/
// sources) or in a hand-authored .claude/ directory. The names are read from
// the tracked file list of each ancestor directory, up to the repository top.
var inheritedPatterns = []struct {
	re   *regexp.Regexp
	kind string
}{
	{regexp.MustCompile(`^(?:\.claude|\.ai-rulez(?:/domains/[^/]+)?)/skills/([^/]+)/SKILL\.md$`), kindSkill},
	{regexp.MustCompile(`^(?:\.claude|\.ai-rulez(?:/domains/[^/]+)?)/commands/([^/]+?)(?:\.md|/COMMAND\.md)$`), kindCommand},
	{regexp.MustCompile(`^(?:\.claude|\.ai-rulez(?:/domains/[^/]+)?)/agents/([^/]+)\.md$`), kindAgent},
	{regexp.MustCompile(`^(?:\.claude|\.ai-rulez(?:/domains/[^/]+)?)/rules/(?:.*/)?([^/]+)\.md$`), kindRule},
}

func (r *runner) addInheritedNames() {
	dirs := []string{""}
	if r.baseRel != "" {
		parts := strings.Split(r.baseRel, "/")
		for i := range parts {
			dirs = append(dirs, strings.Join(parts[:i+1], "/")+"/")
		}
	}
	sets := map[string]map[string]bool{kindSkill: r.skills, kindCommand: r.commands, kindAgent: r.agents, kindRule: r.rules}
	for f := range r.tree.files {
		if !strings.Contains(f, ".claude/") && !strings.Contains(f, ".ai-rulez/") {
			continue
		}
		for _, dir := range dirs {
			if rest, ok := strings.CutPrefix(f, dir); ok {
				addInherited(sets, rest)
			}
		}
	}
}

// addInherited registers the name a file below an ancestor directory defines.
func addInherited(sets map[string]map[string]bool, rest string) {
	for _, p := range inheritedPatterns {
		m := p.re.FindStringSubmatch(rest)
		if m == nil {
			continue
		}
		name := strings.ToLower(m[1])
		sets[p.kind][name] = true
		if p.kind == kindCommand {
			sets[kindSkill][name] = true
		}
	}
}
