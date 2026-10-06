package lint

import (
	"os"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// FrontmatterWarnings runs the frontmatter checks that plain `validate` reports
// as warnings: an agent frontmatter key no tool reads (AR303) and a `skills:`
// entry that names no skill (AR302). `validate --strict` reports the same
// findings at their configured severity (AR302 is an error there).
//
// Severities set in [lint] (severity, ignore, ignore_paths) still apply, so a
// config that silences a rule for strict runs silences it here too.
func FrontmatterWarnings(cfg *config.Config, tree *Tree, opts ...Option) []Finding {
	r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}}
	for _, opt := range opts {
		opt(r)
	}
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	r.baseRel = tree.Rel(r.rootAbs())
	if r.baseRel == "." {
		r.baseRel = ""
	}
	r.resolveSettings()
	configured := map[string]bool{}
	for key := range r.lc.Severity {
		if rule, ok := lookupRule(key); ok {
			configured[rule.Code] = true
		}
	}
	if !configured[CodeFrontmatterSkill] && r.sev[CodeFrontmatterSkill] == SeverityError {
		r.sev[CodeFrontmatterSkill] = SeverityWarning
	}
	r.collect()
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.isDoc {
			continue
		}
		data, err := os.ReadFile(it.abs)
		if err != nil {
			continue
		}
		d := parseDoc(string(data))
		r.docs[it.abs] = d
		if it.kind == kindAgent {
			r.checkFrontmatterKeys(it, parseFrontmatterDoc(d))
		}
		r.checkFrontmatterSkills(it, d)
	}
	sort.SliceStable(r.findings, func(i, j int) bool {
		a, b := r.findings[i], r.findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	return r.findings
}
