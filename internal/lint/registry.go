package lint

// Rule families added after the first strict-validation release register
// themselves from init() in their own file: a RuleInfo for the code registry,
// plus any of a per-item check, a per-text scan or a whole-run check. The
// runner calls the hooks at fixed points, so a new rule never edits lint.go.

type (
	// itemCheck runs once per owned, non-document item with its parsed frontmatter.
	itemCheck func(r *runner, it *item, d doc, fm frontmatter)
	// textScan runs once per scanned text (content, markdown resource, shipped script).
	textScan func(r *runner, t *scanText)
	// runCheck runs once per run, after every item has been checked.
	runCheck func(r *runner)
)

// registered pairs a hook with the unit that gates it.
type registered[F any] struct {
	unit unitSpec
	fn   F
}

var (
	itemChecks []registered[itemCheck]
	textScans  []registered[textScan]
	runChecks  []registered[runCheck]
)

// registerRules adds rules to the code registry.
func registerRules(infos ...RuleInfo) { registry = append(registry, infos...) }

// registerRuleDocs adds the long-form explanation of rules by code. Every
// registered code needs one (TestEveryRuleHasDocs).
func registerRuleDocs(docs map[string]RuleDoc) {
	for code, d := range docs {
		ruleDocs[code] = d
	}
}

// The register functions take the analyzers whose rules the hook can report, so
// a run that selects other analyzers skips it (see units.go).
func registerItemCheck(fn itemCheck, analyzers ...string) {
	itemChecks = append(itemChecks, registered[itemCheck]{mustDeclare("item check", fn, analyzers), fn})
}

func registerTextScan(fn textScan, analyzers ...string) {
	textScans = append(textScans, registered[textScan]{mustDeclare("text scan", fn, analyzers), fn})
}

func registerRunCheck(fn runCheck, analyzers ...string) {
	runChecks = append(runChecks, registered[runCheck]{mustDeclare("run check", fn, analyzers), fn})
}

func (r *runner) runItemChecks(it *item, d doc, fm frontmatter) {
	for _, c := range itemChecks {
		r.unit(c.unit, func() { c.fn(r, it, d, fm) })
	}
}

func (r *runner) runTextScans(abs, raw string) {
	if len(textScans) == 0 {
		return
	}
	var t *scanText
	for _, c := range textScans {
		if !r.selected(c.unit) {
			continue
		}
		if t == nil {
			t = newScanText(r, abs, raw)
		}
		r.unit(c.unit, func() { c.fn(r, t) })
	}
}

func (r *runner) runRunChecks() {
	for _, c := range runChecks {
		r.unit(c.unit, func() { c.fn(r) })
	}
}

// severityOverridden reports whether [lint.severity] names the code.
func (r *runner) severityOverridden(code string) bool {
	for key := range r.lc.Severity {
		if rule, ok := lookupRule(key); ok && rule.Code == code {
			return true
		}
	}
	return false
}

// addSev is add with a severity that depends on the case: a rule whose default
// is a warning can still report its mild cases as info. A [lint.severity]
// entry or `off` by default always wins over the natural severity.
func (r *runner) addSev(natural Severity, code, abs string, line int, format string, args ...any) {
	saved := r.sev[code]
	if saved != SeverityOff && !r.severityOverridden(code) {
		r.sev[code] = natural
	}
	r.add(code, abs, line, format, args...)
	r.sev[code] = saved
}
