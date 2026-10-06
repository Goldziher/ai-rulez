package lint

import (
	"strings"
	"unicode/utf8"
)

// CodeBodyEmpty reports an item with frontmatter but no instructions.
const CodeBodyEmpty = "AR805"

// descWarnAt is where a description starts to warn that it is close to the limit.
const descWarnAt = 900

func init() {
	registerRules(RuleInfo{CodeBodyEmpty, "body-empty", SeverityWarning, "a skill, agent, command or rule has frontmatter but no body, so it instructs nothing"})
	registerItemCheck(checkBodyEmpty, AnalyzerDescriptions)
	registerItemCheck(checkDescriptionNearLimit, AnalyzerDescriptions)
}

func checkBodyEmpty(r *runner, it *item, d doc, _ frontmatter) {
	if it.kind == kindContext {
		return
	}
	for _, l := range d.lines[d.bodyStart:] {
		if strings.TrimSpace(l) != "" {
			return
		}
	}
	r.add(CodeBodyEmpty, it.abs, max(d.bodyStart, 1), "%s %q has no body after its frontmatter, so it gives the model nothing to follow", it.kind, itemID(it.kind, it.cf))
}

// checkDescriptionNearLimit extends AR802: a description that fits the limit but
// is within reach of it warns before an edit pushes it over and the tool truncates it.
func checkDescriptionNearLimit(r *runner, it *item, d doc, _ frontmatter) {
	if it.kind != kindSkill && it.kind != kindAgent && it.kind != kindCommand {
		return
	}
	maxLen := defaultDescMax
	if dc := r.lc.Description; dc != nil && dc.MaxLength > 0 {
		maxLen = dc.MaxLength
	}
	warnAt := min(descWarnAt, maxLen*9/10)
	if n := utf8.RuneCountInString(r.description(it)); n >= warnAt && n <= maxLen {
		r.addSev(SeverityInfo, CodeDescriptionLength, it.abs, d.lineOf(keyDescription, 1), "description is %d characters, within %d of the maximum of %d; trim it before an edit pushes it over", n, maxLen-n, maxLen)
	}
}
